package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type excelBPSPolicyCounterStub struct {
	openAI403CounterResetStub
	counts        map[int64]int64
	increments    int
	resets        []int64
	windowMinutes int
	err           error
}

func (s *excelBPSPolicyCounterStub) IncrementExcelBPSPolicy403Count(_ context.Context, id int64, windowMinutes int) (int64, error) {
	s.increments++
	s.windowMinutes = windowMinutes
	if s.err != nil {
		return 0, s.err
	}
	s.counts[id]++
	return s.counts[id], nil
}

func (s *excelBPSPolicyCounterStub) ResetExcelBPSPolicy403Count(_ context.Context, id int64) error {
	if s.err != nil {
		return s.err
	}
	s.resets = append(s.resets, id)
	delete(s.counts, id)
	return nil
}

func attachExcelBPSPolicyCounter(svc *OpenAIGatewayService) *excelBPSPolicyCounterStub {
	counter := &excelBPSPolicyCounterStub{counts: make(map[int64]int64)}
	if svc.rateLimitService == nil {
		svc.rateLimitService = NewRateLimitService(nil, nil, nil, nil, nil)
	}
	svc.rateLimitService.SetOpenAI403CounterCache(counter)
	return counter
}

func forwardExcelBPSPolicyTest(svc *OpenAIGatewayService, account *Account, stream bool) (*httptest.ResponseRecorder, error) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
	body := fmt.Sprintf(`{"model":"gpt-6-astra","stream":%t,"input":"hello"}`, stream)
	_, err := svc.Forward(context.Background(), c, account, []byte(body))
	return rec, err
}

func TestExcelBPSPolicyCounterDisablesOnlyBPSOnThirdRejection(t *testing.T) {
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
		bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
		bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
	}}
	svc := openAIExcelBPSRecoveryTestService(upstream)
	counter := attachExcelBPSPolicyCounter(svc)
	account := excelAccount()
	account.Extra["openai_excel_bps_auto_disable_on_403"] = true
	account.Extra[ExcelBPSAutoMoveOn403Key] = true
	account.Extra[ExcelBPS403TargetGroupIDKey] = 0
	disableCalls, moveCalls := 0, 0
	svc.accountRepo = &excelBPSGroupActionRepo{
		excelBPSAutoDisableRepo: excelBPSAutoDisableRepo{disable: func(ctx context.Context, got *Account) (bool, error) {
			require.NoError(t, ctx.Err())
			require.Same(t, account, got)
			disableCalls++
			return true, nil
		}},
		move: func(context.Context, *Account) (bool, error) { moveCalls++; return true, nil },
	}
	for attempt := 1; attempt <= 3; attempt++ {
		rec, err := forwardExcelBPSPolicyTest(svc, account, false)
		var failover *UpstreamFailoverError
		require.ErrorAs(t, err, &failover)
		require.Equal(t, 403, failover.StatusCode)
		require.True(t, failover.SuppressScheduleFailure)
		require.False(t, failover.RetryableOnSameAccount)
		require.Empty(t, rec.Body.String(), "threshold must not interrupt cross-account recovery")
		require.Equal(t, int64(attempt), counter.counts[account.ID])
		require.Equal(t, attempt/3, disableCalls)
	}
	require.Equal(t, 180, counter.windowMinutes)
	require.Zero(t, moveCalls)
	require.Empty(t, counter.resets)
	require.Empty(t, counter.resetCalls, "do not reset the native counter")
	require.Equal(t, StatusActive, account.Status)
	require.True(t, account.Schedulable)
	require.True(t, account.IsExcelBPSEnabled(), "do not mutate a shared scheduler snapshot")
}

func TestExcelBPSPolicyCounterSuccessResetsOnlyCompletedAccount(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := "data: " + `{"type":"response.completed","response":{"id":"resp_policy_reset","status":"completed","model":"gpt-6-astra","output":[]}}` + "\n\n"
			upstream := &httpUpstreamRecorder{responses: []*http.Response{
				bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
				bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
				bpsCompletionResponse(200, wire),
				bpsCompletionResponse(403, excelBPSUsagePolicyMessage),
			}}
			svc := openAIExcelBPSRecoveryTestService(upstream)
			counter := attachExcelBPSPolicyCounter(svc)
			account := excelAccount()
			account.Extra["openai_excel_bps_auto_disable_on_403"] = true
			counter.counts[account.ID+1] = 2
			disableCalls := 0
			svc.accountRepo = &excelBPSAutoDisableRepo{disable: func(context.Context, *Account) (bool, error) { disableCalls++; return true, nil }}
			for attempt := 0; attempt < 4; attempt++ {
				_, err := forwardExcelBPSPolicyTest(svc, account, stream)
				if attempt == 2 {
					require.NoError(t, err)
					require.Zero(t, counter.counts[account.ID])
				} else {
					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
				}
			}
			require.Equal(t, int64(1), counter.counts[account.ID])
			require.Equal(t, int64(2), counter.counts[account.ID+1])
			require.Equal(t, []int64{account.ID}, counter.resets)
			require.Empty(t, counter.resetCalls)
			require.Zero(t, disableCalls)
		})
	}
}

