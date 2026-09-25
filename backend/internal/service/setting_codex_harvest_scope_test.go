package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexTicketHarvestScope(t *testing.T) {
	scope, err := NormalizeCodexTicketHarvestScope(CodexTicketHarvestScope{
		Mode:     "selected",
		GroupIDs: []int64{9, 2, 9, 1},
	})
	require.NoError(t, err)
	require.Equal(t, CodexTicketHarvestScope{Mode: "selected", GroupIDs: []int64{1, 2, 9}, AccountPolicy: CodexHarvestSchedulableOnly}, scope)

	all, err := NormalizeCodexTicketHarvestScope(CodexTicketHarvestScope{})
	require.NoError(t, err)
	require.Equal(t, "all", all.Mode)

	for _, invalid := range []CodexTicketHarvestScope{
		{Mode: "unknown"},
		{Mode: "selected", GroupIDs: []int64{0}},
	} {
		_, err = NormalizeCodexTicketHarvestScope(invalid)
		require.Error(t, err)
	}
}

func TestGetCodexTicketHarvestScopeFailsClosedOnStorageOrJSONError(t *testing.T) {
	t.Run("storage error", func(t *testing.T) {
		repo := &codexTicketSettingRepo{
			codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{values: map[string]string{}},
			err:                          errors.New("database unavailable"),
		}
		svc := NewSettingService(repo, &config.Config{})
		_, err := svc.GetCodexTicketHarvestScope(context.Background())
		require.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		repo := &codexTicketSettingRepo{codexPolicyMigrationRepoStub: &codexPolicyMigrationRepoStub{
			values: map[string]string{SettingKeyOpenAICodexTicketHarvestScope: "not-json"},
		}}
		svc := NewSettingService(repo, &config.Config{})
		_, err := svc.GetCodexTicketHarvestScope(context.Background())
		require.Error(t, err)
	})
}

func TestCodexTicketHarvestScopeMatchesGroupsAndPriority(t *testing.T) {
	scope := CodexTicketHarvestScope{Mode: "selected", GroupIDs: []int64{20}}
	account := &Account{
		GroupIDs: []int64{10, 20},
		Priority: 9,
		AccountGroups: []AccountGroup{
			{GroupID: 10, Priority: 8},
			{GroupID: 20, Priority: 3},
		},
	}
	require.True(t, scope.includes(account))
	require.Equal(t, 3, scope.priority(account))

	account.GroupIDs = []int64{10}
	require.False(t, scope.includes(account))
}
