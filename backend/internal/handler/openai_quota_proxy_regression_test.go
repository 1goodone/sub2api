//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaFirstEventSwitchesAccounts(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		settings := service.NewSettingService(&firstConcurrencySettingsRepo{values: map[string]string{}}, &config.Config{})
		require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{StreamFirstResponseEnabled: enabled}))
		t.Cleanup(func() { require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{})) })
		for _, passthrough := range []bool{false, true} {
			for _, healthy := range []bool{false, true} {
				t.Run(fmt.Sprintf("first_response=%t/passthrough=%t/healthy=%t", enabled, passthrough, healthy), func(t *testing.T) {
					upstream := &poolCapacityTransport{reply: func(_ *http.Request, id int64) *http.Response {
						if healthy && id == 2 {
							return poolCapacityResponse(200, "text/event-stream", "data: {\"type\":\"response.completed\",\"response\":"+poolCapacityCompleted+"}\n\n", "")
						}
						return poolCapacityResponse(200, "text/event-stream", "event: response.failed\ndata: "+`{"type":"response.failed","response":{"id":"resp_quota","status":"failed","error":{"type":"usage_limit_reached","code":"usage_limit_reached","message":"The usage limit has been reached","resets_in_seconds":666}}}`+"\n\n", "")
					}}
					accounts := issue715Accounts()
					for i := range accounts {
						accounts[i].Extra = map[string]any{"openai_oauth_passthrough": passthrough}
					}
					h := newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, nil, settings, accounts...)
					c, rec := newIssue715Context(t, `{"model":"gpt-5.1","stream":true,"input":[{"type":"message","role":"user","content":"hello"}]}`)
					ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
					defer cancel()
					c.Request = c.Request.WithContext(ctx)
					h.Responses(c)
					t.Logf("hits=%v status=%d body=%s", upstream.hits, rec.Code, rec.Body.String())
					require.Equal(t, []int64{1, 2}, upstream.hits, "the first quota rejection is not committed output")
					if healthy {
						require.Equal(t, http.StatusOK, rec.Code)
						require.Contains(t, rec.Body.String(), "response.completed")
					} else {
						require.Equal(t, http.StatusServiceUnavailable, rec.Code)
					}
				})
			}
		}
	}
}

func TestOpenAIRequestProxyFailureTriesHealthyAccount(t *testing.T) {
	for _, entry := range []string{"responses", "chat", "messages"} {
		for _, healthy := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/healthy=%t", entry, healthy), func(t *testing.T) {
				accounts := issue715Accounts()
				accounts[0].Extra = map[string]any{"openai_proxy_mode": "request", "openai_proxy_ids": []int64{987654321}}
				if !healthy {
					accounts[1].Extra = accounts[0].Extra
				}
				upstream := &poolCapacityTransport{reply: func(_ *http.Request, id int64) *http.Response {
					return poolCapacityResponse(200, "text/event-stream", "data: {\"type\":\"response.completed\",\"response\":"+poolCapacityCompleted+"}\n\n", "")
				}}
				h := newOpenAIResponsesFailoverTestHandlerWithCache(t, upstream, nil, accounts...)
				c, rec := newIssue715Context(t, `{"model":"gpt-5.1","stream":false,"input":"hello","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`)
				key, _ := middleware2.GetAPIKeyFromContext(c)
				key.Group.AllowMessagesDispatch = true
				switch entry {
				case "responses":
					h.Responses(c)
				case "chat":
					h.ChatCompletions(c)
				case "messages":
					h.Messages(c)
				}
				t.Logf("hits=%v status=%d body=%s", upstream.hits, rec.Code, rec.Body.String())
				if healthy {
					require.Equal(t, []int64{2}, upstream.hits)
					require.Equal(t, http.StatusOK, rec.Code)
				} else {
					require.Empty(t, upstream.hits)
					require.Equal(t, http.StatusServiceUnavailable, rec.Code)
				}
				require.True(t, json.Valid(rec.Body.Bytes()), "exactly one JSON response, no appended SSE: %s", rec.Body.String())
			})
		}
	}
}
