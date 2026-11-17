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
	"github.com/tidwall/gjson"
)

// Exercise account recovery through the handler and the full native HTTP builder.
// The mock provider rejects unsupported fields instead of accepting every payload.
type oauthHistoryRecoveryUpstream struct {
	service.HTTPUpstream
	ids    []int64
	bodies [][]byte
}

func (u *oauthHistoryRecoveryUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, id)
	b, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, b)
	// Model the native provider contract, including for tools injected by the gateway.
	if gjson.GetBytes(b, `tools.#(type==web_search).external_web_access`).Exists() {
		return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"unsupported external_web_access"}}`))}, nil
	}
	output := `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"continued"}]}]`
	if id == 1 {
		output = "[" + issue715Agent + "]"
	}
	body := fmt.Sprintf("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_review_%d\",\"status\":\"completed\",\"model\":\"gpt-6.1-sol\",\"output\":%s,\"usage\":{\"input_tokens\":5,\"output_tokens\":3}}}\n\n", id, output)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestOpenAI409OAuthHistoryRecoveryNativeWire(t *testing.T) {
	settings := service.NewSettingService(&firstConcurrencySettingsRepo{values: map[string]string{}}, &config.Config{})
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{OpenAI409OmitEncryptedHistoryEnabled: true}))
	t.Cleanup(func() { require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{})) })

	for _, lite := range []bool{false, true} {
		for _, mode := range []string{"none", "auto", "required", "restricted", "string_auto", "string_required", "mixed_strings"} {
			t.Run(fmt.Sprintf("lite=%t/%s", lite, mode), func(t *testing.T) {
				accounts := issue715Accounts()
				upstream := &oauthHistoryRecoveryUpstream{}
				h := newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, nil, settings, accounts...)
				request := func(body string) (int, string) {
					c, rec := newIssue715Context(t, body)
					c.Request.Header.Set("session_id", "history-search")
					if lite {
						c.Request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
					}
					h.Responses(c)
					return rec.Code, rec.Body.String()
				}
				status, _ := request(`{"model":"gpt-6.1-sol","stream":true,"input":[{"role":"user","content":"start"}]}`)
				require.Equal(t, 200, status)
				require.Equal(t, []int64{1}, upstream.ids)
				accounts[0].Schedulable = false
				tools := `[]`
				choice := `"auto"`
				wantChoice := `"none"`
				if mode != "none" {
					tools = `[{"type":"function","name":"record","parameters":{"type":"object"}},{"type":"function","name":"verify","parameters":{"type":"object"}}]`
					choice = fmt.Sprintf("%q", mode)
					wantChoice = fmt.Sprintf(`{"type":"allowed_tools","mode":%q,"tools":[{"type":"function","name":"record"},{"type":"function","name":"verify"}]}`, mode)
					if mode == "restricted" {
						choice = `{"type":"allowed_tools","mode":"required","tools":[{"type":"function","name":"record"}]}`
						wantChoice = choice
					}
				}
				if strings.HasPrefix(mode, "string_") || mode == "mixed_strings" {
					selection := "required"
					if mode == "string_auto" {
						selection = "auto"
					}
					tools = `["edit"]`
					refs := `[{"type":"custom","name":"edit"}]`
					if mode == "mixed_strings" {
						tools = `["edit",{"type":"function","name":"verify","parameters":{"type":"object"}}]`
						refs = `[{"type":"custom","name":"edit"},{"type":"function","name":"verify"}]`
					}
					choice = fmt.Sprintf("%q", selection)
					wantChoice = fmt.Sprintf(`{"type":"allowed_tools","mode":%q,"tools":%s}`, selection, refs)
				}
				status, answer := request(`{"model":"gpt-6.1-sol","stream":true,"input":[` + issue715Agent + `,{"type":"web_search_call","id":"ws_history","status":"completed","action":{"type":"search","query":"fixture"}},{"role":"user","content":"continue the existing task"}],"tools":` + tools + `,"tool_choice":` + choice + `}`)
				require.Equal(t, 200, status, answer)
				require.Equal(t, []int64{1, 2}, upstream.ids, answer)
				require.Contains(t, answer, "continued")
				require.NotContains(t, answer, "response.failed")
				wire := upstream.bodies[1]
				if strings.HasPrefix(mode, "string_") || mode == "mixed_strings" {
					require.Equal(t, "edit", gjson.GetBytes(wire, "tools.0").String(), "preserve the Lite custom declaration")
				}
				require.JSONEq(t, wantChoice, gjson.GetBytes(wire, "tool_choice").Raw)
				search := gjson.GetBytes(wire, `tools.#(type==web_search)`)
				if lite {
					require.False(t, search.Exists())
					for _, item := range gjson.GetBytes(wire, "input").Array() {
						if item.Get("type").String() == "additional_tools" {
							if found := item.Get(`tools.#(type==web_search)`); found.Exists() {
								search = found
							}
						}
					}
				}
				require.True(t, search.Exists())
				require.False(t, search.Get("external_web_access").Exists())
			})
		}
	}
}
