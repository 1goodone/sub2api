package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type mixedRoutingRateRepoStub struct {
	UserGroupRateRepository
	rates map[int64]float64
}

func (s *mixedRoutingRateRepoStub) GetByUserID(context.Context, int64) (map[int64]float64, error) {
	return s.rates, nil
}

type mixedRoutingGroupRepoStub struct {
	GroupRepository
	groups map[int64]*Group
}

func (s *mixedRoutingGroupRepoStub) GetByID(_ context.Context, id int64) (*Group, error) {
	group := s.groups[id]
	if group == nil {
		return nil, ErrGroupNotFound
	}
	return group, nil
}

type mixedRoutingSubscriptionRepoStub struct {
	UserSubscriptionRepository
}

func (s *mixedRoutingSubscriptionRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	return &UserSubscription{ID: groupID + 100, UserID: userID, GroupID: groupID}, nil
}

type mixedRoutingHealthStub struct {
	unavailable map[int64]bool
	err         error
}

func (s *mixedRoutingHealthStub) UnavailableGroupIDs(context.Context, []int64) (map[int64]bool, error) {
	return s.unavailable, s.err
}

func mixedRoutingTestGroup(id int64, platform string, rate float64) *Group {
	return &Group{
		ID:               id,
		Name:             "group",
		Platform:         platform,
		RateMultiplier:   rate,
		Status:           StatusActive,
		Hydrated:         true,
		SubscriptionType: SubscriptionTypeStandard,
	}
}

func mixedRoutingTestKey(groups ...*Group) *APIKey {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return &APIKey{
		ID:            10,
		UserID:        20,
		MixedMode:     true,
		MixedGroupIDs: ids,
		MixedGroups:   groups,
		User:          &User{ID: 20, Status: StatusActive},
	}
}

func TestNormalizeMixedGroupConfig(t *testing.T) {
	ids, err := normalizeMixedGroupConfig(false, []int64{1, 2})
	require.NoError(t, err)
	require.Empty(t, ids)

	valid := []int64{3, 1, 2}
	ids, err = normalizeMixedGroupConfig(true, valid)
	require.NoError(t, err)
	require.Equal(t, valid, ids)

	for name, candidate := range map[string][]int64{
		"one group":         {1},
		"more than eight":   {1, 2, 3, 4, 5, 6, 7, 8, 9},
		"duplicate group":   {1, 1},
		"nonpositive group": {0, 2},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := normalizeMixedGroupConfig(true, candidate)
			require.Error(t, err)
		})
	}
}

func TestValidateMixedGroupsRequiresSamePlatformAndPermission(t *testing.T) {
	public := mixedRoutingTestGroup(1, PlatformOpenAI, 1)
	otherPlatform := mixedRoutingTestGroup(2, PlatformGrok, 1)
	exclusive := mixedRoutingTestGroup(3, PlatformOpenAI, 1)
	exclusive.IsExclusive = true
	svc := &APIKeyService{groupRepo: &mixedRoutingGroupRepoStub{groups: map[int64]*Group{
		1: public,
		2: otherPlatform,
		3: exclusive,
	}}}
	user := &User{ID: 20}

	require.ErrorIs(t, svc.validateMixedGroups(context.Background(), user, []int64{1, 2}), ErrMixedGroupPlatform)
	require.ErrorIs(t, svc.validateMixedGroups(context.Background(), user, []int64{1, 3}), ErrGroupNotAllowed)

	user.AllowedGroups = []int64{3}
	require.NoError(t, svc.validateMixedGroups(context.Background(), user, []int64{1, 3}))
}

