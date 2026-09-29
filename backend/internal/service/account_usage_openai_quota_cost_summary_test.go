package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/stretchr/testify/require"
)

type openAIQuotaCostRepoStub struct {
	UsageLogRepository
	stats map[int64]*usagestats.AccountStats
	calls int
}

func (s *openAIQuotaCostRepoStub) GetAccountWindowStatsByStartBatch(_ context.Context, starts map[int64]time.Time) (map[int64]*usagestats.AccountStats, error) {
	s.calls++
	result := make(map[int64]*usagestats.AccountStats, len(starts))
	for accountID := range starts {
		result[accountID] = s.stats[accountID]
	}
	return result, nil
}

func TestGetOpenAIQuotaCostSummary(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	repo := &openAIQuotaCostRepoStub{stats: map[int64]*usagestats.AccountStats{
		1: {Cost: 4},
		2: {Cost: 3},
	}}
	svc := &AccountUsageService{usageLogRepo: repo}
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
			"codex_5h_used_percent": 50.0,
			"codex_5h_reset_at":     now.Add(2 * time.Hour).Format(time.RFC3339),
			"codex_7d_used_percent": 25.0,
			"codex_7d_reset_at":     now.Add(4 * 24 * time.Hour).Format(time.RFC3339),
		}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{
			"codex_5h_used_percent": 75.0,
			"codex_5h_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
			"codex_7d_used_percent": 50.0,
			"codex_7d_reset_at":     now.Add(2 * 24 * time.Hour).Format(time.RFC3339),
		}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}

	summary, err := svc.GetOpenAIQuotaCostSummary(context.Background(), accounts, now)
	require.NoError(t, err)
	require.Equal(t, 2, repo.calls)
	require.NotNil(t, summary.FiveHour.RemainingCost)
	require.NotNil(t, summary.FiveHour.EstimatedCost)
	require.InDelta(t, 5, *summary.FiveHour.RemainingCost, 1e-9)
	require.InDelta(t, 12, *summary.FiveHour.EstimatedCost, 1e-9)
	require.NotNil(t, summary.SevenDay.RemainingCost)
	require.NotNil(t, summary.SevenDay.EstimatedCost)
	require.InDelta(t, 15, *summary.SevenDay.RemainingCost, 1e-9)
	require.InDelta(t, 22, *summary.SevenDay.EstimatedCost, 1e-9)
}
