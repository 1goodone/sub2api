package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type excelBPSQuotaWrite struct {
	accountID int64
	updates   map[string]any
	resetAt   time.Time
	ctxErr    error
	deadline  time.Time
}

type excelBPSQuotaRepo struct {
	AccountRepository
	writes    chan excelBPSQuotaWrite
	updateErr error
	limitErr  error
}

type excelBPSQuotaSettingsRepo struct {
	SettingRepository
	cooldown string
}

func (r *excelBPSQuotaSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	if key == SettingKeyRateLimit429CooldownSettings {
		return r.cooldown, nil
	}
	return "", ErrSettingNotFound
}

func (r *excelBPSQuotaRepo) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	copied := make(map[string]any, len(updates))
	for key, value := range updates {
		copied[key] = value
	}
	deadline, _ := ctx.Deadline()
	r.writes <- excelBPSQuotaWrite{accountID: id, updates: copied, ctxErr: ctx.Err(), deadline: deadline}
	return r.updateErr
}

func (r *excelBPSQuotaRepo) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	deadline, _ := ctx.Deadline()
	r.writes <- excelBPSQuotaWrite{accountID: id, resetAt: resetAt, ctxErr: ctx.Err(), deadline: deadline}
	return r.limitErr
}

func nextExcelBPSQuotaWrite(t *testing.T, repo *excelBPSQuotaRepo) excelBPSQuotaWrite {
	t.Helper()
	select {
	case write := <-repo.writes:
		return write
	case <-time.After(3 * time.Second):
		t.Fatal("expected an account quota update")
		return excelBPSQuotaWrite{}
	}
}

func requireNoExcelBPSQuotaWrite(t *testing.T, repo *excelBPSQuotaRepo) {
	t.Helper()
	select {
	case write := <-repo.writes:
		t.Fatalf("unexpected account quota update: %+v", write)
	case <-time.After(20 * time.Millisecond):
	}
}

func nextExcelBPSRateLimitWrite(t *testing.T, repo *excelBPSQuotaRepo) excelBPSQuotaWrite {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case write := <-repo.writes:
			if !write.resetAt.IsZero() {
				return write
			}
		case <-timer.C:
			t.Fatal("expected an account rate-limit update")
			return excelBPSQuotaWrite{}
		}
	}
}

func excelBPSQuotaHeaders(used5h, used7d string) http.Header {
	headers := http.Header{}
	headers.Set("x-codex-primary-used-percent", used7d)
	headers.Set("x-codex-primary-reset-after-seconds", "604800")
	headers.Set("x-codex-primary-window-minutes", "10080")
	headers.Set("x-codex-secondary-used-percent", used5h)
	headers.Set("x-codex-secondary-reset-after-seconds", "18000")
	headers.Set("x-codex-secondary-window-minutes", "300")
	return headers
}

func TestExcelBPSQuotaSnapshotAtResponseBoundary(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, brokenStream := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/broken=%t", stream, brokenStream), func(t *testing.T) {
				wire := "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_bps_quota\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n\n"
				if brokenStream {
					wire = ""
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: excelBPSQuotaHeaders("63", "28"), Body: io.NopCloser(strings.NewReader(wire)),
				}}
				svc := openAIClientToolsTestService(upstream)
				svc.codexSnapshotThrottle = newAccountWriteThrottle(time.Hour)
				repo := &excelBPSQuotaRepo{writes: make(chan excelBPSQuotaWrite, 4)}
				svc.accountRepo = repo
				account := excelAccount()
				body := []byte(fmt.Sprintf("{\"model\":\"gpt-6-astra\",\"input\":\"test\",\"stream\":%t}", stream))
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				_, err := svc.Forward(context.Background(), c, account, body)
				if brokenStream {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				write := nextExcelBPSQuotaWrite(t, repo)
				require.Equal(t, account.ID, write.accountID)
				require.Equal(t, 63.0, write.updates["codex_5h_used_percent"])
				require.Equal(t, 28.0, write.updates["codex_7d_used_percent"])
				updatedAtValue, ok := write.updates["codex_usage_updated_at"].(string)
				require.True(t, ok)
				updatedAt, err := time.Parse(time.RFC3339, updatedAtValue)
				require.NoError(t, err)
				require.WithinDuration(t, time.Now(), updatedAt, 5*time.Second)
				require.NoError(t, write.ctxErr)
				require.True(t, write.resetAt.IsZero())
				require.NotContains(t, account.Extra, "codex_usage_updated_at", "do not mutate a shared account snapshot")
				// A second response still succeeds but uses the existing snapshot throttle.
				upstream.resp.Body = io.NopCloser(strings.NewReader(wire))
				c, _ = gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				_, _ = svc.Forward(context.Background(), c, account, body)
				requireNoExcelBPSQuotaWrite(t, repo)
			})
		}
	}
}

