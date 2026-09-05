//go:build unit

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestPaymentVerificationCooldownIsAtomicAndExpires(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cooldown := NewPaymentVerificationCooldown(rdb)
	ctx := context.Background()

	acquired, err := cooldown.TryAcquire(ctx, 42, 15*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)

	acquired, err = cooldown.TryAcquire(ctx, 42, 15*time.Second)
	require.NoError(t, err)
	require.False(t, acquired)

	mr.FastForward(16 * time.Second)
	acquired, err = cooldown.TryAcquire(ctx, 42, 15*time.Second)
	require.NoError(t, err)
	require.True(t, acquired)
}
