package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSUsagePolicyRecoveryDoesNotMutateAccount(t *testing.T) {
	for _, raw := range []string{
		`{"error":{"message":"403: This request was blocked by our usage policy."}}`,
		"403: This request was blocked by our usage policy.",
		`{"error":{"code":"content_policy_violation"}}`,
	} {
		svc := openAIExcelBPSRecoveryTestService(&httpUpstreamRecorder{resp: bpsCompletionResponse(403, raw)})
		disableCalls, moveCalls := 0, 0
		svc.accountRepo = &excelBPSGroupActionRepo{
			excelBPSAutoDisableRepo: excelBPSAutoDisableRepo{disable: func(context.Context, *Account) (bool, error) { disableCalls++; return true, nil }},
			move:                    func(context.Context, *Account) (bool, error) { moveCalls++; return true, nil },
		}
		account := excelAccount()
		account.Extra["openai_excel_bps_auto_disable_on_403"] = true
		account.Extra[ExcelBPSAutoMoveOn403Key] = true
		account.Extra[ExcelBPS403TargetGroupIDKey] = 0
		svc.openaiAccountStats = newOpenAIAccountRuntimeStats()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
		_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-6-astra","input":"hello"}`))
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, 403, failover.StatusCode)
		require.False(t, failover.RetryableOnSameAccount)
		require.True(t, failover.SuppressScheduleFailure)
		require.Empty(t, rec.Body.String())
		require.False(t, IsResponseCommitted(c))
		require.Zero(t, disableCalls)
		require.Zero(t, moveCalls)
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
		require.False(t, svc.isExcelBPSModelCoolingDown(account, "gpt-6-astra"))
		require.False(t, svc.ReportOpenAIAccountScheduleResult(account, "gpt-6-astra", false, nil, err))
	}
}

func TestExcelBPSUsagePolicyCorrectionRecovery(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, unknownTool := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/unknownTool=%t", stream, unknownTool), func(t *testing.T) {
				code := "text(42);"
				tools := `[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]`
				if unknownTool {
					code = `{"name":"unknown","arguments":{}}`
					tools = `[{"type":"function","name":"shell"}]`
				}
				rejected := &excelBPSRepairBody{Reader: strings.NewReader(`{"error":{"message":"403: This request was blocked by our usage policy."}}`)}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{
					bpsCompletionResponse(200, excelBPSRepairWire(t, "policy", "Run", code)),
					{StatusCode: 403, Header: http.Header{}, Body: rejected},
				}}
				svc := openAIExcelBPSRecoveryTestService(upstream)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
				body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"hello","tools":%s}`, stream, tools)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(body))
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, 403, failover.StatusCode)
				require.False(t, failover.RetryableOnSameAccount)
				require.Len(t, upstream.requests, 2, "only the original send and its rejected correction")
				require.True(t, rejected.closed.Load())
				require.Empty(t, rec.Body.String())
			})
		}
	}
}

func TestExcelBPSUsagePolicyCorrectionDoesNotReplayVisibleOutput(t *testing.T) {
	wire := "data: " + `{"type":"response.output_text.delta","delta":"DELIVERED"}` + "\n\n" + excelBPSRepairWire(t, "policy", "Run", "text(42);")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		bpsCompletionResponse(200, wire),
		{StatusCode: 403, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("403: This request was blocked by our usage policy."))},
	}}
	svc := openAIExcelBPSRecoveryTestService(upstream)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	body := []byte(`{"model":"gpt-6-astra","stream":true,"input":"hello","tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]}`)
	_, err := svc.Forward(context.Background(), c, excelAccount(), body)
	require.Error(t, err)
	var failover *UpstreamFailoverError
	require.NotErrorAs(t, err, &failover)
	require.Contains(t, rec.Body.String(), "DELIVERED")
	require.LessOrEqual(t, len(upstream.requests), 2)
}
