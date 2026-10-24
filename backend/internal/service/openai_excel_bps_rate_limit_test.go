package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func bpsRateLimitState(t *testing.T, s *OpenAIGatewayService, a *Account, model string) excelBPSRateLimitState {
	t.Helper()
	v, ok := s.excelBPSModelRateLimits.Load(excelBPSRateLimitKey{a.ID, model})
	require.True(t, ok)
	return v.(excelBPSRateLimitState)
}

func TestBPSModelRateLimitIsolationAndSwitch(t *testing.T) {
	s := &OpenAIGatewayService{}
	a := excelAccount()
	a.Credentials["model_mapping"] = map[string]any{"alias": "gpt-6.1-sol"}
	on := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	off := context.WithValue(context.Background(), excelBPSRecoveryKey{}, false)
	s.coolDownExcelBPSAttempt(on, a, "gpt-6.1-sol", nil, nil, nil, 429)
	require.True(t, s.isExcelBPSCoolingDown(on, a, "alias"))
	require.False(t, s.isExcelBPSCoolingDown(on, a, "gpt-6-astra"))
	other := *a
	other.ID++
	require.False(t, s.isExcelBPSCoolingDown(on, &other, "alias"))
	require.False(t, s.isExcelBPSCoolingDown(off, a, "alias"), "turning recovery off ignores its model backoff")
	native := context.WithValue(on, excelBPSSchedulingKey{}, false)
	require.False(t, s.isExcelBPSCoolingDown(native, a, "alias"))
	require.False(t, s.isOpenAIAccountRuntimeBlocked(a))
	require.True(t, a.IsSchedulable())
	a.Extra["openai_excel_bps"] = false
	require.False(t, s.isExcelBPSCoolingDown(on, a, "alias"))
}

func TestBPSModelRateLimitBackoffAndSuccess(t *testing.T) {
	s := &OpenAIGatewayService{}
	a := excelAccount()
	ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	model := "gpt-6.1-sol"
	for n, seconds := range []int{30, 60, 120, 240, 300, 300} {
		before := time.Now()
		s.coolDownExcelBPSAttempt(ctx, a, model, nil, nil, nil, 429)
		state := bpsRateLimitState(t, s, a, model)
		require.Equal(t, n+1, state.failures)
		require.WithinDuration(t, before.Add(time.Duration(seconds)*time.Second), state.until, time.Second)
	}
	s.resetExcelBPSModelRateLimit(a, model, time.Now().Add(-time.Hour))
	require.Equal(t, 6, bpsRateLimitState(t, s, a, model).failures, "old in-flight success cannot clear a newer failure")
	s.resetExcelBPSModelRateLimit(a, model, time.Now())
	require.False(t, s.isExcelBPSCoolingDown(ctx, a, model))
	s.coolDownExcelBPSAttempt(ctx, a, model, nil, nil, nil, 429)
	require.Equal(t, 1, bpsRateLimitState(t, s, a, model).failures)
	key := excelBPSRateLimitKey{a.ID, model}
	old := bpsRateLimitState(t, s, a, model)
	old.until = time.Now().Add(-excelBPSRateLimitStreakTTL - time.Minute)
	s.excelBPSModelRateLimits.Store(key, old)
	s.coolDownExcelBPSAttempt(ctx, a, model, nil, nil, nil, 429)
	require.Equal(t, 1, bpsRateLimitState(t, s, a, model).failures)
}

func TestBPSModelRateLimitHintsAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		hint    *basispoints.RateLimitHint
		headers http.Header
		seconds int
		source  string
	}{
		{"body seconds", &basispoints.RateLimitHint{ResetAfterSeconds: 120}, nil, 120, "upstream_reset_after"},
		{"body absolute", &basispoints.RateLimitHint{ResetAtUnix: time.Now().Unix() + 180}, nil, 180, "upstream_reset_at"},
		{"retry after", nil, http.Header{"Retry-After": []string{"90"}}, 90, "retry_after"},
		{"expired", &basispoints.RateLimitHint{ResetAtUnix: 1}, nil, 30, "exponential_backoff"},
		{"body longer than header", &basispoints.RateLimitHint{ResetAfterSeconds: 120}, http.Header{"Retry-After": []string{"90"}}, 120, "upstream_reset_after"},
		{"header longer than body", &basispoints.RateLimitHint{ResetAfterSeconds: 120}, http.Header{"Retry-After": []string{"180"}}, 180, "retry_after"},
		{"bounded", &basispoints.RateLimitHint{ResetAfterSeconds: 9223372036854775807}, nil, 7200, "upstream_reset_after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &OpenAIGatewayService{}
			a := excelAccount()
			ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
			before := time.Now()
			s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", tc.headers, nil, &basispoints.UpstreamFailure{Status: 429, Code: "rate_limit_exceeded", RateLimit: tc.hint}, 200)
			state := bpsRateLimitState(t, s, a, "gpt-6.1-sol")
			require.Equal(t, tc.source, state.source)
			require.WithinDuration(t, before.Add(time.Duration(tc.seconds)*time.Second), state.until, 2*time.Second)
			s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", http.Header{"Retry-After": []string{"1"}}, nil, &basispoints.UpstreamFailure{Status: 429}, 200)
			require.False(t, bpsRateLimitState(t, s, a, "gpt-6.1-sol").until.Before(state.until))
		})
	}
}

func TestBPSModelRateLimitConcurrentUpdates(t *testing.T) {
	s := &OpenAIGatewayService{}
	a := excelAccount()
	ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, nil, nil, 429) }()
	}
	wg.Wait()
	require.Equal(t, 12, bpsRateLimitState(t, s, a, "gpt-6.1-sol").failures)
}

func TestBPSRateLimitBudgetCountsDistinctAccounts(t *testing.T) {
	s := &OpenAIGatewayService{}
	route := &excelBPSFailoverRoute{model: "gpt-6.1-sol"}
	ctx := context.WithValue(context.WithValue(context.Background(), excelBPSRecoveryKey{}, true), excelBPSFailoverRouteKey{}, route)
	a := excelAccount()
	for _, id := range []int64{1, 1, 2, 3} {
		a.ID = id
		f := &UpstreamFailoverError{StatusCode: 429}
		s.limitExcelBPS429Failover(ctx, nil, a, "gpt-6.1-sol", f)
		require.Equal(t, id != 3, f.ShouldRetryNextAccount())
	}
	s.limitExcelBPS429Failover(ctx, nil, a, "gpt-6.1-sol", &UpstreamFailoverError{StatusCode: 503})
	f := &UpstreamFailoverError{StatusCode: 429}
	s.limitExcelBPS429Failover(ctx, nil, a, "gpt-6.1-sol", f)
	require.True(t, f.ShouldRetryNextAccount())
	off := context.WithValue(ctx, excelBPSRecoveryKey{}, false)
	for _, id := range []int64{4, 5, 6, 7} {
		a.ID = id
		f := &UpstreamFailoverError{StatusCode: 429}
		s.limitExcelBPS429Failover(off, nil, a, "gpt-6.1-sol", f)
		require.True(t, f.ShouldRetryNextAccount())
	}
}

func TestBPSRateLimitSSETimingReachesGateway(t *testing.T) {
	for _, stream := range []bool{true, false} {
		upstream := &httpUpstreamRecorder{resp: bpsCompletionResponse(200, "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_limit\",\"status\":\"failed\",\"output\":[],\"error\":{\"code\":\"rate_limit_exceeded\",\"message\":\"PRIVATE\",\"resets_in_seconds\":120}}}\n\n")}
		svc := openAIExcelBPSRecoveryTestService(upstream)
		a := excelAccount()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		body := []byte(`{"model":"gpt-6.1-sol","input":"hello","stream":false}`)
		if stream {
			body = []byte(`{"model":"gpt-6.1-sol","input":"hello","stream":true}`)
		}
		_, err := svc.Forward(context.Background(), c, a, body)
		var failure *UpstreamFailoverError
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ShouldRetryNextAccount())
		require.Empty(t, rec.Body.String())
		state := bpsRateLimitState(t, svc, a, "gpt-6.1-sol")
		require.Equal(t, "upstream_reset_after", state.source)
		require.WithinDuration(t, time.Now().Add(120*time.Second), state.until, 2*time.Second)
		require.False(t, svc.isExcelBPSRateLimited(a, "gpt-6-astra"))
		require.False(t, svc.isOpenAIAccountRuntimeBlocked(a))
	}
}
