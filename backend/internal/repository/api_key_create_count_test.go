package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyCacheIncrementCreateCountUsesFixedWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer func() { _ = client.Close() }()
	cache := NewAPIKeyCache(client)
	ctx := context.Background()
	key := apiKeyCreateCountKey(7)

	count, err := cache.(interface {
		IncrementCreateCount(context.Context, int64, time.Duration) (int64, error)
	}).IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, time.Hour, server.TTL(key))

	server.FastForward(40 * time.Minute)
	count, err = cache.(interface {
		IncrementCreateCount(context.Context, int64, time.Duration) (int64, error)
	}).IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Equal(t, 20*time.Minute, server.TTL(key))

	server.FastForward(21 * time.Minute)
	count, err = cache.(interface {
		IncrementCreateCount(context.Context, int64, time.Duration) (int64, error)
	}).IncrementCreateCount(ctx, 7, time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}
