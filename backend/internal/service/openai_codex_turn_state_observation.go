package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
)

const codexTurnStateObservationLimit = 1 << 20

type codexTurnStateRequest struct {
	accountID int64
	model     string
	entry     codexTurnStateCacheEntry
}

type codexTurnStateModelContextKey struct{}

func (s *OpenAIGatewayService) prepareCodexTurnStateHTTPRequest(ctx context.Context, account *Account, model, token string, req *http.Request) error {
	owner := s.codexTurnStateOwner(ctx, account)
	requestCtx := withCodexTurnStateOwner(req.Context(), owner)
	if owner == CodexTurnStateModeStatePool {
		requestCtx = context.WithValue(requestCtx, codexTurnStateModelContextKey{}, model)
	}
	*req = *req.WithContext(requestCtx)
	return s.applyCodexTurnStateForHTTPRequest(requestCtx, account, model, token, req.Header)
}

type codexTurnStateObservation struct {
	once           sync.Once
	expectedLength int
	model          string
	invalidate     func(string)
	onHeader       func(string)
	onDiagnostic   func(string)
}

func newCodexTurnStateObservation(ctx context.Context, repo AccountRepository, candidate *codexTurnStateRequest, wake func()) *codexTurnStateObservation {
	if repo == nil || candidate == nil {
		return nil
	}
	accountID := candidate.accountID
	event := CodexStateEvent{AccountID: accountID, Model: candidate.model, Source: "business", SentLength: len(candidate.entry.State)}
	return &codexTurnStateObservation{onHeader: func(state string) { event.ReturnedLength = len(state) }, onDiagnostic: func(reason string) {
		e := event
		e.Kind = "diagnostic"
		e.Reason = reason
		recordCodexStateEvent(repo, e)
	}, expectedLength: len(candidate.entry.State), model: candidate.model, invalidate: func(reason string) {
		event.Kind = "invalidation_observed"
		event.Reason = reason
		recordCodexStateEvent(repo, event)
		// The client may disconnect on a failure frame. Persist the observation with
		// a bounded, detached context so that cancellation cannot revive bad state.
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		for range 3 {
			current, err := repo.GetByID(saveCtx, accountID)
			if err != nil {
				slog.Warn("codex_turn_state_observation_read_failed", "account_id", accountID, "error", err)
				return
			}
			config, err := CodexTurnStateProbeConfigFromExtra(current.Extra)
			if err != nil || !config.Enabled || !IsCodexTurnStateProbeAccount(current) {
				return
			}
			cache := codexTurnStateCacheFromExtra(current.Extra)
			entry, ok := cache[candidate.model]
			if !ok || entry != candidate.entry {
				event.Kind = "invalidation_skipped"
				event.Reason = "candidate_already_changed"
				recordCodexStateEvent(repo, event)
				return
			}
			delete(cache, candidate.model)
			failures := codexTurnStateProbeFailuresFromExtra(current.Extra)
			failures[candidate.model] = codexTurnStateProbeFailure{Attempts: 0, FailedAt: time.Now().UTC(), Reason: reason}
			saved, err := saveCodexTurnStateProbeRuntime(saveCtx, repo, current, codexTurnStateProbeRuntimeUpdates(cache, failures))
			if err != nil {
				slog.Warn("codex_turn_state_observation_save_failed", "account_id", accountID, "error", err)
				return
			}
			if saved {
				event.Kind = "invalidated"
				event.Reason = reason
				recordCodexStateEvent(repo, event)
				slog.Info("codex_turn_state_invalidated", "account_id", accountID, "model", candidate.model, "reason", reason)
				if wake != nil {
					wake()
				}
				return
			}
		}
		slog.Warn("codex_turn_state_observation_conflict", "account_id", accountID, "model", candidate.model)
	}}
}

func (o *codexTurnStateObservation) observeReason(reason string) {
	if o != nil && o.invalidate != nil && reason != "" {
		o.once.Do(func() { o.invalidate(reason) })
	}
}

func (o *codexTurnStateObservation) observeHeader(state string) {
	if o != nil && o.onHeader != nil {
		o.onHeader(state)
	}
	if o != nil && state != "" && o.expectedLength > 0 && len(state) != o.expectedLength {
		o.observeReason("unexpected_state_length")
	}
}

func (o *codexTurnStateObservation) observeEvent(payload []byte, eventType string) {
	if o == nil {
		return
	}
	if eventType == "" {
		eventType = gjson.GetBytes(payload, "type").String()
	}
	if o.onDiagnostic != nil && (gjson.GetBytes(payload, "error.code").String() == "invalid_encrypted_content" || gjson.GetBytes(payload, "response.error.code").String() == "invalid_encrypted_content") {
		o.onDiagnostic("invalid_encrypted_content")
	}
	if eventType == "response.created" && o.model != "gpt-5.6-luna" && gjson.GetBytes(payload, "response.model").String() == "gpt-5.6-luna" {
		o.observeReason("response_created_gpt_5_6_luna")
		return
	}
	// Inspect structured error fields only. A user/tool/model quoting this code
	// in ordinary output is not evidence of an upstream overload.
	for _, path := range []string{"error.code", "error.type", "response.error.code", "response.error.type"} {
		if gjson.GetBytes(payload, path).String() == "server_is_overloaded" {
			o.observeReason("server_is_overloaded")
			return
		}
	}
	if eventType == "server_is_overloaded" || (eventType == "error" && gjson.GetBytes(payload, "code").String() == "server_is_overloaded") {
		o.observeReason("server_is_overloaded")
		return
	}
	if codexTurnStateMetadataEvent(eventType) {
		headers := gjson.GetBytes(payload, "headers")
		headers.ForEach(func(key, value gjson.Result) bool {
			if strings.EqualFold(key.String(), openAICodexTurnStateHeader) && value.Type == gjson.String {
				o.observeHeader(value.String())
			}
			return true
		})
	}
}

