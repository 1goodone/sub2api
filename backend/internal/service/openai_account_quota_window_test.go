package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMatchesOpenAIQuotaWindow(t *testing.T) {
	now := time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	fiveHourReset := now.Add(2 * time.Hour)
	account := &Account{
		Platform:         PlatformOpenAI,
		Type:             AccountTypeOAuth,
		Status:           StatusActive,
		Schedulable:      true,
		RateLimitResetAt: &fiveHourReset,
		Extra: map[string]any{
			"codex_5h_used_percent": 95.0,
			"codex_5h_reset_at":     fiveHourReset.Format(time.RFC3339),
			"codex_7d_used_percent": 40.0,
			"codex_7d_reset_at":     now.Add(72 * time.Hour).Format(time.RFC3339),
		},
	}

	require.True(t, MatchesOpenAIQuotaWindow(account, OpenAIQuotaWindow5h, now))
	require.False(t, MatchesOpenAIQuotaWindow(account, OpenAIQuotaWindow7d, now))

	account.Extra["codex_7d_used_percent"] = 100.0
	require.True(t, MatchesOpenAIQuotaWindow(account, OpenAIQuotaWindow7d, now))

	tempUntil := now.Add(time.Hour)
	account.TempUnschedulableUntil = &tempUntil
	require.False(t, MatchesOpenAIQuotaWindow(account, OpenAIQuotaWindow5h, now))
}
