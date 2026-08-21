package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildAccountForCreateEnablesCodexQuotaOverdraftForOpenAIOAuth(t *testing.T) {
	for _, supplied := range []any{nil, false, true} {
		t.Run(fmt.Sprint(supplied), func(t *testing.T) {
			extra := map[string]any{
				CodexQuotaOverdraftProbeExtraKey: map[string]any{"status": "passed"},
				"custom":                         "keep",
			}
			if supplied != nil {
				extra[CodexQuotaOverdraftEnabledExtraKey] = supplied
			}

			account, err := buildAccountForCreate(&CreateAccountInput{
				Name:     "new-codex-account",
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
			}, extra)

			require.NoError(t, err)
			require.Equal(t, true, account.Extra[CodexQuotaOverdraftEnabledExtraKey])
			require.NotContains(t, account.Extra, CodexQuotaOverdraftProbeExtraKey)
			require.Equal(t, "keep", account.Extra["custom"])
		})
	}
}

func TestPrepareCodexQuotaOverdraftForCreateRejectsIneligibleAccounts(t *testing.T) {
	parentID := int64(1)
	accounts := map[string]*Account{
		"openai api key": {Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		"other oauth":    {Platform: PlatformAnthropic, Type: AccountTypeOAuth},
		"spark shadow":   {Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parentID},
	}

	for name, account := range accounts {
		t.Run(name, func(t *testing.T) {
			account.Extra = map[string]any{
				CodexQuotaOverdraftEnabledExtraKey: true,
				CodexQuotaOverdraftProbeExtraKey:   map[string]any{"status": "passed"},
				"custom":                           "keep",
			}

			PrepareCodexQuotaOverdraftForCreate(account)

			require.NotContains(t, account.Extra, CodexQuotaOverdraftEnabledExtraKey)
			require.NotContains(t, account.Extra, CodexQuotaOverdraftProbeExtraKey)
			require.Equal(t, "keep", account.Extra["custom"])
		})
	}
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

func TestAdminUpdateAccountPreservesManualCodexQuotaOverdraftDisable(t *testing.T) {
	const accountID int64 = 905
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
		accountID: {
			ID:       accountID,
			Name:     "before",
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra: map[string]any{
				CodexQuotaOverdraftEnabledExtraKey: false,
			},
		},
	}}

	updated, err := (&adminServiceImpl{accountRepo: repo}).UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
		Name:  "after",
		Extra: map[string]any{"custom": "value"},
	})

	require.NoError(t, err)
	require.Equal(t, false, updated.Extra[CodexQuotaOverdraftEnabledExtraKey])
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
