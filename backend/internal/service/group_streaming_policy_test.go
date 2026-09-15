//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupStreamingPolicyPersistenceAndCache(t *testing.T) {
	ctx := context.Background()
	repo := &groupRepoStubForAdmin{createID: 3}
	invalidator := &authCacheInvalidatorStub{}
	svc := &adminServiceImpl{groupRepo: repo, authCacheInvalidator: invalidator}
	group, err := svc.CreateGroup(ctx, &CreateGroupInput{
		Name: "streaming", Platform: PlatformOpenAI, RateMultiplier: 1, DisableNonStreaming: true,
	})
	require.NoError(t, err)
	require.True(t, group.DisableNonStreaming)
	repo.getByID = group
	group, err = svc.UpdateGroup(ctx, group.ID, &UpdateGroupInput{Name: "renamed"})
	require.NoError(t, err)
	require.True(t, group.DisableNonStreaming, "omitted field must preserve the flag")

	keyService := &APIKeyService{}
	key := &APIKey{ID: 1, User: &User{ID: 2}, GroupID: &group.ID, Group: group}
	data, err := json.Marshal(&APIKeyAuthCacheEntry{Snapshot: keyService.snapshotFromAPIKey(ctx, key)})
	require.NoError(t, err)
	var entry APIKeyAuthCacheEntry
	require.NoError(t, json.Unmarshal(data, &entry))
	cached, used, err := keyService.applyAuthCacheEntry("test", &entry)
	require.NoError(t, err)
	require.True(t, used)
	require.True(t, cached.Group.DisableNonStreaming)
	entry.Snapshot.Version--
	_, used, err = keyService.applyAuthCacheEntry("test", &entry)
	require.NoError(t, err)
	require.False(t, used, "old cache schemas must reload the group")

	disabled := false
	group, err = svc.UpdateGroup(ctx, group.ID, &UpdateGroupInput{DisableNonStreaming: &disabled})
	require.NoError(t, err)
	require.False(t, group.DisableNonStreaming)
	require.Equal(t, []int64{3, 3}, invalidator.groupIDs)
}

func TestGroupStreamingPolicyFallbackAndMixedRouting(t *testing.T) {
	fallbackID := int64(2)
	primary := &Group{ID: 1, Hydrated: true, ClaudeCodeOnly: true, FallbackGroupID: &fallbackID}
	fallback := &Group{ID: fallbackID, Hydrated: true, DisableNonStreaming: true}
	svc := &GatewayService{groupRepo: &groupRepoStubForAdmin{getByIDByID: map[int64]*Group{1: primary, 2: fallback}}}
	_, _, err := svc.resolveGatewayGroup(WithNonStreamingTextRequest(context.Background()), &primary.ID)
	require.ErrorIs(t, err, ErrNonStreamingDisabled)
	resolved, _, err := svc.resolveGatewayGroup(context.Background(), &primary.ID)
	require.NoError(t, err)
	require.Equal(t, fallback.ID, resolved.ID, "streaming keeps fallback routing")
	require.True(t, shouldTryNextMixedGroupAfterSelectionError(ErrNonStreamingDisabled))

	key := &APIKey{MixedMode: true}
	state := &APIKeyRoutingState{apiKey: key, candidates: []APIKeyRoutingCandidate{
		{Group: fallback}, {Group: &Group{ID: 3}},
	}}
	ctx := WithNonStreamingTextRequest(WithAPIKeyRoutingState(context.Background(), state))
	require.True(t, RestrictAPIKeyRoutingCandidates(ctx, func(g *Group) bool { return CheckGroupStreamingPolicy(ctx, g) == nil }))
	require.Equal(t, int64(3), key.Group.ID)
	require.Len(t, state.candidates, 1)
	require.True(t, RestrictAPIKeyRoutingCandidates(ctx, func(g *Group) bool { return true }))
	require.Len(t, state.candidates, 1, "later filters cannot restore a denied group")
}