func TestPrepareAPIKeyRoutingUsesActualRateAndResetsPerRequest(t *testing.T) {
	standard := mixedRoutingTestGroup(1, PlatformOpenAI, 1)
	peak := mixedRoutingTestGroup(2, PlatformOpenAI, 0.4)
	peak.SubscriptionType = SubscriptionTypeSubscription
	peak.PeakRateEnabled = true
	peak.PeakStart = "12:00"
	peak.PeakEnd = "13:00"
	peak.PeakRateMultiplier = 3
	key := mixedRoutingTestKey(peak, standard)
	svc := &APIKeyService{
		userGroupRateRepo: &mixedRoutingRateRepoStub{rates: map[int64]float64{peak.ID: 0.4}},
		userSubRepo:       &mixedRoutingSubscriptionRepoStub{},
	}

	now := time.Date(2026, time.August, 24, 12, 30, 0, 0, timezone.Location())
	effective, state, err := svc.prepareAPIKeyRoutingAt(context.Background(), key, now)
	require.NoError(t, err)
	require.Equal(t, standard.ID, *effective.GroupID, "0.4 user rate x 3 peak rate must sort after 1.0")
	ids, ok := state.CandidateGroupIDs(effective.GroupID)
	require.True(t, ok)
	require.Equal(t, []int64{standard.ID, peak.ID}, ids)

	require.True(t, state.ApplyGroup(peak.ID))
	require.False(t, state.ApplyGroup(standard.ID), "one request must not downgrade after an upgrade")

	nextKey, nextState, err := svc.prepareAPIKeyRoutingAt(context.Background(), key, now)
	require.NoError(t, err)
	require.Equal(t, standard.ID, *nextKey.GroupID)
	nextIDs, ok := nextState.CandidateGroupIDs(nextKey.GroupID)
	require.True(t, ok)
	require.Equal(t, []int64{standard.ID, peak.ID}, nextIDs)
}

func TestPrepareAPIKeyRoutingHealthIsFailOpenAndRecovers(t *testing.T) {
	cheap := mixedRoutingTestGroup(1, PlatformOpenAI, 1)
	expensive := mixedRoutingTestGroup(2, PlatformOpenAI, 2)
	key := mixedRoutingTestKey(cheap, expensive)
	health := &mixedRoutingHealthStub{unavailable: map[int64]bool{cheap.ID: true}}
	svc := &APIKeyService{routingHealth: health}

	effective, _, err := svc.PrepareAPIKeyRouting(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, expensive.ID, *effective.GroupID)

	health.unavailable = nil
	effective, _, err = svc.PrepareAPIKeyRouting(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, cheap.ID, *effective.GroupID)

	health.err = errors.New("monitor unavailable")
	effective, _, err = svc.PrepareAPIKeyRouting(context.Background(), key)
	require.NoError(t, err)
	require.Equal(t, cheap.ID, *effective.GroupID, "health read failures must fail open")
}

func TestMixedGroupBillingErrorsDistinguishGlobalAndCandidateFailures(t *testing.T) {
	for _, err := range []error{
		errors.New("unknown billing failure"),
		ErrBillingServiceUnavailable,
		ErrInsufficientBalance,
		ErrUserPlatformDailyQuotaExhausted,
		ErrUserPlatformWeeklyQuotaExhausted,
		ErrUserPlatformMonthlyQuotaExhausted,
		ErrUserRPMExceeded,
		ErrAPIKeyRateLimit5hExceeded,
		ErrAPIKeyRateLimit1dExceeded,
		ErrAPIKeyRateLimit7dExceeded,
	} {
		require.False(t, shouldTryNextMixedGroupAfterBillingError(err), "%v must stop routing", err)
	}

	for _, err := range []error{
		ErrSubscriptionInvalid,
		ErrDailyLimitExceeded,
		ErrWeeklyLimitExceeded,
		ErrMonthlyLimitExceeded,
		ErrGroupRPMExceeded,
	} {
		require.True(t, shouldTryNextMixedGroupAfterBillingError(err), "%v must allow the next candidate", err)
	}
	require.True(t, shouldTryNextMixedGroupAfterSelectionError(ErrNoAvailableAccounts))
	require.False(t, shouldTryNextMixedGroupAfterSelectionError(errors.New("invalid request")))
}
