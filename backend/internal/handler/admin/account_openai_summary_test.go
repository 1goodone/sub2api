package admin

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIAccountSummary(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	quotaPauseUntil := now.Add(3 * time.Hour)
	accounts := []service.Account{
		{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 3},
		{ID: 2, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, Concurrency: 2, RateLimitResetAt: &fiveHourReset, Extra: map[string]any{
			"codex_5h_used_percent": 95.0,
			"codex_5h_reset_at":     fiveHourReset.Format(time.RFC3339),
		}},
		{ID: 3, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusError, Concurrency: 1},
		{ID: 4, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, TempUnschedulableUntil: &quotaPauseUntil, TempUnschedulableReason: service.BuildTempUnschedReasonPayload(service.CodexQuotaOverdraftPauseSource, "quota exhausted")},
		{ID: 5, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Schedulable: true, TempUnschedulableUntil: &quotaPauseUntil, TempUnschedulableReason: "maintenance"},
		{ID: 6, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Concurrency: 4},
	}

	remaining, estimated := 12.5, 25.0
	quotaCosts := service.OpenAIQuotaCostSummary{
		FiveHour: service.OpenAIQuotaCostEstimate{RemainingCost: &remaining, EstimatedCost: &estimated, TotalAccounts: 5},
		SevenDay: service.OpenAIQuotaCostEstimate{TotalAccounts: 5},
	}
	summary := buildOpenAIAccountSummary(accounts, map[int64]int{1: 1, 2: 2, 6: 3}, quotaCosts, now)
	require.Equal(t, openAIAccountTypeCounts{OAuth: 5, APIKey: 1}, summary.Accounts)
	require.Equal(t, openAIOAuthStatusCounts{Normal: 1, RateLimited: 2, Error: 1}, summary.OAuthStatus)
	require.Equal(t, quotaCosts, summary.OAuthQuota)
	require.Equal(t, openAIConcurrencyCount{Used: 3, Total: 6}, summary.Concurrency.OAuth)
	require.Equal(t, openAIConcurrencyCount{Used: 3, Total: 4}, summary.Concurrency.APIKey)
	unknown := buildOpenAIAccountSummary(accounts, nil, service.OpenAIQuotaCostSummary{}, now)
	require.Equal(t, 5, unknown.OAuthQuota.FiveHour.TotalAccounts)
	require.Nil(t, unknown.OAuthQuota.FiveHour.RemainingCost)
}
