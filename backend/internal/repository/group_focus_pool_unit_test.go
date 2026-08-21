package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGroupRepositoryUpdatePersistsFocusPoolSQLite(t *testing.T) {
	apiKeyRepo, client := newAPIKeyRepoSQLite(t)
	repo := newGroupRepositoryWithSQL(client, apiKeyRepo.sql)
	ctx := context.Background()
	group := &service.Group{
		Name:             "focus-pool",
		Platform:         service.PlatformOpenAI,
		RateMultiplier:   1,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	require.NoError(t, repo.Create(ctx, group))

	firstID := mustCreateAPIKeyRepoAccount(t, ctx, client, "focus-first")
	secondID := mustCreateAPIKeyRepoAccount(t, ctx, client, "focus-second")
	for priority, accountID := range []int64{firstID, secondID} {
		_, err := client.AccountGroup.Create().
			SetAccountID(accountID).
			SetGroupID(group.ID).
			SetPriority(priority).
			Save(ctx)
		require.NoError(t, err)
	}

	group.FocusAccountIDs = []int64{secondID, firstID}
	require.NoError(t, repo.Update(ctx, group))

	got, err := repo.GetByIDLite(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, []int64{secondID, firstID}, got.FocusAccountIDs)
	require.NotNil(t, got.FocusAccountID)
	require.Equal(t, secondID, *got.FocusAccountID)

	focused, err := client.AccountGroup.Query().
		Where(accountgroup.GroupIDEQ(group.ID), accountgroup.IsFocus(true)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, focused)
}
