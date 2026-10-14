package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestSlotReservationLifecycle(t *testing.T) {
	mr := miniredis.RunT(t)
	now := time.Now().Truncate(time.Second)
	mr.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := NewConcurrencyCache(client, 15, 900)
	svc := service.NewConcurrencyService(cache)
	account := &service.Account{ID: 10, Type: service.AccountTypeOAuth, Extra: map[string]any{service.SlotReservationEnabled: true}}
	ctx := func(user int64, session string) context.Context {
		return service.WithAccountSlotReservation(context.Background(), user, session)
	}
	acquire := func(c context.Context, max int) *service.AcquireResult {
		r, err := svc.AcquireAccountSlot(c, 10, max)
		require.NoError(t, err)
		return r
	}
	count := func(want int) {
		n, err := cache.GetAccountConcurrency(context.Background(), 10)
		require.NoError(t, err)
		require.Equal(t, want, n)
	}
	c := ctx(1, "conversation")
	r := acquire(c, 1)
	require.True(t, r.Acquired)
	service.BeginAccountSlotReservation(c, account)(nil, true)
	r.ReleaseFunc()
	r.ReleaseFunc()
	count(1)
	deadlines, err := svc.GetAccountSlotReservationsBatch(context.Background(), []int64{10})
	require.NoError(t, err)
	require.Equal(t, []int64{now.Unix() + 60}, deadlines[10])
	require.False(t, acquire(ctx(2, "conversation"), 1).Acquired, "another user cannot claim it")
	require.False(t, acquire(ctx(1, "other"), 1).Acquired, "another session cannot claim it")
	mr.SetTime(now.Add(30 * time.Second))
	c = ctx(1, "conversation")
	r = acquire(c, 1)
	require.True(t, r.Acquired, "owner can reclaim a full account")
	require.False(t, acquire(ctx(1, "conversation"), 1).Acquired, "an active slot cannot be shared")
	deadlines, err = svc.GetAccountSlotReservationsBatch(context.Background(), []int64{10})
	require.NoError(t, err)
	require.Empty(t, deadlines[10])
	service.BeginAccountSlotReservation(c, account)(nil, true)
	r.ReleaseFunc()
	deadlines, err = svc.GetAccountSlotReservationsBatch(context.Background(), []int64{10})
	require.NoError(t, err)
	require.Equal(t, []int64{now.Unix() + 90}, deadlines[10])
	mr.SetTime(now.Add(90 * time.Second))
	count(0)
	r = acquire(ctx(2, "other"), 1)
	require.True(t, r.Acquired)
	r.ReleaseFunc()
	count(0)
}

func TestSlotReservationSchedulerProjection(t *testing.T) {
	a := &service.Account{Type: service.AccountTypeOAuth, Extra: filterSchedulerExtra(map[string]any{
		service.SlotReservationEnabled: true, service.SlotReservationSeconds: 120,
		"unused_large_payload": "discard",
	})}
	require.Equal(t, 120, a.GetSlotReservationSeconds())
	require.NotContains(t, a.Extra, "unused_large_payload")
}

func TestSlotReservationCompletionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name                          string
		typ                           string
		enabled                       bool
		seconds                       int
		err                           error
		success, cancel, releaseEarly bool
		want                          int
	}{
		{"default disabled", service.AccountTypeOAuth, false, 60, nil, true, false, false, 0},
		{"api key excluded", service.AccountTypeAPIKey, true, 60, nil, true, false, false, 0},
		{"custom duration", service.AccountTypeOAuth, true, 17, nil, true, false, false, 1},
		{"rate limit", service.AccountTypeOAuth, true, 60, &service.UpstreamFailoverError{StatusCode: 429}, false, false, false, 0},
		{"transport error", service.AccountTypeOAuth, true, 60, errors.New("upstream EOF"), false, false, false, 0},
		{"stream failed terminal", service.AccountTypeOAuth, true, 60, nil, false, false, false, 0},
		{"deadline", service.AccountTypeOAuth, true, 60, context.DeadlineExceeded, false, false, false, 0},
		{"user cancel", service.AccountTypeOAuth, true, 60, context.Canceled, false, true, true, 1},
		{"upstream failure after client cancel", service.AccountTypeOAuth, true, 60, errors.New("upstream error"), false, true, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			now := time.Now().Truncate(time.Second)
			mr.SetTime(now)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			cache := NewConcurrencyCache(client, 15, 900)
			svc := service.NewConcurrencyService(cache)
			ctx, cancel := context.WithCancel(service.WithAccountSlotReservation(context.Background(), 1, "s"))
			defer cancel()
			r, err := svc.AcquireAccountSlot(ctx, 10, 1)
			require.NoError(t, err)
			require.True(t, r.Acquired)
			account := &service.Account{ID: 10, Type: tc.typ, Extra: map[string]any{service.SlotReservationEnabled: tc.enabled, service.SlotReservationSeconds: tc.seconds}}
			finish := service.BeginAccountSlotReservation(ctx, account)
			if tc.cancel {
				cancel()
			}
			if tc.releaseEarly {
				r.ReleaseFunc()
			}
			finish(tc.err, tc.success)
			r.ReleaseFunc()
			n, err := cache.GetAccountConcurrency(context.Background(), 10)
			require.NoError(t, err)
			require.Equal(t, tc.want, n)
			if tc.want == 1 {
				deadlines, err := svc.GetAccountSlotReservationsBatch(context.Background(), []int64{10})
				require.NoError(t, err)
				require.Equal(t, []int64{now.Unix() + int64(tc.seconds)}, deadlines[10])
			}
		})
	}
}

