//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type excelBPSFailoverUpstream struct {
	service.HTTPUpstream
	ids        []int64
	bodies     [][]byte
	allLimited bool
	cancel     context.CancelFunc
}

func (u *excelBPSFailoverUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, id)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	u.bodies = append(u.bodies, body)
	if u.cancel != nil {
		u.cancel()
	}
	if id == 1 || u.allLimited {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader("PRIVATE_UPSTREAM"))}, nil
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_bps_failover\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"success\"}]}],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"))}, nil
}

func enableExcelBPSFailoverAccounts(accounts []service.Account) {
	for i := range accounts {
		accounts[i].Extra = map[string]any{"openai_excel_bps": true, "openai_passthrough": true}
		accounts[i].Credentials["chatgpt_account_id"] = fmt.Sprintf("bps-account-%d", accounts[i].ID)
	}
}

func TestExcelBPS429HandlerSwitchesAndSkipsCoolingAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			upstream := &excelBPSFailoverUpstream{}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, enableExcelBPSFailoverAccounts)
			for attempt := 0; attempt < 2; attempt++ {
				c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
				body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"tools":[{"type":"function","name":"shell"}],"input":[{"role":"user","content":"inspect"},{"type":"function_call","name":"shell","call_id":"call_history","arguments":"{}"},{"type":"function_call_output","call_id":"call_history","output":"project"}]}`, stream)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(body))
				h.Responses(c)
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), "success")
				require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
				require.NotContains(t, rec.Body.String(), "basispoints_rate_limited")
			}
			require.Equal(t, []int64{1, 2, 2}, upstream.ids)
			for _, body := range upstream.bodies {
				require.Contains(t, gjson.GetBytes(body, "input").Raw, "call_history")
				require.Contains(t, gjson.GetBytes(body, "input").Raw, "project")
			}
		})
	}
}

func TestExcelBPS429HandlerStopsWhenExhaustedOrCanceled(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprint(canceled), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := &excelBPSFailoverUpstream{allLimited: true}
			if canceled {
				upstream.cancel = cancel
			}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, enableExcelBPSFailoverAccounts)
			c, rec := newOpenAIResponsesFailoverTestContext(t, ctx)
			c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader("{\"model\":\"gpt-6-astra\",\"input\":\"hello\"}")).WithContext(ctx)
			h.Responses(c)
			if canceled {
				require.Equal(t, []int64{1}, upstream.ids)
				require.Equal(t, statusClientClosedRequest, c.Writer.Status())
				require.Empty(t, rec.Body.String())
			} else {
				require.Equal(t, []int64{1, 2}, upstream.ids)
				require.Equal(t, http.StatusTooManyRequests, rec.Code)
				require.Equal(t, "60", rec.Header().Get("Retry-After"))
				require.NotContains(t, rec.Body.String(), "PRIVATE_UPSTREAM")
			}
		})
	}
}
