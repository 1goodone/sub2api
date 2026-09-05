//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

type paymentVerificationCooldownStub struct {
	acquired atomic.Bool
	calls    atomic.Int32
	err      error
}

func (c *paymentVerificationCooldownStub) TryAcquire(context.Context, int64, time.Duration) (bool, error) {
	c.calls.Add(1)
	if c.err != nil {
		return false, c.err
	}
	return c.acquired.Load(), nil
}

type paymentVerificationBlockingProvider struct {
	paymentOrderLifecycleQueryProvider
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *paymentVerificationBlockingProvider) QueryOrder(ctx context.Context, tradeNo string) (*payment.QueryOrderResponse, error) {
	p.calls.Add(1)
	p.lastQueryTradeNo = tradeNo
	select {
	case <-p.started:
	default:
		close(p.started)
	}
	select {
	case <-p.release:
		return p.resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestReconcilePaidWithCooldownMergesConcurrentChecks(t *testing.T) {
	provider := &paymentVerificationBlockingProvider{
		paymentOrderLifecycleQueryProvider: paymentOrderLifecycleQueryProvider{
			resp: &payment.QueryOrderResponse{Status: payment.ProviderStatusPending},
		},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc, order, userID := newPaymentVerificationGateFixture(t, provider)

	const callers = 8
	start := make(chan struct{}, callers)
	proceed := make(chan struct{})
	results := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			start <- struct{}{}
			<-proceed
			_, err := svc.VerifyOrderByOutTradeNo(context.Background(), order.OutTradeNo, userID)
			results <- err
		}()
	}
	for i := 0; i < callers; i++ {
		<-start
	}
	close(proceed)
	require.Eventually(t, func() bool {
		return provider.calls.Load() == 1
	}, time.Second, time.Millisecond)
	// Keep the first provider call in flight long enough for every released
	// caller to enter the singleflight group before allowing it to finish.
	time.Sleep(200 * time.Millisecond)
	close(provider.release)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), provider.calls.Load())
}

func TestReconcilePaidWithCooldownSuppressesChecksDuringCooldown(t *testing.T) {
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{Status: payment.ProviderStatusPending},
	}
	cooldown := &paymentVerificationCooldownStub{}
	cooldown.acquired.Store(true)
	svc, order, userID := newPaymentVerificationGateFixture(t, provider)
	svc.SetPaymentVerificationCooldown(cooldown)

	_, err := svc.VerifyOrderByOutTradeNo(context.Background(), order.OutTradeNo, userID)
	require.NoError(t, err)
	cooldown.acquired.Store(false)
	_, err = svc.VerifyOrderByOutTradeNo(context.Background(), order.OutTradeNo, userID)
	require.NoError(t, err)

	require.Equal(t, 1, provider.queryCalls)
	require.Equal(t, int32(2), cooldown.calls.Load())
}

func TestReconcilePaidWithCooldownFailsOpenWhenRedisUnavailable(t *testing.T) {
	provider := &paymentOrderLifecycleQueryProvider{
		resp: &payment.QueryOrderResponse{Status: payment.ProviderStatusPending},
	}
	cooldown := &paymentVerificationCooldownStub{err: errors.New("redis unavailable")}
	svc, order, userID := newPaymentVerificationGateFixture(t, provider)
	svc.SetPaymentVerificationCooldown(cooldown)

	_, err := svc.VerifyOrderByOutTradeNo(context.Background(), order.OutTradeNo, userID)
	require.NoError(t, err)
	require.Equal(t, 1, provider.queryCalls)
	require.Equal(t, int32(1), cooldown.calls.Load())
}

func newPaymentVerificationGateFixture(t *testing.T, provider payment.Provider) (*PaymentService, *dbent.PaymentOrder, int64) {
	t.Helper()
	ctx := context.Background()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().
		SetEmail("payment-verification-gate@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-verification-gate-user").
		Save(ctx)
	require.NoError(t, err)

	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(88).
		SetPayAmount(88).
		SetFeeRate(0).
		SetRechargeCode("PAYMENT-VERIFICATION-GATE").
		SetOutTradeNo("sub2_payment_verification_gate").
		SetPaymentType(provider.ProviderKey()).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("api.example.com").
		Save(ctx)
	require.NoError(t, err)

	registry := payment.NewRegistry()
	registry.Register(provider)
	return &PaymentService{
		entClient:       client,
		registry:        registry,
		providersLoaded: true,
	}, order, user.ID
}
