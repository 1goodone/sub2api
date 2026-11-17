//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type recoveryQuotaUpstream struct {
	oauthHistoryRecoveryUpstream
	mode     string
	onReject func()
}

func (u *recoveryQuotaUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	response, err := u.oauthHistoryRecoveryUpstream.Do(req, proxy, id, concurrency)
	if err != nil || id == 1 || (id == 3 && u.mode != "exhausted" && u.mode != "heartbeat_exhausted") {
		return response, err
	}
	_ = response.Body.Close()
	if u.onReject != nil {
		u.onReject()
	}
	if u.mode == "disconnect" {
		return nil, io.ErrUnexpectedEOF
	}
	status, contentType := 429, "application/json"
	headers := http.Header{}
	body := `{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","resets_in_seconds":666}}`
	switch u.mode {
	case "headers_only":
		body = `{ "error": {"type":"rate_limit_exceeded"}}`
		headers.Set("X-Codex-Primary-Used-Percent", "100")
		headers.Set("X-Codex-Primary-Window-Minutes", "300")
		headers.Set("X-Codex-Primary-Reset-After-Seconds", "666")
	case "transient":
		body = `{"error":{"type":"rate_limit_exceeded","message":"Too many requests"}}`
	case "bad_request":
		status, body = 400, `{"error":{"type":"invalid_request_error","message":"fixture rejected"}}`
	case "stream", "tool_shell", "tool_output":
		status, contentType = 200, "text/event-stream"
		body = "data: " + `{"type":"response.failed","response":{"error":{"type":"usage_limit_reached","message":"quota exhausted"}}}` + "\n\n"
		if u.mode == "tool_output" || u.mode == "tool_shell" {
			arguments := ""
			if u.mode == "tool_output" {
				arguments = "{}"
			}
			body = fmt.Sprintf("data: "+`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_new","call_id":"call_new","name":"record","arguments":%q}}`+"\n\n", arguments) + body
		}
	}
	headers.Set("Content-Type", contentType)
	return &http.Response{StatusCode: status, Header: headers, Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestOpenAI409QuotaRecovery(t *testing.T) {
	testOpenAI409QuotaRecovery(t, false)
}

func TestOpenAI409QuotaRecoveryRedis(t *testing.T) {
	testOpenAI409QuotaRecovery(t, true)
}

func testOpenAI409QuotaRecovery(t *testing.T, realRedis bool) {
	settings := service.NewSettingService(&firstConcurrencySettingsRepo{values: map[string]string{}}, &config.Config{})
	require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{OpenAI409OmitEncryptedHistoryEnabled: true}))
	t.Cleanup(func() { require.NoError(t, settings.UpdateSettings(context.Background(), &service.SystemSettings{})) })
	modes := []string{"success", "exhausted", "headers_only", "transient", "bad_request", "stream", "tool_shell", "tool_output", "disconnect", "budget", "heartbeat", "heartbeat_exhausted"}
	if realRedis {
		modes = []string{"success", "exhausted", "shadow_plain", "shadow_tools", "owner_failed", "owner_pending", "owner_conflict"}
	}
	for _, lite := range []bool{false, true} {
		for _, mode := range modes {
			t.Run(fmt.Sprintf("lite=%t/%s", lite, mode), func(t *testing.T) {
				accounts := issue715Accounts()
				third := accounts[1]
				third.ID, third.Priority = 3, 2
				accounts = append(accounts, third)
				upstream := &recoveryQuotaUpstream{mode: mode}
				h := newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, nil, settings, accounts...)
				var ownersBefore map[string]string
				var ownersAfter func() map[string]string
				seed := h
				if realRedis {
					clients := issue805Redis(t)
					seed = newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, repository.NewGatewayCache(clients()), settings, accounts...)
					h = newOpenAIResponsesFailoverTestHandlerWithSettingsAndCache(t, upstream, repository.NewGatewayCache(clients()), settings, accounts...)
					probe := clients()
					ownersAfter = func() map[string]string { return issue805Owners(t, probe) }
					upstream.onReject = func() {
						if !strings.HasPrefix(mode, "owner_") {
							return
						}
						for key, value := range ownersBefore {
							state := strings.TrimPrefix(mode, "owner_")
							if state == "pending" {
								state = "pending:other-request:" + value
							}
							require.NoError(t, probe.Set(context.Background(), key, state, time.Minute).Err())
						}
					}
				}
				if strings.HasPrefix(mode, "heartbeat") {
					h.cfg.Gateway.StreamKeepaliveInterval = 1
					upstream.onReject = func() { time.Sleep(1200 * time.Millisecond) }
				}
				if mode == "budget" {
					h.maxAccountSwitches = 0
				}
				request := func(node *OpenAIGatewayHandler, body string) (int, string) {
					if strings.HasPrefix(mode, "shadow_") {
						body = strings.ReplaceAll(body, `{"type":"web_search_call","id":"ws_quota","status":"completed"},`, "")
						if mode == "shadow_plain" {
							body = strings.ReplaceAll(body, `{"type":"function_call","name":"record","call_id":"call_done","arguments":"{}"},{"type":"function_call_output","call_id":"call_done","output":"already done"},`, "")
						}
					}
					c, rec := newIssue715Context(t, body)
					c.Request.Header.Set("session_id", "recovery-quota")
					if lite {
						c.Request.Header.Set("X-OpenAI-Internal-Codex-Responses-Lite", "true")
					}
					node.Responses(c)
					return rec.Code, rec.Body.String()
				}
				status, answer := request(seed, `{"model":"gpt-6.1-sol","stream":true,"input":[{"role":"user","content":"start"}]}`)
				require.Equal(t, 200, status, answer)
				require.Equal(t, []int64{1}, upstream.ids)
				if realRedis {
					ownersBefore = ownersAfter()
					require.Len(t, ownersBefore, 1)
					for _, value := range ownersBefore {
						require.True(t, issue805CompletedOwner(value, 1))
					}
				}
				accounts[0].Schedulable = false
				status, answer = request(h, `{"model":"gpt-6.1-sol","stream":true,"instructions":"Keep completed work","input":[`+issue715Agent+`,{"type":"web_search_call","id":"ws_quota","status":"completed"},{"type":"function_call","name":"record","call_id":"call_done","arguments":"{}"},{"type":"function_call_output","call_id":"call_done","output":"already done"},{"role":"user","content":"continue"}],"tools":[{"type":"function","name":"record","parameters":{"type":"object"}}],"tool_choice":"auto"}`)
				if mode == "success" || mode == "exhausted" || mode == "headers_only" || strings.HasPrefix(mode, "heartbeat") || strings.HasPrefix(mode, "shadow_") {
					require.Equal(t, []int64{1, 2, 3}, upstream.ids, answer)
					for _, field := range []string{"input", "tools", "tool_choice", "instructions"} {
						require.JSONEq(t, gjson.GetBytes(upstream.bodies[1], field).Raw, gjson.GetBytes(upstream.bodies[2], field).Raw, field)
					}
				} else {
					require.Equal(t, []int64{1, 2}, upstream.ids, answer)
				}
				switch mode {
				case "success", "headers_only", "heartbeat", "shadow_plain", "shadow_tools":
					require.Equal(t, 200, status, answer)
					require.Contains(t, answer, "continued")
					require.NotContains(t, answer, "response.failed")
				case "exhausted", "budget", "stream", "tool_shell":
					require.Equal(t, 503, status, answer)
					require.Contains(t, answer, "server_error")
					require.NotContains(t, answer, "rate_limit_error")
				case "transient":
					require.Equal(t, 429, status, answer)
				case "bad_request":
					require.Equal(t, 400, status, answer)
				case "tool_output":
					require.Equal(t, 200, status, answer)
					require.Contains(t, answer, "response.failed")
					require.Contains(t, answer, "call_new")
					require.NotContains(t, answer, "continued")
				case "disconnect":
					require.Equal(t, 502, status, answer)
					require.NotContains(t, answer, "continued")
				case "heartbeat_exhausted":
					require.Equal(t, 200, status, answer)
					require.Equal(t, 1, strings.Count(answer, "event: response.failed"), answer)
					require.Contains(t, answer, `"code":"server_error"`)
					require.NotContains(t, answer, "rate_limit_error")
				case "owner_failed", "owner_pending", "owner_conflict":
					require.Equal(t, 409, status, answer)
				}
				if strings.HasPrefix(mode, "heartbeat") {
					require.Contains(t, answer, ": ", "must exercise a real header-wait heartbeat")
				}
				if realRedis {
					after := ownersAfter()
					require.Len(t, after, len(ownersBefore), "refused attempt must not publish an owner")
					for key, value := range ownersBefore {
						if strings.HasPrefix(mode, "owner_") {
							value = strings.TrimPrefix(mode, "owner_")
							if value == "pending" {
								value = "pending:other-request:" + ownersBefore[key]
							}
						}
						require.Equal(t, value, after[key], "must preserve the original owner or its newer fence")
					}
				}
			})
		}
	}
}
