package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestBPSRateLimitForwardBudgetAndLegacySwitch(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusTooManyRequests} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("http=%d/enabled=%t", status, enabled), func(t *testing.T) {
				wire := `{"error":{"code":"rate_limit_exceeded","message":"PRIVATE"}}`
				if status == http.StatusOK {
					wire = bpsFailureWire(t, "response.failed", "rate_limit_exceeded", "", 0)
				}
				upstream := &httpUpstreamRecorder{responses: []*http.Response{bpsCompletionResponse(status, wire), bpsCompletionResponse(status, wire), bpsCompletionResponse(status, wire)}}
				s := openAIExcelBPSRecoveryTestService(upstream)
				ctx := WithExcelBPSScheduling(context.WithValue(context.Background(), excelBPSRecoveryKey{}, enabled), nil)
				for i := 1; i <= 3; i++ {
					a := excelAccount()
					a.ID = int64(i)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
					_, err := s.Forward(ctx, c, a, []byte(`{"model":"gpt-6.1-sol","input":"hello","stream":true}`))
					var failure *UpstreamFailoverError
					if !enabled && status == http.StatusOK {
						require.NotErrorAs(t, err, &failure)
						require.NotEmpty(t, rec.Body.String())
						continue
					}
					require.ErrorAs(t, err, &failure)
					require.Equal(t, !enabled || i < 3, failure.ShouldRetryNextAccount())
					require.Empty(t, rec.Body.String())
				}
				require.Len(t, upstream.requests, 3)
			})
		}
	}
}

func TestBPSRateLimitHTTPResetRemainsModelScoped(t *testing.T) {
	s := &OpenAIGatewayService{}
	a := excelAccount()
	ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, []byte(`{"error":{"type":"rate_limit_exceeded","code":"rate_limit_exceeded","resets_in_seconds":120}}`), nil, 429)
	require.Equal(t, "model_limit_cooldown", bpsRateLimitState(t, s, a, "gpt-6.1-sol").source)
	require.False(t, s.isExcelBPSRateLimited(a, "gpt-6-astra"))
	require.False(t, s.isOpenAIAccountRuntimeBlocked(a))
}

func TestBPSRateLimitLogsAreStructuredAndCorrelated(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	ctx := logger.IntoContext(context.WithValue(context.Background(), excelBPSRecoveryKey{}, true), zap.New(core).With(zap.String("request_id", "request_fixture")))
	s := &OpenAIGatewayService{}
	a := excelAccount()
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, []byte(`{"error":{"message":"PRIVATE_REQUEST_OR_TOKEN","resets_in_seconds":90}}`), nil, 429)
	entries := logs.FilterMessage("openai.excel_bps_model_rate_limited").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "request_fixture", fields["request_id"])
	require.Equal(t, "bps_account_model", fields["scope"])
	require.Equal(t, int64(429), fields["upstream_http_status"])
	require.Equal(t, int64(600), fields["cooldown_seconds"])
	require.Equal(t, "model_limit_cooldown", fields["cooldown_source"])
	require.Equal(t, int64(90), fields["upstream_reset_after_seconds"])
	require.Equal(t, int64(1), fields["consecutive_failures"])
	require.NotContains(t, fmt.Sprint(fields), "PRIVATE_REQUEST_OR_TOKEN")
}

func TestBPSRateLimitOlderFailurePreservesNewerTimestamp(t *testing.T) {
	s := &OpenAIGatewayService{}
	a := excelAccount()
	ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	later := time.Now().Add(time.Second)
	s.excelBPSModelRateLimits.Store(excelBPSRateLimitKey{a.ID, "gpt-6.1-sol"}, excelBPSRateLimitState{until: later.Add(time.Minute), lastFailure: later, failures: 1, source: "model_limit_cooldown"})
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, nil, nil, 429)
	require.Equal(t, later, bpsRateLimitState(t, s, a, "gpt-6.1-sol").lastFailure)
}

func TestBPSRateLimitCompletedResponseRespectsOneDay(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, observation := range []bool{false, true} {
			upstream := &httpUpstreamRecorder{resp: bpsCompletionResponse(200, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"status\":\"completed\",\"model\":\"gpt-6.1-sol\",\"output\":[]}}\n\n")}
			s := openAIExcelBPSRecoveryTestService(upstream)
			a := excelAccount()
			key := excelBPSRateLimitKey{a.ID, "gpt-6.1-sol"}
			until := time.Now().Add(-time.Second)
			if active {
				until = time.Now().Add(24 * time.Hour)
			}
			s.excelBPSModelRateLimits.Store(key, excelBPSRateLimitState{until: until, lastFailure: time.Now().Add(-time.Minute), failures: 3})
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx := context.Background()
			if observation {
				ctx = withPelicanTestOptions(ctx, pelicanTestOptions{observeOnly: true})
			}
			_, err := s.Forward(ctx, c, a, []byte(`{"model":"gpt-6.1-sol","input":"hello","stream":true}`))
			require.NoError(t, err)
			_, exists := s.excelBPSModelRateLimits.Load(key)
			require.Equal(t, active || observation, exists, "success cannot lift an active restriction, and observation cannot clear business failure state")
		}
	}
}

func TestBPSRateLimitToolCorrectionDoesNotReplayAcceptedGeneration(t *testing.T) {
	for _, stream := range []bool{false, true} {
		upstream := &httpUpstreamRecorder{responses: []*http.Response{
			bpsCompletionResponse(200, excelBPSRepairWire(t, "limited_correction", "Run", "text(42);")),
			bpsCompletionResponse(200, "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"output\":[],\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"PRIVATE\",\"resets_in_seconds\":120}}}\n\n"),
		}}
		s := openAIExcelBPSRecoveryTestService(upstream)
		a := excelAccount()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","input":"test","stream":%t,"tools":[{"type":"namespace","name":"functions","tools":[{"type":"custom","name":"exec"}]}]}`, stream))
		_, err := s.Forward(context.Background(), c, a, body)
		var failover *UpstreamFailoverError
		require.Error(t, err)
		require.NotErrorAs(t, err, &failover)
		require.Len(t, upstream.requests, 2)
		require.NotContains(t, rec.Body.String(), "PRIVATE")
		require.Equal(t, "model_limit_cooldown", bpsRateLimitState(t, s, a, "gpt-5.6-sol").source)
		require.False(t, s.isOpenAIAccountRuntimeBlocked(a))
	}
}
