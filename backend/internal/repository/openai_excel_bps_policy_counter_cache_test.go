//go:build unit

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestExcelBPSPolicyCounterRedisIsolationAndWindow(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	native := NewOpenAI403CounterCache(client)
	counter, ok := native.(service.ExcelBPSPolicyCounterCache)
	require.True(t, ok, "production constructor must expose the separate BPS counter")
	ctx := context.Background()
	_, err := native.IncrementOpenAI403Count(ctx, 58, 180)
	require.NoError(t, err)
	_, err = counter.IncrementExcelBPSPolicy403Count(ctx, 59, 180)
	require.NoError(t, err)
	for i := int64(1); i <= 3; i++ {
		count, err := counter.IncrementExcelBPSPolicy403Count(ctx, 58, 180)
		require.NoError(t, err)
		require.Equal(t, i, count)
	}
	key := fmt.Sprintf("%s%d", excelBPSPolicy403CounterPrefix, 58)
	require.Equal(t, 180*time.Minute, server.TTL(key))
	server.FastForward(time.Minute)
	_, err = counter.IncrementExcelBPSPolicy403Count(ctx, 58, 180)
	require.NoError(t, err)
	require.Equal(t, 179*time.Minute, server.TTL(key), "failures must not extend the original window")
	require.NoError(t, counter.ResetExcelBPSPolicy403Count(ctx, 58))
	require.False(t, server.Exists(key))
	nativeCount, err := native.IncrementOpenAI403Count(ctx, 58, 180)
	require.NoError(t, err)
	require.Equal(t, int64(2), nativeCount, "BPS success must not reset native failures")
	otherCount, err := counter.IncrementExcelBPSPolicy403Count(ctx, 59, 180)
	require.NoError(t, err)
	require.Equal(t, int64(2), otherCount)
	_, err = counter.IncrementExcelBPSPolicy403Count(ctx, 58, 180)
	require.NoError(t, err)
	require.NoError(t, native.ResetOpenAI403Count(ctx, 58))
	count, err := counter.IncrementExcelBPSPolicy403Count(ctx, 58, 180)
	require.NoError(t, err)
	require.Equal(t, int64(2), count, "native success must not reset BPS failures")
	server.FastForward(180 * time.Minute)
	count, err = counter.IncrementExcelBPSPolicy403Count(ctx, 58, 180)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

func TestExcelBPSPolicyCounterRedisConcurrentIncrements(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	counter := NewOpenAI403CounterCache(client).(service.ExcelBPSPolicyCounterCache)
	var wg sync.WaitGroup
	counts := make(chan int64, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			count, err := counter.IncrementExcelBPSPolicy403Count(context.Background(), 58, 180)
			if err != nil {
				t.Error(err)
				return
			}
			counts <- count
		}()
	}
	wg.Wait()
	close(counts)
	seen := make(map[int64]bool)
	for count := range counts {
		require.False(t, seen[count])
		seen[count] = true
	}
	require.Len(t, seen, 20)
	require.True(t, seen[1])
	require.True(t, seen[20])
}
