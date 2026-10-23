//go:build unit

package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type excelBPSUsagePolicyRepairUpstream struct {
	*excelBPSRecoveryUpstream
	firstWire string
}

func (u *excelBPSUsagePolicyRepairUpstream) Do(req *http.Request, proxy string, id int64, concurrency int) (*http.Response, error) {
	if len(u.ids) == 0 {
		u.ids = append(u.ids, id)
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(u.firstWire))}, nil
	}
	return u.excelBPSRecoveryUpstream.Do(req, proxy, id, concurrency)
}

func TestExcelBPSUsagePolicyCorrectionSwitchesAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			args, err := json.Marshal(map[string]any{"summary": "Run", "code": "text(42);", "extended_summary": "{}", "destructive": false, "references": []any{}})
			require.NoError(t, err)
			event, err := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{
				"id": "failed_attempt", "status": "completed", "model": "gpt-6-astra",
				"output": []any{map[string]any{"type": "function_call", "name": "run_officejs", "id": "fc_failed", "call_id": "call_failed", "arguments": string(args), "status": "completed"}},
			}})
			require.NoError(t, err)
			u := &excelBPSUsagePolicyRepairUpstream{
				excelBPSRecoveryUpstream: &excelBPSRecoveryUpstream{status: 403, wire: "403: This request was blocked by our usage policy."},
				firstWire:                "data: " + string(event) + "\n\n",
			}
			h := newOpenAIResponsesFailoverTestHandlerWithSettings(t, u, 2, newExcelBPSRecoverySettings(true), enableExcelBPSFailoverAccounts)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"hello","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]}`, stream)
			c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(body))
			h.Responses(c)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.Equal(t, []int64{1, 1, 2}, u.ids, "after the correction is rejected, switch rather than replay account 1")
			require.Contains(t, rec.Body.String(), "RECOVERED")
			require.NotContains(t, rec.Body.String(), "failed_attempt")
			require.NotContains(t, rec.Body.String(), "usage policy")
		})
	}
}

func TestExcelBPSUsagePolicyRecoveryKeepsAccountsAvailable(t *testing.T) {
	u := &excelBPSRecoveryUpstream{status: 403, wire: `{"error":{"message":"403: This request was blocked by our usage policy."}}`}
	h := newOpenAIResponsesFailoverTestHandlerWithSettings(t, u, 4, newExcelBPSRecoverySettings(true), func(accounts []service.Account) {
		enableExcelBPSFailoverAccounts(accounts)
		accounts[1].Extra["openai_excel_bps"] = false
		accounts[2].Credentials["model_mapping"] = map[string]any{"gpt-6-astra": "gpt-6-luna"}
	})
	for attempt := 0; attempt < 2; attempt++ {
		c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
		c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(`{"model":"gpt-6-astra","input":"hello"}`))
		h.Responses(c)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "RECOVERED")
		require.NotContains(t, rec.Body.String(), "usage policy")
	}
	// Skip the failed account only for this request; do not pause it or its model.
	// Account 2 is native-only and account 3 maps to another model, so neither is tried.
	require.Equal(t, []int64{1, 4, 1, 4}, u.ids)
}

func TestExcelBPSUsagePolicyRecoveryExhaustionIsBounded(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			u := &excelBPSRecoveryUpstream{status: 403, wire: "403: This request was blocked by our usage policy.", allFailed: true}
			h := newOpenAIResponsesFailoverTestHandlerWithSettings(t, u, 5, newExcelBPSRecoverySettings(true), enableExcelBPSFailoverAccounts)
			h.maxAccountSwitches = 2
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			c.Request = httptest.NewRequest(http.MethodPost, "/responses", strings.NewReader(fmt.Sprintf(`{"model":"gpt-6-astra","input":"hello","stream":%t}`, stream)))
			h.Responses(c)
			require.Equal(t, []int64{1, 2, 3}, u.ids, "two switches, with no retry on the rejected account")
			require.GreaterOrEqual(t, rec.Code, 400)
			require.NotEmpty(t, rec.Body.String())
			require.NotContains(t, rec.Body.String(), "usage policy")
		})
	}
}