// This wrapper observes bytes as the existing gateway reads them. It does not
// consume ahead, rewrite output, or wait for a complete stream. Large events
// are skipped with bounded memory; subsequent events are still observed.
type codexTurnStateObservedBody struct {
	io.ReadCloser
	observer  *codexTurnStateObservation
	sse       bool
	line      []byte
	data      []byte
	eventType string
	skipLine  bool
	skipEvent bool
}

func (r *codexTurnStateObservedBody) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if r.sse {
		r.feed(p[:n])
	} else if !r.skipEvent {
		if len(r.data)+n > codexTurnStateObservationLimit {
			r.data = nil
			r.skipEvent = true
		} else {
			r.data = append(r.data, p[:n]...)
		}
	}
	if err == io.EOF {
		if r.sse {
			if len(r.line) > 0 && !r.skipLine {
				r.processLine(r.line)
				r.line = nil
			}
			r.flushEvent()
		} else if !r.skipEvent {
			r.observer.observeEvent(r.data, "")
			r.data = nil
		}
	}
	return n, err
}

func (r *codexTurnStateObservedBody) feed(p []byte) {
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		part := p
		if end >= 0 {
			part = p[:end]
		}
		if !r.skipLine {
			if len(r.line)+len(part) > codexTurnStateObservationLimit {
				r.line = nil
				r.skipLine = true
				r.skipEvent = true
			} else {
				r.line = append(r.line, part...)
			}
		}
		if end < 0 {
			return
		}
		if !r.skipLine {
			r.processLine(r.line)
		}
		r.line = r.line[:0]
		r.skipLine = false
		p = p[end+1:]
	}
}

func (r *codexTurnStateObservedBody) processLine(line []byte) {
	line = bytes.TrimSuffix(line, []byte{'\r'})
	if len(line) == 0 {
		r.flushEvent()
		return
	}
	if bytes.HasPrefix(line, []byte("event:")) {
		r.eventType = strings.TrimSpace(string(line[6:]))
		return
	}
	if !bytes.HasPrefix(line, []byte("data:")) || r.skipEvent {
		return
	}
	value := bytes.TrimPrefix(line[5:], []byte{' '})
	if len(r.data)+len(value)+1 > codexTurnStateObservationLimit {
		r.data = nil
		r.skipEvent = true
		return
	}
	r.data = append(r.data, value...)
	r.data = append(r.data, '\n')
}

func (r *codexTurnStateObservedBody) flushEvent() {
	if !r.skipEvent && len(r.data) > 0 {
		r.observer.observeEvent(r.data, r.eventType)
	}
	r.data = r.data[:0]
	r.eventType = ""
	r.skipEvent = false
}

// A connection can outlive candidate renewals. Refresh only runtime state;
// its pinned engine, model mapping and credential identity remain unchanged.
func (s *OpenAIGatewayService) currentCodexTurnStateAccount(ctx context.Context, account *Account) (*Account, error) {
	if s.accountRepo == nil {
		return account, nil
	}
	current, err := s.accountRepo.GetByID(ctx, account.ID)
	if err != nil {
		return nil, err
	}
	if current == nil || current.Type != account.Type || current.Platform != account.Platform ||
		current.IsShadow() || codexTurnStateOwnerIdentity(current) != codexTurnStateOwnerIdentity(account) {
		return nil, ErrCodexTurnStateUnavailable
	}
	snapshot := *account
	snapshot.Extra = maps.Clone(account.Extra)
	if snapshot.Extra == nil {
		snapshot.Extra = make(map[string]any)
	}
	for _, key := range []string{CodexTurnStateProbeCacheExtraKey, CodexTurnStateOutboundIdentityExtraKey, CodexTurnStateRoutingCookieExtraKey} {
		delete(snapshot.Extra, key)
		if value, ok := current.Extra[key]; ok {
			snapshot.Extra[key] = value
		}
	}
	return &snapshot, nil
}

func (s *OpenAIGatewayService) codexTurnStateObservation(ctx context.Context, account *Account, model string) *codexTurnStateObservation {
	model = codexTurnStateModelKey(account, model)
	entry, ok := codexTurnStateCacheFromExtra(account.Extra)[model]
	if !ok {
		return nil
	}
	return newCodexTurnStateObservation(ctx, s.accountRepo, &codexTurnStateRequest{accountID: account.ID, model: model, entry: entry}, func() {
		if s.codexTurnStateProbe != nil {
			s.codexTurnStateProbe.Wake()
		}
	})
}

func (s *OpenAIGatewayService) prepareCodexTurnStateWSFrame(ctx context.Context, account *Account, owner, token string, payload []byte) ([]byte, *codexTurnStateObservation, error) {
	if owner != CodexTurnStateModeStatePool {
		return payload, nil, nil
	}
	current, err := s.currentCodexTurnStateAccount(ctx, account)
	if err != nil {
		return nil, nil, err
	}
	output, err := applyCodexTurnStateToWSFrame(current, owner, token, payload)
	if err != nil {
		return nil, nil, err
	}
	return output, s.codexTurnStateObservation(ctx, current, gjson.GetBytes(output, "model").String()), nil
}

func observeCodexTurnStateHTTPResponse(resp *http.Response, observer *codexTurnStateObservation) {
	if resp == nil || observer == nil {
		return
	}
	observer.observeHeader(resp.Header.Get(openAICodexTurnStateHeader))
	if resp.Body != nil {
		resp.Body = &codexTurnStateObservedBody{ReadCloser: resp.Body, observer: observer, sse: strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")}
	}
}
