package service

import (
	"math"
	"strings"
	"time"
)

const (
	OpenAIQuotaWindow5h = "5h"
	OpenAIQuotaWindow7d = "7d"
)

// MatchesOpenAIQuotaWindow reports whether an OAuth account is currently
// rate-limited by the requested canonical Codex quota window.
func MatchesOpenAIQuotaWindow(account *Account, window string, now time.Time) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.Status != StatusActive {
		return false
	}
	if account.RateLimitResetAt == nil || !account.RateLimitResetAt.After(now) {
		return false
	}
	if account.TempUnschedulableUntil != nil && account.TempUnschedulableUntil.After(now) {
		return false
	}

	window = strings.ToLower(strings.TrimSpace(window))
	if window != OpenAIQuotaWindow5h && window != OpenAIQuotaWindow7d {
		return false
	}
	candidate := openAIThresholdCandidate(account.Extra, window, now)
	if candidate == nil || candidate.until == nil || !candidate.until.After(now) {
		return false
	}

	// A fully exhausted snapshot is definitive. For configurable scheduling
	// thresholds below 100%, the persisted cooldown points at the winning
	// canonical window, so matching reset boundaries preserves runtime truth.
	if candidate.usedPercent >= 100 {
		return true
	}
	return math.Abs(candidate.until.Sub(*account.RateLimitResetAt).Seconds()) <= 120
}
