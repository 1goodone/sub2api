//go:build unit

package handler

import (
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

type excelBPSRecoveryUpstream struct {
	service.HTTPUpstream
	ids       []int64
	status    int
	wire      string
	allFailed bool
}

func (u *excelBPSRecoveryUpstream) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	u.ids = append(u.ids, id)
	if req.URL.Path != "/basispoints/api/responses" {
		return nil, fmt.Errorf("unexpected native fallback: %s", req.URL.Path)
	}
	if id == 1 || u.allFailed {
		return &http.Response{StatusCode: u.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(u.wire))}, nil
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("data: " + `{"type":"response.completed","response":{"id":"recovered","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"RECOVERED"}]}]}}` + "\n\n"))}, nil
}

func bpsRecoveryFailure(code string) string {
	return "data: " + fmt.Sprintf(`{"type":"response.failed","response":{"id":"failed_attempt","status":"failed","output":[],"error":{"code":%q,"message":"PRIVATE_ERROR"}}}`, code) + "\n\n"
}

func TestExcelBPSRecoversBeforeClientOutput(t *testing.T) {
	preamble := "data: " + `{"type":"response.created","response":{"id":"failed_attempt","status":"in_progress","output":[]}}` + "\n\n"
	for _, tc := range []struct {
		name   string
		status int
		wire   string
	}{
		{"model permission", 403, `{"error":{"code":"model_not_allowed"}}`},
		{"model unavailable", 404, `{"error":{"code":"model_not_found"}}`},
		{"revoked credential", 401, `{"error":{"code":"token_revoked"}}`},
		{"server error", 500, `{"error":{"type":"server_error"}}`},
		{"in-band rate limit", 200, preamble + bpsRecoveryFailure("rate_limit_exceeded")},
		{"in-band server error", 200, preamble + bpsRecoveryFailure("server_error")},
		{"in-band model permission", 200, preamble + bpsRecoveryFailure("model_not_allowed")},
		{"in-band authentication", 200, preamble + bpsRecoveryFailure("invalid_api_key")},
		{"preoutput EOF", 200, preamble},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				u := &excelBPSRecoveryUpstream{status: tc.status, wire: tc.wire}
				h := newOpenAIResponsesFailoverTestHandler(t, u, enableExcelBPSFailoverAccounts)
				c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-6-astra","input":"hello","stream":%t}`, stream)))
				h.Responses(c)
				require.Equal(t, http.StatusOK, rec.Code, "accounts=%v body=%s", u.ids, rec.Body.String())
				require.Contains(t, rec.Body.String(), "RECOVERED")
				require.NotContains(t, rec.Body.String(), "failed_attempt")
				require.NotContains(t, rec.Body.String(), "PRIVATE_ERROR")
				require.NotContains(t, rec.Body.String(), "response.failed")
				require.Equal(t, int64(2), u.ids[len(u.ids)-1])
				require.LessOrEqual(t, len(u.ids), 3)
				if tc.name == "revoked credential" || tc.name == "in-band authentication" {
					// A later request must continue skipping the paused account.
					next, nextRecorder := newOpenAIResponsesFailoverTestContext(t, nil)
					next.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"next request"}`))
					h.Responses(next)
					require.Equal(t, http.StatusOK, nextRecorder.Code)
					require.Equal(t, []int64{1, 2, 2}, u.ids)
				}
			})
		}
	}
}

func TestExcelBPSRecoveryPreservesRouteAndModel(t *testing.T) {
	u := &excelBPSRecoveryUpstream{status: 403, wire: `{"error":{"code":"model_not_allowed"}}`}
	h := newOpenAIResponsesFailoverTestHandlerWithAccountCount(t, u, 4, func(accounts []service.Account) {
		enableExcelBPSFailoverAccounts(accounts)
		accounts[1].Extra["openai_excel_bps"] = false
		accounts[2].Credentials["model_mapping"] = map[string]any{"gpt-6-astra": "gpt-6-luna"}
	})
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello"}`))
	h.Responses(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, []int64{1, 4}, u.ids)
}

func TestExcelBPSDoesNotReplayDeliveredOutputOrPolicyFailures(t *testing.T) {
	text := "data: " + `{"type":"response.output_text.delta","delta":"DELIVERED"}` + "\n\n"
	for _, wire := range []string{text + bpsRecoveryFailure("rate_limit_exceeded"), text, bpsRecoveryFailure("cyber_policy"), bpsRecoveryFailure("invalid_prompt"), bpsRecoveryFailure("UNKNOWN_CODE")} {
		u := &excelBPSRecoveryUpstream{status: 200, wire: wire}
		h := newOpenAIResponsesFailoverTestHandler(t, u, enableExcelBPSFailoverAccounts)
		c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
		c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello","stream":true}`))
		h.Responses(c)
		require.Equal(t, []int64{1}, u.ids, rec.Body.String())
		require.NotContains(t, rec.Body.String(), "RECOVERED")
	}
}

func TestExcelBPSRecoveryExhaustionIsBounded(t *testing.T) {
	u := &excelBPSRecoveryUpstream{status: 500, wire: `{"error":{"type":"server_error","message":"PRIVATE_ERROR"}}`, allFailed: true}
	h := newOpenAIResponsesFailoverTestHandler(t, u, enableExcelBPSFailoverAccounts)
	c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello"}`))
	h.Responses(c)
	require.LessOrEqual(t, len(u.ids), 4)
	require.GreaterOrEqual(t, rec.Code, 500)
	require.NotEmpty(t, gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
	require.NotContains(t, rec.Body.String(), "PRIVATE_ERROR")
}
