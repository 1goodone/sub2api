package service

import (
	"context"
	"io"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type codexPoolRuntimeRepo struct {
	codexPoolAccountRepo
	writes int
	events []CodexStateEvent
}

func (r *codexPoolRuntimeRepo) CompareAndSwapCodexTurnStateProbe(ctx context.Context, expected *Account, updates map[string]any) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !reflect.DeepEqual(expected.Extra, r.account.Extra) {
		return false, nil
	}
	updated := *r.account
	updated.Extra = maps.Clone(r.account.Extra)
	for key, value := range updates {
		delete(updated.Extra, key)
		if value != nil {
			updated.Extra[key] = value
		}
	}
	r.account = &updated
	r.writes++
	return true, nil
}

func (r *codexPoolRuntimeRepo) AppendCodexStateEvent(_ context.Context, event CodexStateEvent) error {
	r.events = append(r.events, event)
	return nil
}
func (r *codexPoolRuntimeRepo) ListCodexStateEvents(context.Context, int64, string, int64, int) ([]CodexStateEvent, error) {
	return r.events, nil
}
func (r *codexPoolRuntimeRepo) CleanupCodexStateEvents(context.Context, time.Time) error { return nil }

func TestCodexTurnStateObservationOnlyInvalidatesTheSentCandidate(t *testing.T) {
	for _, scenario := range []string{"overload", "downgrade", "wrong_length", "encrypted_error", "quoted_error", "newer_candidate", "switched_owner"} {
		t.Run(scenario, func(t *testing.T) {
			account := poolTestAccount("plus")
			repo := &codexPoolRuntimeRepo{codexPoolAccountRepo: codexPoolAccountRepo{account: account}}
			svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, nil)
			svc.accountRepo = repo
			ctx, cancel := context.WithCancel(context.Background())
			observation := svc.codexTurnStateObservation(ctx, account, "gpt-6-astra")
			cancel() // A disconnected client cannot prevent invalidation from persisting.
			payload := []byte(`{"type":"error","error":{"code":"server_is_overloaded"}}`)
			switch scenario {
			case "downgrade":
				payload = []byte(`{"type":"response.created","response":{"model":"gpt-5.6-luna"}}`)
			case "wrong_length":
				observation.observeHeader(strings.Repeat("x", 312))
			case "encrypted_error":
				payload = []byte(`{"type":"response.failed","response":{"error":{"code":"invalid_encrypted_content"}}}`)
			case "quoted_error":
				payload = []byte(`{"type":"response.output_text.delta","delta":"server_is_overloaded"}`)
			case "newer_candidate":
				cache := codexTurnStateCacheFromExtra(account.Extra)
				entry := cache["gpt-6-astra"]
				entry.State = poolTestState("plus", time.Now().Add(10*time.Second))
				cache["gpt-6-astra"] = entry
				account.Extra[CodexTurnStateProbeCacheExtraKey] = codexTurnStateCacheToExtra(cache)
			case "switched_owner":
				account.Extra[CodexTurnStateModeExtraKey] = CodexTurnStateModeLegacyTicket
			}
			observation.observeEvent(payload, "")
			observation.observeEvent(payload, "")
			invalidate := scenario == "overload" || scenario == "downgrade" || scenario == "wrong_length"
			if invalidate {
				require.Equal(t, 1, repo.writes)
				require.Empty(t, codexTurnStateCacheFromExtra(repo.account.Extra))
			} else {
				require.Zero(t, repo.writes)
				require.NotEmpty(t, codexTurnStateCacheFromExtra(repo.account.Extra))
			}
			require.Contains(t, repo.account.Extra, openAICodexTicketExtraKey("gpt-6-astra"))
		})
	}
}