func TestExcelBPSPolicyCounterIncompleteResponseDoesNotReset(t *testing.T) {
	for name, wire := range map[string]string{
		"failed":     "data: " + `{"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"unavailable"}}}` + "\n\n",
		"incomplete": "data: " + `{"type":"response.created","response":{"id":"unfinished"}}` + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			svc := openAIExcelBPSRecoveryTestService(&httpUpstreamRecorder{resp: bpsCompletionResponse(200, wire)})
			counter := attachExcelBPSPolicyCounter(svc)
			account := excelAccount()
			account.Extra["openai_excel_bps_auto_disable_on_403"] = true
			counter.counts[account.ID] = 2
			_, err := forwardExcelBPSPolicyTest(svc, account, false)
			require.Error(t, err)
			require.Equal(t, int64(2), counter.counts[account.ID])
			require.Empty(t, counter.resets)
			require.Zero(t, counter.increments)
		})
	}
}

func TestExcelBPSPolicyCounterHonorsScopeAndObservation(t *testing.T) {
	for _, mode := range []string{"recovery off", "account opt-in off", "native account", "observation", "experiment", "cancelled", "no cache", "legacy cache", "cache failure"} {
		t.Run(mode, func(t *testing.T) {
			svc := openAIExcelBPSRecoveryTestService(nil)
			counter := attachExcelBPSPolicyCounter(svc)
			account := excelAccount()
			account.Extra["openai_excel_bps_auto_disable_on_403"] = true
			counter.counts[account.ID] = 2
			disableCalls := 0
			svc.accountRepo = &excelBPSAutoDisableRepo{disable: func(context.Context, *Account) (bool, error) { disableCalls++; return true, nil }}
			ctx := context.Background()
			switch mode {
			case "recovery off":
				svc.settingService = nil
			case "account opt-in off":
				account.Extra["openai_excel_bps_auto_disable_on_403"] = false
			case "native account":
				account.Extra["openai_excel_bps"] = false
			case "observation":
				ctx = withPelicanTestOptions(ctx, pelicanTestOptions{observeOnly: true})
			case "experiment":
				ctx = context.WithValue(ctx, controlledExperimentContextKey{}, &controlledExperimentMode{})
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "no cache":
				svc.rateLimitService.SetOpenAI403CounterCache(nil)
			case "legacy cache":
				svc.rateLimitService.SetOpenAI403CounterCache(&openAI403CounterResetStub{})
			case "cache failure":
				counter.err = errors.New("cache unavailable")
			}
			for i := 0; i < 3; i++ {
				svc.recordExcelBPSPolicyRejection(ctx, account)
			}
			svc.resetExcelBPSPolicyRejections(ctx, account)
			require.Zero(t, disableCalls)
			require.Equal(t, int64(2), counter.counts[account.ID])
			require.Empty(t, counter.resets)
			if mode != "cache failure" {
				require.Zero(t, counter.increments)
			}
		})
	}
}

func TestExcelBPSPolicyCounterCacheFailureStillAllowsFailover(t *testing.T) {
	svc := openAIExcelBPSRecoveryTestService(&httpUpstreamRecorder{resp: bpsCompletionResponse(403, excelBPSUsagePolicyMessage)})
	counter := attachExcelBPSPolicyCounter(svc)
	counter.err = errors.New("cache unavailable")
	account := excelAccount()
	account.Extra["openai_excel_bps_auto_disable_on_403"] = true
	svc.accountRepo = &excelBPSAutoDisableRepo{disable: func(context.Context, *Account) (bool, error) {
		t.Fatal("cache failure must not disable BPS")
		return false, nil
	}}
	rec, err := forwardExcelBPSPolicyTest(svc, account, false)
	var failover *UpstreamFailoverError
	require.ErrorAs(t, err, &failover)
	require.Empty(t, rec.Body.String())
	require.Equal(t, 1, counter.increments)
}
