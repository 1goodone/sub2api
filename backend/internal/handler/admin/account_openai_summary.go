package admin

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type openAIAccountTypeCounts struct {
	OAuth  int `json:"oauth"`
	APIKey int `json:"apikey"`
}

type openAIOAuthStatusCounts struct {
	Normal      int `json:"normal"`
	RateLimited int `json:"rate_limited"`
	Error       int `json:"error"`
}

type openAIConcurrencyCount struct {
	Used  int `json:"used"`
	Total int `json:"total"`
}

type openAIConcurrencyCounts struct {
	OAuth  openAIConcurrencyCount `json:"oauth"`
	APIKey openAIConcurrencyCount `json:"apikey"`
}

type openAIAccountSummary struct {
	Accounts    openAIAccountTypeCounts        `json:"accounts"`
	OAuthStatus openAIOAuthStatusCounts        `json:"oauth_status"`
	OAuthQuota  service.OpenAIQuotaCostSummary `json:"oauth_quota"`
	Concurrency openAIConcurrencyCounts        `json:"concurrency"`
}

func buildOpenAIAccountSummary(accounts []service.Account, currentConcurrency map[int64]int, quotaCosts service.OpenAIQuotaCostSummary, now time.Time) openAIAccountSummary {
	summary := openAIAccountSummary{OAuthQuota: quotaCosts}
	for i := range accounts {
		account := &accounts[i]
		switch account.Type {
		case service.AccountTypeOAuth:
			summary.Accounts.OAuth++
			summary.Concurrency.OAuth.Total += account.Concurrency
			summary.Concurrency.OAuth.Used += currentConcurrency[account.ID]

			isRateLimited := account.Status == service.StatusActive && account.RateLimitResetAt != nil && account.RateLimitResetAt.After(now) &&
				(account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(now))
			isNormal := account.Status == service.StatusActive && account.Schedulable && !isRateLimited &&
				(account.TempUnschedulableUntil == nil || !account.TempUnschedulableUntil.After(now))
			if isNormal {
				summary.OAuthStatus.Normal++
			}
			if isRateLimited {
				summary.OAuthStatus.RateLimited++
			}
			if account.Status == service.StatusError {
				summary.OAuthStatus.Error++
			}
		case service.AccountTypeAPIKey:
			summary.Accounts.APIKey++
			summary.Concurrency.APIKey.Total += account.Concurrency
			summary.Concurrency.APIKey.Used += currentConcurrency[account.ID]
		}
	}
	summary.OAuthQuota.FiveHour.TotalAccounts = summary.Accounts.OAuth
	summary.OAuthQuota.SevenDay.TotalAccounts = summary.Accounts.OAuth
	return summary
}

// GetOpenAIAccountSummary returns full-pool statistics for the account cards.
// GET /api/v1/admin/accounts/openai-summary
func (h *AccountHandler) GetOpenAIAccountSummary(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))

	var groupID int64
	if groupIDStr := c.Query("group"); groupIDStr != "" {
		if groupIDStr == accountListGroupUngroupedQueryValue {
			groupID = service.AccountListGroupUngrouped
		} else {
			parsed, err := strconv.ParseInt(groupIDStr, 10, 64)
			if err != nil || parsed < 0 {
				response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
				return
			}
			groupID = parsed
		}
	}

	accounts, err := h.adminService.ListAccountsForSchedulerScoreFilter(
		c.Request.Context(), service.PlatformOpenAI, "", "", search, groupID, privacyMode,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	currentConcurrency := make(map[int64]int)
	if h.concurrencyService != nil && len(accounts) > 0 {
		ids := make([]int64, 0, len(accounts))
		for i := range accounts {
			if accounts[i].Type == service.AccountTypeOAuth || accounts[i].Type == service.AccountTypeAPIKey {
				ids = append(ids, accounts[i].ID)
			}
		}
		if counts, concurrencyErr := h.concurrencyService.GetAccountConcurrencyBatch(c.Request.Context(), ids); concurrencyErr == nil && counts != nil {
			currentConcurrency = counts
		}
	}

	now := time.Now().UTC()
	quotaCosts := service.OpenAIQuotaCostSummary{}
	if h.accountUsageService != nil {
		quotaCosts, err = h.accountUsageService.GetOpenAIQuotaCostSummary(c.Request.Context(), accounts, now)
		if err != nil {
			slog.Warn("openai_account_summary_quota_cost_failed", "error", err)
			quotaCosts = service.OpenAIQuotaCostSummary{}
		}
	}

	response.Success(c, buildOpenAIAccountSummary(accounts, currentConcurrency, quotaCosts, now))
}
