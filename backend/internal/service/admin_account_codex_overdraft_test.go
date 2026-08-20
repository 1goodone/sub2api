package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAccountForCreateDropsCodexQuotaOverdraftState(t *testing.T) {
	extra := map[string]any{
		CodexQuotaOverdraftEnabledExtraKey: true,
		CodexQuotaOverdraftProbeExtraKey:   map[string]any{"status": "passed"},
		"custom":                           "keep",
	}

	account, err := buildAccountForCreate(&CreateAccountInput{
		Name:     "new-codex-account",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
	}, extra)

	require.NoError(t, err)
	require.NotContains(t, account.Extra, CodexQuotaOverdraftEnabledExtraKey)
	require.NotContains(t, account.Extra, CodexQuotaOverdraftProbeExtraKey)
	require.Equal(t, "keep", account.Extra["custom"])
}

func TestAdminUpdateAccountPreservesCodexQuotaOverdraftStateWhenOmitted(t *testing.T) {
	const accountID int64 = 901
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Name:     "before",
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				CodexQuotaOverdraftEnabledExtraKey: true,
				CodexQuotaOverdraftProbeExtraKey:   map[string]any{"status": "passed"},
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Name:  "after",
		Extra: map[string]any{"custom": "value"},
	})

	require.NoError(t, err)
	require.Equal(t, "after", updated.Name)
	require.Equal(t, true, updated.Extra[CodexQuotaOverdraftEnabledExtraKey])
	require.Equal(t, map[string]any{"status": "passed"}, updated.Extra[CodexQuotaOverdraftProbeExtraKey])
	require.Equal(t, "value", updated.Extra["custom"])
}

func TestAdminUpdateAccountExtraRejectsCodexQuotaOverdraftForIneligibleAccount(t *testing.T) {
	const accountID int64 = 902
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
	}}

	err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), accountID, map[string]any{
		CodexQuotaOverdraftEnabledExtraKey: true,
	})

	require.Error(t, err)
	require.Empty(t, repo.updates)
}

func TestAdminUpdateAccountExtraRejectsNonBooleanCodexQuotaOverdraft(t *testing.T) {
	const accountID int64 = 903
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	}}

	err := (&adminServiceImpl{accountRepo: repo}).UpdateAccountExtra(context.Background(), accountID, map[string]any{
		CodexQuotaOverdraftEnabledExtraKey: "true",
	})

	require.Error(t, err)
	require.Empty(t, repo.updates)
}

func TestDisableCodexQuotaOverdraftFallbackClearsProbeState(t *testing.T) {
	const accountID int64 = 904
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				CodexQuotaOverdraftEnabledExtraKey: true,
				CodexQuotaOverdraftProbeExtraKey:   map[string]any{"status": "passed"},
			},
		},
	}}

	err := (&adminServiceImpl{accountRepo: repo}).DisableCodexQuotaOverdraft(context.Background(), accountID)

	require.NoError(t, err)
	require.Equal(t, false, repo.accounts[accountID].Extra[CodexQuotaOverdraftEnabledExtraKey])
	require.Nil(t, repo.accounts[accountID].Extra[CodexQuotaOverdraftProbeExtraKey])
	require.Len(t, repo.updates[accountID], 1)
}
