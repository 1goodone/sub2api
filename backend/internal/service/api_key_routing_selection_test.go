//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMixedGroupSelectionUpgradesAndNextRequestReturnsToCheapest(t *testing.T) {
	cheap := mixedRoutingTestGroup(1, PlatformOpenAI, 1)
	fallback := mixedRoutingTestGroup(2, PlatformOpenAI, 2)
	key := mixedRoutingTestKey(cheap, fallback)
	keyService := &APIKeyService{}
	groupRepo := &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{
		cheap.ID:    cheap,
		fallback.ID: fallback,
	}}
	fallbackAccount := Account{
		ID:          22,
		Platform:    PlatformOpenAI,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		AccountGroups: []AccountGroup{
			{GroupID: fallback.ID},
		},
	}

	effective, state, err := keyService.PrepareAPIKeyRouting(context.Background(), key)
	require.NoError(t, err)
	gateway := &GatewayService{
		accountRepo: newGroupAwareMockRepo([]Account{fallbackAccount}),
		groupRepo:   groupRepo,
		cache:       &mockGatewayCacheForPlatform{},
		cfg:         testConfig(),
	}
	account, err := gateway.SelectAccountForModelWithExclusions(
		WithAPIKeyRoutingState(context.Background(), state),
		effective.GroupID,
		"",
		"",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, fallbackAccount.ID, account.ID)
	require.Equal(t, fallback.ID, *effective.GroupID)

	cheapAccount := Account{
		ID:          11,
		Platform:    PlatformOpenAI,
		Priority:    1,
		Status:      StatusActive,
		Schedulable: true,
		AccountGroups: []AccountGroup{
			{GroupID: cheap.ID},
		},
	}
	nextKey, nextState, err := keyService.PrepareAPIKeyRouting(context.Background(), key)
	require.NoError(t, err)
	nextGateway := &GatewayService{
		accountRepo: newGroupAwareMockRepo([]Account{cheapAccount, fallbackAccount}),
		groupRepo:   groupRepo,
		cache:       &mockGatewayCacheForPlatform{},
		cfg:         testConfig(),
	}
	account, err = nextGateway.SelectAccountForModelWithExclusions(
		WithAPIKeyRoutingState(context.Background(), nextState),
		nextKey.GroupID,
		"",
		"",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, cheapAccount.ID, account.ID)
	require.Equal(t, cheap.ID, *nextKey.GroupID)
}
