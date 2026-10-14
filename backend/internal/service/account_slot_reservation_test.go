package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSlotReservationConfiguration(t *testing.T) {
	for _, seconds := range []any{0, -1, 1.5, 3601, "60", nil, math.NaN(), math.Inf(1)} {
		require.Error(t, ValidateSlotReservationExtra(map[string]any{SlotReservationSeconds: seconds}))
	}
	for _, seconds := range []any{1, 60.0, int64(3600), json.Number("90")} {
		require.NoError(t, ValidateSlotReservationExtra(map[string]any{SlotReservationSeconds: seconds}))
	}
	require.Error(t, ValidateSlotReservationExtra(map[string]any{SlotReservationEnabled: "true"}))
	require.NoError(t, ValidateSlotReservationExtra(nil))
	a := &Account{Type: AccountTypeOAuth, Extra: map[string]any{SlotReservationEnabled: true}}
	require.Equal(t, 60, a.GetSlotReservationSeconds())
	a.Type = AccountTypeAPIKey
	require.Zero(t, a.GetSlotReservationSeconds())
}

type reservationForwardCache struct {
	stubConcurrencyCache
	reservedAccount int64
	finishedSeconds int
}

func (c *reservationForwardCache) AcquireReservedAccountSlot(context.Context, int64, int, string, string) (bool, error) {
	return true, nil
}
func (c *reservationForwardCache) FinishReservedAccountSlot(_ context.Context, _ int64, _, _ string, seconds int) error {
	c.finishedSeconds = seconds
	return nil
}
func (*reservationForwardCache) CancelAccountSlotReservation(context.Context, int64, string) error {
	return nil
}
func (*reservationForwardCache) GetAccountSlotReservationsBatch(context.Context, []int64) (map[int64][]int64, error) {
	return nil, nil
}
func (c *reservationForwardCache) GetReservedSessionAccount(context.Context, string) (int64, error) {
	return c.reservedAccount, nil
}

func TestSlotReservationForwardingBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   int
	}{
		{"success", 200, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.4\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n", 60},
		{"user input error", 400, `{"error":{"type":"invalid_request_error","message":"invalid input"}}`, 60},
		{"upstream rate limit", 429, `{"error":{"type":"rate_limit_error","message":"rate limited"}}`, 0},
		{"failed stream", 200, "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_1\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"upstream failed\"}}}\n\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := &reservationForwardCache{}
			concurrency := NewConcurrencyService(cache)
			ctx := WithAccountSlotReservation(context.Background(), 1, "conversation")
			slot, err := concurrency.AcquireAccountSlot(ctx, 10, 1)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":true}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))).WithContext(ctx)
			contentType := "text/event-stream"
			if tc.status != 200 {
				contentType = "application/json"
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(tc.body))}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "test-account"}, Extra: map[string]any{SlotReservationEnabled: true}}
			_, _ = svc.ForwardAsChatCompletions(ctx, c, account, body, "", "gpt-5.4")
			slot.ReleaseFunc()
			require.Equal(t, tc.want, cache.finishedSeconds)
		})
	}
}

func TestSlotReservationUserErrorClassification(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Writer.WriteHeader(400)
	require.True(t, reservationUserError(context.Background(), errors.New("invalid input"), []*gin.Context{c}))
	c.Set(OpsUpstreamStatusCodeKey, 429)
	require.False(t, reservationUserError(context.Background(), errors.New("mapped upstream error"), []*gin.Context{c}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, reservationUserError(ctx, context.Canceled, nil))
	require.False(t, reservationUserError(ctx, &UpstreamFailoverError{StatusCode: 429}, nil))
	require.False(t, reservationUserError(ctx, context.DeadlineExceeded, nil))
}

func TestSlotReservationSchedulerUsesReservationOverExistingSticky(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(fmt.Sprint(advanced), func(t *testing.T) {
			groupID := int64(10)
			accounts := []Account{
				{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}},
				{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}},
			}
			cache := &reservationForwardCache{reservedAccount: 2}
			gatewayCache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:session": 1}}
			cfg := &config.Config{}
			cfg.Gateway.Scheduling.LoadBatchEnabled = true
			svc := &OpenAIGatewayService{accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}, cache: gatewayCache, cfg: cfg, concurrencyService: NewConcurrencyService(cache)}
			if advanced {
				svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
			}
			ctx := WithAccountSlotReservation(context.Background(), 1, "session")
			selection, _, err := svc.SelectAccountWithScheduler(ctx, &groupID, "", "session", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(2), selection.Account.ID)
			selection.ReleaseFunc()
			accounts[1].Schedulable = false
			selection, _, err = svc.SelectAccountWithScheduler(WithAccountSlotReservation(context.Background(), 1, "session"), &groupID, "", "session", "gpt-5.1", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(1), selection.Account.ID, "reservation must not bypass eligibility")
			selection.ReleaseFunc()
		})
	}
}

func TestSlotReservationUpstreamErrorInvalidatesBeforeInternalRetry(t *testing.T) {
	cache := &reservationForwardCache{}
	svc := NewConcurrencyService(cache)
	ctx := WithAccountSlotReservation(context.Background(), 1, "s")
	slot, err := svc.AcquireAccountSlot(ctx, 10, 1)
	require.NoError(t, err)
	account := &Account{ID: 10, Type: AccountTypeOAuth, Extra: map[string]any{SlotReservationEnabled: true}}
	finish := BeginAccountSlotReservation(ctx, account)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
	SetOpsUpstreamError(c, 429, "rate limited", "")
	finish(nil, true)
	slot.ReleaseFunc()
	require.Zero(t, cache.finishedSeconds, "internal retry success must not restore reservation")
}