func TestExcelBPSQuotaSnapshotRequiresHeaders(t *testing.T) {
	for _, headers := range []http.Header{nil, {"X-Codex-Primary-Used-Percent": {"invalid"}}} {
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(""))}}
		svc := openAIClientToolsTestService(upstream)
		svc.codexSnapshotThrottle = newAccountWriteThrottle(time.Hour)
		repo := &excelBPSQuotaRepo{writes: make(chan excelBPSQuotaWrite, 4)}
		svc.accountRepo = repo
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		_, _ = svc.Forward(context.Background(), c, excelAccount(), []byte("{\"model\":\"gpt-6-astra\",\"input\":\"test\"}"))
		requireNoExcelBPSQuotaWrite(t, repo)
	}
}

type excelBPSCancelReader struct {
	io.Reader
	cancel context.CancelFunc
}

func (r *excelBPSCancelReader) Read(p []byte) (int, error) {
	r.cancel()
	return r.Reader.Read(p)
}

func TestExcelBPS429ClassifiesQuotaAndTransientLimits(t *testing.T) {
	for _, tc := range []struct {
		name             string
		headers          http.Header
		raw              string
		cancelOnRead     bool
		wantAccountBlock bool
	}{
		{name: "five hour quota", headers: excelBPSQuotaHeaders("100", "20"), wantAccountBlock: true},
		{name: "seven day quota", headers: excelBPSQuotaHeaders("20", "100"), wantAccountBlock: true},
		{name: "body reset timestamp", raw: fmt.Sprintf(`{"error":{"type":"usage_limit_reached","resets_at":%d}}`, time.Now().Add(2*time.Hour).Unix()), wantAccountBlock: true},
		{name: "body reset duration", raw: `{"error":{"type":"usage_limit_reached","resets_in_seconds":7200}}`, wantAccountBlock: true},
		{name: "unexhausted headers", headers: excelBPSQuotaHeaders("30", "20")},
		{name: "generic rate limit"},
		{name: "retry after", headers: http.Header{"Retry-After": {"23"}}},
		{name: "malformed body", raw: "not json"},
		{name: "client cancellation", headers: excelBPSQuotaHeaders("100", "100"), cancelOnRead: true},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				raw := tc.raw
				if raw == "" {
					raw = `{"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"PRIVATE_UPSTREAM"}}`
				}
				var reader io.Reader = strings.NewReader(raw)
				if tc.cancelOnRead {
					reader = &excelBPSCancelReader{Reader: reader, cancel: cancel}
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusTooManyRequests, Header: tc.headers, Body: io.NopCloser(reader)}}
				svc := openAIClientToolsTestService(upstream)
				repo := &excelBPSQuotaRepo{writes: make(chan excelBPSQuotaWrite, 4)}
				svc.accountRepo = repo
				svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
				svc.rateLimitService.SetSettingService(NewSettingService(&excelBPSQuotaSettingsRepo{cooldown: `{"enabled":true,"cooldown_seconds":11}`}, svc.cfg))
				svc.rateLimitService.SetAccountRuntimeBlocker(svc)
				account := excelAccount()
				account.Extra["openai_excel_bps_auto_disable_on_403"] = true
				retryStarted := time.Now()
				svc.openaiOAuth429RetryStartedAt.Store(account.ID, retryStarted)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
				body := []byte(fmt.Sprintf(`{"model":"gpt-6-astra","input":"test","stream":%t}`, stream))

				_, err := svc.Forward(ctx, c, account, body)

				var failover *UpstreamFailoverError
				if tc.cancelOnRead {
					require.ErrorIs(t, err, context.Canceled)
					require.NotErrorAs(t, err, &failover)
				} else {
					require.ErrorAs(t, err, &failover)
					require.Equal(t, http.StatusTooManyRequests, failover.StatusCode)
					require.True(t, failover.ShouldRetryNextAccount())
					require.False(t, failover.RetryableOnSameAccount)
					require.False(t, failover.ShouldReportAccountScheduleFailure())
					require.True(t, failover.SkipOAuth429AttemptLimit)
					require.NotContains(t, string(failover.ResponseBody), "PRIVATE_UPSTREAM")
					require.Equal(t, tc.headers.Get("Retry-After"), failover.ResponseHeaders.Get("Retry-After"))
				}
				require.False(t, IsResponseCommitted(c))
				require.False(t, c.Writer.Written())
				require.Empty(t, rec.Body.String())
				require.Len(t, upstream.requests, 1)
				require.True(t, account.IsSchedulable())
				require.True(t, account.IsExcelBPSEnabled())

				if tc.wantAccountBlock {
					write := nextExcelBPSRateLimitWrite(t, repo)
					require.Equal(t, account.ID, write.accountID)
					require.Greater(t, time.Until(write.resetAt), time.Hour)
					require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.False(t, svc.isExcelBPSRateLimited(account, "gpt-6-astra"))
					_, retryExists := svc.openaiOAuth429RetryStartedAt.Load(account.ID)
					require.False(t, retryExists)
				} else {
					requireNoExcelBPSQuotaWrite(t, repo)
					require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.Equal(t, !tc.cancelOnRead, svc.isExcelBPSRateLimited(account, "gpt-6-astra"))
					retryState, ok := svc.openaiOAuth429RetryStartedAt.Load(account.ID)
					require.True(t, ok)
					require.Equal(t, retryStarted, retryState)
				}
			})
		}
	}
}