func TestCodexTurnStateHTTPTransportPreservesStreamAndBlocksInvalidatedRetry(t *testing.T) {
	account := poolTestAccount("plus")
	repo := &codexPoolRuntimeRepo{codexPoolAccountRepo: codexPoolAccountRepo{account: account}}
	wire := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\r\n\r\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\"}}\r\n\r\n"
	calls := 0
	upstream := &codexTicketFuncUpstream{do: func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, codexTurnStateCacheFromExtra(account.Extra)["gpt-6-astra"].State, req.Header.Get(openAICodexTurnStateHeader))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(iotest.OneByteReader(strings.NewReader(wire)))}, nil
	}}
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, upstream)
	svc.accountRepo = repo
	req, err := http.NewRequest(http.MethodPost, "https://example.test/responses", strings.NewReader(`{"model":"gpt-6-astra"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer tok")
	require.NoError(t, svc.prepareCodexTurnStateHTTPRequest(context.Background(), account, "gpt-6-astra", "tok", req))
	resp, err := svc.doOpenAIUpstream(req, "", account)
	require.NoError(t, err)
	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, wire, string(got))
	require.Equal(t, 1, repo.writes)
	_, err = svc.doOpenAIUpstream(req, "", account)
	require.ErrorIs(t, err, ErrCodexTurnStateUnavailable)
	require.Equal(t, 1, calls, "retry must not send the original invalidated snapshot")
}

func TestCodexTurnStateObservationSkipsOversizedEvents(t *testing.T) {
	reasons := []string{}
	observer := &codexTurnStateObservation{invalidate: func(reason string) { reasons = append(reasons, reason) }}
	wire := "data: " + strings.Repeat("x", codexTurnStateObservationLimit+1) + "\n\ndata: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\"}}\n\n"
	body := &codexTurnStateObservedBody{ReadCloser: io.NopCloser(strings.NewReader(wire)), observer: observer, sse: true}
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, wire, string(got))
	require.Equal(t, []string{"server_is_overloaded"}, reasons)
}

func TestCodexTurnStateWSSessionUsesRenewedPoolWithoutChangingOwner(t *testing.T) {
	original := poolTestAccount("plus")
	current := *original
	current.Extra = maps.Clone(original.Extra)
	cache := codexTurnStateCacheFromExtra(current.Extra)
	entry := cache["gpt-6-astra"]
	entry.State = poolTestState("plus", time.Now().Add(10*time.Second))
	cache["gpt-6-astra"] = entry
	current.Extra[CodexTurnStateProbeCacheExtraKey] = codexTurnStateCacheToExtra(cache)
	current.Extra[CodexTurnStateModeExtraKey] = CodexTurnStateModeLegacyTicket
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{}, nil)
	svc.accountRepo = &codexPoolAccountRepo{account: &current}
	frame := []byte(`{"type":"response.create","model":"gpt-6-astra"}`)
	out, _, err := svc.prepareCodexTurnStateWSFrame(context.Background(), original, CodexTurnStateModeStatePool, "tok", frame)
	require.NoError(t, err)
	require.Equal(t, entry.State, gjson.GetBytes(out, "client_metadata.x-codex-turn-state").String())
	current.Credentials = maps.Clone(original.Credentials)
	current.Credentials["chatgpt_account_id"] = "different-owner"
	_, _, err = svc.prepareCodexTurnStateWSFrame(context.Background(), original, CodexTurnStateModeStatePool, "tok", frame)
	require.ErrorIs(t, err, ErrCodexTurnStateUnavailable)
}

type codexPoolProxyRepo struct{ ProxyRepository }

func (codexPoolProxyRepo) GetByID(context.Context, int64) (*Proxy, error) {
	return &Proxy{ID: 7, Protocol: "http", Host: "proxy.example.test", Port: 8080, Status: StatusActive}, nil
}

func TestCodexTurnStateImportRequiresSuccessfulSameModelValidation(t *testing.T) {
	for _, scenario := range []string{"accepted", "wrong_model", "incomplete"} {
		t.Run(scenario, func(t *testing.T) {
			a := poolTestAccount("plus")
			a.Type = AccountTypeSetupToken
			repo := &codexPoolRuntimeRepo{codexPoolAccountRepo: codexPoolAccountRepo{account: a}}
			svc := NewCodexTurnStateProbeService(repo, codexPoolProxyRepo{}, nil, nil)
			t.Cleanup(svc.Stop)
			svc.settingRepo = &codexTicketLifecycleSettings{get: func(context.Context, string) (string, error) { return "7", nil }}
			state := poolTestState("plus", time.Now().Add(10*time.Second))
			svc.requestDo = func(req *http.Request, proxy string) (*http.Response, error) {
				require.True(t, req.Close)
				require.Equal(t, "http://proxy.example.test:8080", proxy)
				require.Equal(t, state, req.Header.Get(openAICodexTurnStateHeader))
				payload := `{"type":"response.completed","response":{"model":"gpt-6-astra"}}`
				if scenario == "wrong_model" {
					payload = `{"type":"response.completed","response":{"model":"gpt-5.6-luna"}}`
				}
				if scenario == "incomplete" {
					payload = `{"type":"response.created","response":{"model":"gpt-6-astra"}}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\n"))}, nil
			}
			_, err := svc.ImportState(context.Background(), a.ID, "gpt-6-astra", state)
			if scenario == "accepted" {
				require.NoError(t, err)
				require.Equal(t, state, codexTurnStateCacheFromExtra(repo.account.Extra)["gpt-6-astra"].State)
			} else {
				require.Error(t, err)
				require.Zero(t, repo.writes)
			}
		})
	}
}