func TestSlotReservationParallelAndAccountError(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache := NewConcurrencyCache(client, 15, 900)
	svc := service.NewConcurrencyService(cache)
	account := &service.Account{ID: 10, Type: service.AccountTypeOAuth, Extra: map[string]any{service.SlotReservationEnabled: true}}
	for i := 0; i < 2; i++ {
		ctx := service.WithAccountSlotReservation(context.Background(), int64(i+1), "s")
		r, err := svc.AcquireAccountSlot(ctx, 10, 3)
		require.NoError(t, err)
		require.True(t, r.Acquired)
		service.BeginAccountSlotReservation(ctx, account)(nil, true)
		r.ReleaseFunc()
	}
	ctx := service.WithAccountSlotReservation(context.Background(), 3, "s")
	r, err := svc.AcquireAccountSlot(ctx, 10, 3)
	require.NoError(t, err)
	require.True(t, r.Acquired)
	service.BeginAccountSlotReservation(ctx, account)(&service.UpstreamFailoverError{StatusCode: 429}, false)
	r.ReleaseFunc()
	n, err := cache.GetAccountConcurrency(context.Background(), 10)
	require.NoError(t, err)
	require.Zero(t, n, "unhealthy account drops every idle reservation")
	// Parallel requests from one conversation count separately, then coalesce.
	c1 := service.WithAccountSlotReservation(context.Background(), 1, "parallel")
	c2 := service.WithAccountSlotReservation(context.Background(), 1, "parallel")
	a, err := svc.AcquireAccountSlot(c1, 10, 2)
	require.NoError(t, err)
	require.True(t, a.Acquired)
	b, err := svc.AcquireAccountSlot(c2, 10, 2)
	require.NoError(t, err)
	require.True(t, b.Acquired)
	n, err = cache.GetAccountConcurrency(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 2, n)
	service.BeginAccountSlotReservation(c1, account)(nil, true)
	a.ReleaseFunc()
	service.BeginAccountSlotReservation(c2, account)(nil, true)
	b.ReleaseFunc()
	n, err = cache.GetAccountConcurrency(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
}

func TestSlotReservationErrorFencesOlderInFlightSuccess(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache := NewConcurrencyCache(client, 15, 900)
	svc := service.NewConcurrencyService(cache)
	account := &service.Account{ID: 10, Type: service.AccountTypeOAuth, Extra: map[string]any{service.SlotReservationEnabled: true}}
	c1 := service.WithAccountSlotReservation(context.Background(), 1, "a")
	c2 := service.WithAccountSlotReservation(context.Background(), 2, "b")
	a, err := svc.AcquireAccountSlot(c1, 10, 2)
	require.NoError(t, err)
	b, err := svc.AcquireAccountSlot(c2, 10, 2)
	require.NoError(t, err)
	service.BeginAccountSlotReservation(c1, account)(errors.New("rate limited"), false)
	a.ReleaseFunc()
	service.BeginAccountSlotReservation(c2, account)(nil, true)
	b.ReleaseFunc()
	n, err := cache.GetAccountConcurrency(context.Background(), 10)
	require.NoError(t, err)
	require.Zero(t, n)
}

func TestSlotReservationLongTTLAndStartup(t *testing.T) {
	mr := miniredis.RunT(t)
	now := time.Now().Truncate(time.Second)
	mr.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	cache := NewConcurrencyCache(client, 1, 60).(*concurrencyCache)
	svc := service.NewConcurrencyService(cache)
	ctx := service.WithAccountSlotReservation(context.Background(), 1, "long")
	a, err := svc.AcquireAccountSlot(ctx, 10, 2)
	require.NoError(t, err)
	account := &service.Account{ID: 10, Type: service.AccountTypeOAuth, Extra: map[string]any{service.SlotReservationEnabled: true, service.SlotReservationSeconds: 3600}}
	service.BeginAccountSlotReservation(ctx, account)(nil, true)
	a.ReleaseFunc()
	ok, err := cache.AcquireAccountSlot(context.Background(), 10, 2, "other-process")
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, cache.CleanupExpiredAccountSlots(context.Background(), 10))
	require.NoError(t, cache.CleanupStaleProcessSlots(context.Background(), "new-process"))
	require.GreaterOrEqual(t, mr.TTL(accountSlotKey(10)), 3600*time.Second)
	mr.FastForward(120 * time.Second)
	mr.SetTime(now.Add(120 * time.Second))
	n, err := cache.GetAccountConcurrency(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	id, err := cache.GetReservedSessionAccount(context.Background(), "missing")
	require.NoError(t, err)
	require.Zero(t, id)
}
