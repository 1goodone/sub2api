package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

type codexStateAcquisition struct {
	state                  string
	obtained               time.Time
	routing                codexTurnStateRoutingCookies
	status, returnedLength int
	err                    error
}

func (s *CodexTurnStateProbeService) acquireState(parent context.Context, account *Account, proxy *Proxy, token, model, supplied string, identity codexTurnStateOutboundIdentity) (out codexStateAcquisition) {
	ctx, cancel := context.WithTimeout(parent, codexTurnStateProbeTimeout)
	defer cancel()
	out.obtained = time.Now().UTC()
	if account == nil || proxy == nil {
		out.err = errors.New("probe_unavailable")
		return
	}
	payload := createOpenAITestPayload(model, true)
	applyCodexTurnStateOutboundIdentityProbePayload(payload, identity)
	body, err := json.Marshal(payload)
	if err != nil {
		out.err = errors.New("invalid_probe_payload")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatgptCodexURL, bytes.NewReader(body))
	if err != nil {
		out.err = errors.New("invalid_probe_request")
		return
	}
	ensureCodexIdentityHeaders(req.Header)
	applyOpenAICodexProbeHeaders(req.Header)
	setOpenAIChatGPTAccountHeaders(req.Header, account)
	if ua := strings.TrimSpace(account.GetOpenAIUserAgent()); ua != "" {
		enforceCodexIdentityHeadersWithUA(req.Header, ua)
	}
	account.ApplyHeaderOverrides(req.Header)
	applyCodexTurnStateOutboundIdentityHeaders(req.Header, identity)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	applyCodexRoutingCookies(req.Header, account, out.obtained)
	req.Header.Del(openAICodexTurnStateHeader)
	if supplied != "" {
		req.Header.Set(openAICodexTurnStateHeader, supplied)
	}
	req.Close = true
	req = req.WithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(req.Context(), HTTPUpstreamProfileOpenAIHarvest)))
	var resp *http.Response
	if s.requestDo != nil {
		resp, err = s.requestDo(req, proxy.URL())
	} else {
		if s.httpUpstream == nil {
			out.err = errors.New("probe_unavailable")
			return
		}
		resp, err = s.httpUpstream.Do(req, proxy.URL(), account.ID, account.Concurrency)
	}
	if err != nil {
		out.err = errors.New("probe_transport_failed")
		if ctx.Err() != nil {
			out.err = errors.New("probe_timeout")
			if errors.Is(ctx.Err(), context.Canceled) {
				out.err = errors.New("probe_cancelled")
			}
		}
		return
	}
	if resp == nil {
		out.err = errors.New("empty_response")
		return
	}
	if resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	out.status = resp.StatusCode
	if routing, ok := codexRoutingCookiesFromResponse(resp.Header, out.obtained); ok {
		out.routing = routing
	}
	out.state = resp.Header.Get(openAICodexTurnStateHeader)
	out.returnedLength = len(out.state)
	if out.status != http.StatusOK {
		out.err = fmt.Errorf("upstream_http_%d", out.status)
		return
	}
	check := func(state string) error {
		if len(state) != CodexTurnStateHealthyLength(account) {
			return errors.New("unexpected_state_length")
		}
		_, _, e := codexStateTimes(state, out.obtained, time.Now())
		return e
	}
	if out.state != "" {
		if out.err = check(out.state); out.err != nil {
			return
		}
		if supplied == "" {
			return
		}
		if out.state != supplied {
			out.err = errors.New("state_not_accepted")
			return
		}
	}
	if resp.Body == nil {
		out.err = errors.New("missing_sse_body")
		return
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 4<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var data strings.Builder
	served, completed := "", false
	consume := func() bool {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		if payload == "" || payload == "[DONE]" {
			return false
		}
		if !gjson.Valid(payload) {
			out.err = errors.New("invalid_sse_event")
			return true
		}
		kind := gjson.Get(payload, "type").String()
		for _, path := range []string{"error.code", "error.type", "response.error.code", "response.error.type"} {
			code := gjson.Get(payload, path).String()
			if code == "server_is_overloaded" || code == "invalid_encrypted_content" {
				out.err = errors.New(code)
				return true
			}
		}
		if kind == "error" || kind == "response.failed" || kind == "response.incomplete" || kind == "server_is_overloaded" {
			out.err = errors.New("upstream_response_failed")
			return true
		}
		for _, path := range []string{"error.code", "response.error.code"} {
			if gjson.Get(payload, path).String() != "" {
				out.err = errors.New("upstream_response_failed")
				return true
			}
		}
		if kind == "response.created" || kind == "response.completed" {
			if value := gjson.Get(payload, "response.model").String(); value != "" {
				served = value
			}
		}
		if served != "" && served != model {
			out.err = errors.New("upstream_model_mismatch")
			return true
		}
		if state := codexTurnStateFromMetadata([]byte(payload)); state != "" {
			out.state = state
			out.returnedLength = len(state)
			if out.err = check(state); out.err != nil {
				return true
			}
			if supplied != "" && state != supplied {
				out.err = errors.New("state_not_accepted")
				return true
			}
			if supplied == "" {
				return true
			}
		}
		if kind == "response.completed" {
			completed = true
			return true
		}
		return false
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if consume() {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			_, _ = data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			_ = data.WriteByte('\n')
		}
	}
	if data.Len() > 0 && out.err == nil {
		consume()
	}
	if out.err != nil {
		return
	}
	if scanner.Err() != nil || ctx.Err() != nil {
		out.err = errors.New("probe_stream_incomplete")
		return
	}
	if supplied != "" {
		if !completed || served != model {
			out.err = errors.New("validation_not_completed")
			return
		}
		out.state = supplied
	} else if out.state == "" {
		out.err = errCodexTurnStateMissing
		return
	}
	out.err = check(out.state)
	return
}
