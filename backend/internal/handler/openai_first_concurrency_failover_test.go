//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type firstConcurrencySettingsRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *firstConcurrencySettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	return r.values[key], nil
}
func (r *firstConcurrencySettingsRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return r.values, nil
}
func (r *firstConcurrencySettingsRepo) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r *firstConcurrencySettingsRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.values = values
	return nil
}

type firstConcurrencyUpstream struct {
	service.HTTPUpstream
	ids        []int64
	priorEvent bool
}

func (u *firstConcurrencyUpstream) Do(_ *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, id)
	body := "data: " + `{"type":"response.completed","response":{"id":"resp_success","status":"completed","model":"gpt-5.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"local OAuth succeeded"}]}],"usage":{"input_tokens":5,"output_tokens":3}}}` + "\n\n"
	if id == 1 {
		body = "event: response.failed\ndata: " + `{"type":"response.failed","response":{"id":"resp_rejected","status":"failed","error":{"code":"gateway_concurrency_limit","message":"Concurrency limit exceeded for account, please retry later"}},"sequence_number":0}` + "\n\n"
		if u.priorEvent {
			body = "data: " + `{"type":"response.created","response":{"id":"resp_rejected"}}` + "\n\n" + body
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestRelayFirstConcurrencySwitchesToLocalOAuth(t *testing.T) {
	settings := service.NewSettingService(&firstConcurrencySettingsRepo{values: map[string]string{}}, &config.Config{})
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{StreamFirstResponseEnabled: true}))
	t.Cleanup(func() {
		require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{}))
	})
	for _, passthrough := range []bool{false, true} {
		for _, priorEvent := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%t/prior_event=%t", passthrough, priorEvent), func(t *testing.T) {
				upstream := &firstConcurrencyUpstream{priorEvent: priorEvent}
				accounts := issue715Accounts()
				accounts[0].Type = service.AccountTypeAPIKey
				accounts[0].Credentials = map[string]any{"api_key": "fixture-relay-key", "base_url": "https://relay.example.invalid"}
				accounts[0].Extra = map[string]any{"openai_passthrough": passthrough}
				h := newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, nil, settings, accounts...)
				c, rec := newIssue715Context(t, `{"model":"gpt-5.1","stream":true,"input":[{"type":"message","role":"user","content":"Say hello"}]}`)
				h.Responses(c)
				if priorEvent {
					require.Equal(t, []int64{1}, upstream.ids)
					require.Contains(t, rec.Body.String(), "gateway_concurrency_limit")
				} else {
					require.Equal(t, []int64{1, 2}, upstream.ids, rec.Body.String())
					require.Equal(t, http.StatusOK, rec.Code)
					require.Contains(t, rec.Body.String(), "local OAuth succeeded")
					require.NotContains(t, rec.Body.String(), "gateway_concurrency_limit")
				}
			})
		}
	}
}
