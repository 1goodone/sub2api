package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type channelRoutingHealthRepoStub struct {
	ChannelMonitorRepository
	calls       int
	unavailable map[int64]bool
	err         error
}

func (s *channelRoutingHealthRepoStub) UnavailableGroupIDs(context.Context, []int64) (map[int64]bool, error) {
	s.calls++
	return s.unavailable, s.err
}

type channelRoutingRuntimeStub struct {
	runtime ChannelMonitorRuntime
}

func (s channelRoutingRuntimeStub) GetChannelMonitorRuntime(context.Context) ChannelMonitorRuntime {
	return s.runtime
}

func TestChannelRoutingHealthOnlyUsesEnabledV1(t *testing.T) {
	repo := &channelRoutingHealthRepoStub{unavailable: map[int64]bool{2: true}}
	svc := NewChannelMonitorService(repo, nil)

	for _, runtime := range []ChannelMonitorRuntime{
		{Enabled: false, Mode: ChannelMonitorModeV1},
		{Enabled: true, Mode: ChannelMonitorModeV2},
	} {
		svc.SetRuntimeReader(channelRoutingRuntimeStub{runtime: runtime})
		unavailable, err := svc.UnavailableGroupIDs(context.Background(), []int64{1, 2})
		require.NoError(t, err)
		require.Empty(t, unavailable)
	}
	require.Zero(t, repo.calls)

	svc.SetRuntimeReader(channelRoutingRuntimeStub{runtime: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}})
	unavailable, err := svc.UnavailableGroupIDs(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{2: true}, unavailable)
	require.Equal(t, 1, repo.calls)

	unavailable, err = svc.UnavailableGroupIDs(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{2: true}, unavailable)
	require.Equal(t, 1, repo.calls, "fresh routing health should be cached")

	repo.err = errors.New("database unavailable")
	_, err = svc.UnavailableGroupIDs(context.Background(), []int64{3})
	require.Error(t, err)

	unavailable, err = svc.UnavailableGroupIDs(context.Background(), []int64{3})
	require.NoError(t, err)
	require.Empty(t, unavailable)
	require.Equal(t, 2, repo.calls, "failed health reads should be cached fail-open briefly")
}

func TestMonitorHTTPFailureKindDoesNotBlockOnAuthOrRateLimit(t *testing.T) {
	require.Equal(t, "upstream_5xx", monitorHTTPFailureKind(http.StatusInternalServerError))
	require.Equal(t, "auth", monitorHTTPFailureKind(http.StatusUnauthorized))
	require.Equal(t, "auth", monitorHTTPFailureKind(http.StatusForbidden))
	require.Equal(t, "rate_limit", monitorHTTPFailureKind(http.StatusTooManyRequests))
	require.Equal(t, "upstream_4xx", monitorHTTPFailureKind(http.StatusBadRequest))
}