func TestExcelBPSCooldownIsScopedAndExpires(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := excelAccount()
	account.Extra["openai_excel_bps_models"] = []string{"gpt-6-astra"}
	account.Credentials["model_mapping"] = map[string]any{"alias": "gpt-6-astra"}
	svc.coolDownExcelBPS(context.Background(), account, http.Header{"Retry-After": {"60"}}, nil)
	bps := WithExcelBPSScheduling(context.Background(), []byte(`{"input":"hello"}`))
	native := WithExcelBPSScheduling(context.Background(), []byte(`{"tools":[{"type":"image_generation"}]}`))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(bps, account, "gpt-6-astra", false))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(bps, account, "alias", true))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(bps, account, "gpt-6-sol", false))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(native, account, "gpt-6-astra", false))
	require.True(t, svc.isOpenAIAccountRequestRuntimeBlocked(context.Background(), account, "gpt-6-astra", false))
	account.Extra["openai_excel_bps"] = false
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(bps, account, "gpt-6-astra", false))
	account.Extra["openai_excel_bps"] = true
	before, _ := svc.excelBPSRateLimitedUntil.Load(account.ID)
	svc.coolDownExcelBPS(context.Background(), account, http.Header{"Retry-After": {"1"}}, nil)
	after, _ := svc.excelBPSRateLimitedUntil.Load(account.ID)
	require.Equal(t, before, after, "concurrent shorter limits must not release a longer cooldown")
	svc.excelBPSRateLimitedUntil.Store(account.ID, time.Now().Add(-time.Second))
	require.False(t, svc.isOpenAIAccountRequestRuntimeBlocked(bps, account, "gpt-6-astra", false))
	_, exists := svc.excelBPSRateLimitedUntil.Load(account.ID)
	require.False(t, exists)
}

func TestExcelBPSCooldownHonorsSettingsAndRetryAfter(t *testing.T) {
	for _, tc := range []struct {
		name, retryAfter, settings string
		want                       time.Duration
	}{
		{"default", "", "", 5 * time.Second},
		{"configured", "", `{"enabled":true,"cooldown_seconds":11}`, 11 * time.Second},
		{"disabled default", "", `{"enabled":false,"cooldown_seconds":11}`, 5 * time.Second},
		{"disabled retry after", "60", `{"enabled":false,"cooldown_seconds":11}`, time.Minute},
		{"retry after seconds", "23", "", 23 * time.Second},
		{"retry after date", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat), "", time.Minute},
		{"invalid", "invalid", "", 5 * time.Second},
		{"negative", "-1", "", 5 * time.Second},
		{"bounded", "999999", "", time.Duration(maxRateLimit429CooldownSeconds) * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &OpenAIGatewayService{}
			if tc.settings != "" {
				svc.rateLimitService = &RateLimitService{settingService: NewSettingService(&excelBPSQuotaSettingsRepo{cooldown: tc.settings}, nil)}
			}
			a := excelAccount()
			now := time.Now()
			svc.coolDownExcelBPS(context.Background(), a, http.Header{"Retry-After": {tc.retryAfter}}, nil)
			until, exists := svc.excelBPSRateLimitedUntil.Load(a.ID)
			require.Equal(t, tc.want > 0, exists)
			if exists {
				require.WithinDuration(t, now.Add(tc.want), until.(time.Time), 2*time.Second)
			}
		})
	}
}

func TestExcelBPSNon429DoesNotChangeQuotaState(t *testing.T) {
	// Authentication failures have their own scheduling assertions.
	for _, status := range []int{400, 403, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: status, Header: excelBPSQuotaHeaders("100", "100"), Body: io.NopCloser(strings.NewReader("{}"))}}
			svc := openAIClientToolsTestService(upstream)
			repo := &excelBPSQuotaRepo{writes: make(chan excelBPSQuotaWrite, 4)}
			svc.accountRepo = repo
			svc.rateLimitService = NewRateLimitService(repo, nil, svc.cfg, nil, nil)
			svc.rateLimitService.SetAccountRuntimeBlocker(svc)
			account := excelAccount()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			_, err := svc.Forward(context.Background(), c, account, []byte("{\"model\":\"gpt-6-astra\",\"input\":\"test\"}"))
			require.Error(t, err)
			if status >= 500 {
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Equal(t, status, failover.StatusCode)
				require.Empty(t, rec.Body.String())
			} else {
				require.Equal(t, status, rec.Code)
			}
			requireNoExcelBPSQuotaWrite(t, repo)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			require.True(t, account.IsSchedulable())
		})
	}
}
