package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountUsageService_GetOfficialUsageAggregatesCachesAndFallsBack(t *testing.T) {
	var requests atomic.Int32
	var fail atomic.Bool
	now := time.Now().UTC()
	day := now.AddDate(0, 0, -1).Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		require.Equal(t, "workspace-id", r.Header.Get("chatgpt-account-id"))
		require.Equal(t, "day", r.URL.Query().Get("group_by"))
		if fail.Load() {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/counts":
			_, _ = fmt.Fprintf(w, `{"data":[{"date":%q,"totals":{"users":1,"threads":2,"turns":3,"credits":25,"uncached_text_input_tokens":100,"cached_text_input_tokens":200,"text_output_tokens":300,"text_total_tokens":600},"clients":[{"client_id":"CODEX_CLI","turns":2,"credits":15,"text_total_tokens":400},{"client_id":"CODEX_UNKNOWN_DEFAULT","turns":1,"credits":10,"text_total_tokens":200}]}]}`, day)
		case "/breakdown":
			_, _ = fmt.Fprintf(w, `{"data":[{"date":%q,"models":[{"model":"gpt-test","speed":"standard","credits":75},{"model":"gpt-test","speed":"fast","credits":25}]}]}`, day)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	oldCounts, oldBreakdown := officialUsageCountsURL, officialUsageBreakdownURL
	officialUsageCountsURL, officialUsageBreakdownURL = server.URL+"/counts", server.URL+"/breakdown"
	defer func() { officialUsageCountsURL, officialUsageBreakdownURL = oldCounts, oldBreakdown }()

	account := Account{
		ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "access-token", "chatgpt_account_id": "workspace-id"},
		Extra: map[string]any{
			"codex_7d_used_percent":  20.0,
			"codex_7d_reset_at":      now.Add(3 * 24 * time.Hour).Format(time.RFC3339),
			"codex_usage_updated_at": now.Format(time.RFC3339),
		},
	}
	service := NewAccountUsageService(&stubOpenAIAccountRepo{accounts: []Account{account}}, nil, nil, nil, nil, nil, nil, nil, NewUsageCache(), nil, nil)

	result, err := service.GetOfficialUsage(context.Background(), account.ID, "30d", false)
	require.NoError(t, err)
	require.Equal(t, OfficialUsageStateOK, result.State)
	require.Equal(t, int32(2), requests.Load())
	require.InDelta(t, 1.0, result.Totals.USD, 0.0001)
	require.Equal(t, int64(600), result.Totals.Tokens)
	require.Len(t, result.Clients, 2)
	require.Len(t, result.Models, 2)
	require.InDelta(t, 0.75, result.Models[0].USD, 0.0001)
	require.True(t, result.Cycle.Available)
	require.InDelta(t, 5.0, result.Cycle.EstimatedUSD, 0.0001)

	_, err = service.GetOfficialUsage(context.Background(), account.ID, "30d", false)
	require.NoError(t, err)
	require.Equal(t, int32(2), requests.Load(), "fresh cache should avoid upstream calls")

	fail.Store(true)
	stale, err := service.GetOfficialUsage(context.Background(), account.ID, "30d", true)
	require.NoError(t, err)
	require.Equal(t, OfficialUsageStateStale, stale.State)
	require.True(t, stale.Stale)
	require.Contains(t, stale.RefreshError, "频繁")
	require.Equal(t, int32(4), requests.Load())
}

func TestAccountUsageService_GetOfficialUsageRejectsMissingToken(t *testing.T) {
	account := Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	service := NewAccountUsageService(&stubOpenAIAccountRepo{accounts: []Account{account}}, nil, nil, nil, nil, nil, nil, nil, NewUsageCache(), nil, nil)
	result, err := service.GetOfficialUsage(context.Background(), account.ID, "7d", false)
	require.NoError(t, err)
	require.Equal(t, OfficialUsageStateUnauthorized, result.State)
	require.Empty(t, result.Daily)
}

func TestAccountUsageService_GetOfficialUsageClassifiesUpstreamStatus(t *testing.T) {
	var status atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(status.Load()))
	}))
	defer server.Close()
	oldCounts, oldBreakdown := officialUsageCountsURL, officialUsageBreakdownURL
	officialUsageCountsURL, officialUsageBreakdownURL = server.URL, server.URL
	defer func() { officialUsageCountsURL, officialUsageBreakdownURL = oldCounts, oldBreakdown }()
	account := Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token"}}

	for _, test := range []struct {
		status int
		want   OfficialUsageState
	}{
		{http.StatusUnauthorized, OfficialUsageStateUnauthorized},
		{http.StatusTooManyRequests, OfficialUsageStateRateLimited},
		{http.StatusGatewayTimeout, OfficialUsageStateTimeout},
		{http.StatusInternalServerError, OfficialUsageStateUnavailable},
	} {
		status.Store(int32(test.status))
		service := NewAccountUsageService(&stubOpenAIAccountRepo{accounts: []Account{account}}, nil, nil, nil, nil, nil, nil, nil, NewUsageCache(), nil, nil)
		result, err := service.GetOfficialUsage(context.Background(), account.ID, "7d", false)
		require.NoError(t, err)
		require.Equal(t, test.want, result.State)
	}
}
