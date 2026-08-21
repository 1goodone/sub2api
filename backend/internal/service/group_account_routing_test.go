package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestGroupAccountRoutingPreferenceRank(t *testing.T) {
	focusID := int64(3)
	secondFocusID := int64(4)
	auth := &Account{ID: 1, Type: AccountTypeOAuth}
	api := &Account{ID: 2, Type: AccountTypeUpstream}
	focus := &Account{ID: focusID, Type: AccountTypeAPIKey}
	secondFocus := &Account{ID: secondFocusID, Type: AccountTypeOAuth}

	group := &Group{
		FocusAccountIDs:       []int64{focusID, secondFocusID},
		AccountTypePreference: AccountTypePreferenceAuthFirst,
	}
	require.Equal(t, 0, accountRoutingPreferenceRank(group, focus))
	require.Equal(t, 0, accountRoutingPreferenceRank(group, secondFocus))
	require.Equal(t, 1, accountRoutingPreferenceRank(group, auth))
	require.Equal(t, 2, accountRoutingPreferenceRank(group, api))

	group.FocusAccountIDs = nil
	group.AccountTypePreference = AccountTypePreferenceAPIKeyFirst
	require.Equal(t, 1, accountRoutingPreferenceRank(group, api))
	require.Equal(t, 1, accountRoutingPreferenceRank(group, &Account{ID: 4, Type: AccountTypeBedrock}))
	require.Equal(t, 2, accountRoutingPreferenceRank(group, auth))
}

func TestGroupRoutingPreferenceOverridesOldStickySession(t *testing.T) {
	focusID := int64(9)
	secondFocusID := int64(10)
	group := &Group{FocusAccountIDs: []int64{focusID, secondFocusID}}

	require.True(t, groupFocusOverridesSticky(group, 4, nil))
	require.False(t, groupFocusOverridesSticky(group, focusID, nil))
	require.False(t, groupFocusOverridesSticky(group, secondFocusID, nil))
	require.False(t, groupFocusOverridesSticky(group, 4, []int64{4, 5}),
		"model routing must win when the focus account is outside the route")
	require.True(t, groupFocusOverridesSticky(group, 4, []int64{4, focusID}))

	require.True(t, groupAccountTypePreferenceOverridesSticky(
		&Group{AccountTypePreference: AccountTypePreferenceAuthFirst},
		&Account{Type: AccountTypeAPIKey},
	))
	require.False(t, groupAccountTypePreferenceOverridesSticky(
		&Group{AccountTypePreference: AccountTypePreferenceAuthFirst},
		&Account{Type: AccountTypeSetupToken},
	))
	require.True(t, groupAccountTypePreferenceOverridesSticky(
		&Group{AccountTypePreference: AccountTypePreferenceAPIKeyFirst},
		&Account{Type: AccountTypeOAuth},
	))
	require.False(t, groupAccountTypePreferenceOverridesSticky(
		&Group{AccountTypePreference: AccountTypePreferenceAPIKeyFirst},
		&Account{Type: AccountTypeBedrock},
	))
}

func TestFocusPoolUsesShortIdleStickyTTLOnlyForConfiguredGroups(t *testing.T) {
	require.Equal(t, focusPoolStickySessionTTL, routingPreferenceStickyTTL(
		&Group{FocusAccountIDs: []int64{1, 2}}, time.Hour,
	))
	require.Equal(t, time.Hour, routingPreferenceStickyTTL(&Group{}, time.Hour))
	require.Equal(t, 2*time.Minute, routingPreferenceStickyTTL(
		&Group{FocusAccountIDs: []int64{1}}, 2*time.Minute,
	))
}

func TestFilterByMinRoutingPreferenceSupportsFallbackTiers(t *testing.T) {
	focusID := int64(3)
	group := &Group{
		FocusAccountID:        &focusID,
		AccountTypePreference: AccountTypePreferenceAuthFirst,
	}
	accounts := []accountWithLoad{
		{account: &Account{ID: 1, Type: AccountTypeOAuth}},
		{account: &Account{ID: 2, Type: AccountTypeAPIKey}},
		{account: &Account{ID: focusID, Type: AccountTypeAPIKey}},
	}

	first := filterByMinRoutingPreference(accounts, group)
	require.Len(t, first, 1)
	require.Equal(t, focusID, first[0].account.ID)

	second := filterByMinRoutingPreference(accounts[:2], group)
	require.Len(t, second, 1)
	require.Equal(t, int64(1), second[0].account.ID)

	third := filterByMinRoutingPreference(accounts[1:2], group)
	require.Len(t, third, 1)
	require.Equal(t, int64(2), third[0].account.ID)
}

func TestDefaultRoutingPreferenceKeepsOfficialSelectionOrder(t *testing.T) {
	group := &Group{AccountTypePreference: AccountTypePreferenceDefault}
	require.Equal(t, 1, accountRoutingPreferenceRank(group, &Account{ID: 1, Type: AccountTypeOAuth}))
	require.Equal(t, 1, accountRoutingPreferenceRank(group, &Account{ID: 2, Type: AccountTypeAPIKey}))
	require.False(t, groupHasAccountRoutingPreference(group))

	older := &Account{ID: 1, Type: AccountTypeOAuth, Priority: 5}
	newer := &Account{ID: 2, Type: AccountTypeAPIKey, Priority: 0}
	require.True(t, isBetterAccountForGroup(group, newer, older, false))
}

func TestOpenAIGroupRoutingExpandsTopKAndMigratesOldSticky(t *testing.T) {
	focusID := int64(3)
	req := OpenAIAccountScheduleRequest{
		FocusAccountID:        focusID,
		AccountTypePreference: AccountTypePreferenceAuthFirst,
		StickyAccountID:       1,
		SessionHash:           "existing-session",
	}
	require.True(t, req.focusOverridesSticky())

	accounts := []*Account{
		{ID: 1, Type: AccountTypeOAuth, Priority: 0},
		{ID: 2, Type: AccountTypeAPIKey, Priority: 0},
		{ID: focusID, Type: AccountTypeAPIKey, Priority: 10},
	}
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1},
		2: {AccountID: 2},
		3: {AccountID: 3},
	}
	scheduler := &defaultOpenAIAccountScheduler{
		service: &OpenAIGatewayService{},
		stats:   newOpenAIAccountRuntimeStats(),
	}

	plan := scheduler.buildOpenAIAccountLoadPlan(context.Background(), req, accounts, loadMap)
	require.True(t, plan.includeOverflowFallback)
	require.Len(t, plan.selectionOrder, len(accounts))
	require.Equal(t, focusID, plan.selectionOrder[0].account.ID)
}

func TestOpenAIAccountTypePreferencesMigrateExistingSessions(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	tests := []struct {
		name       string
		preference string
		stickyID   int64
		wantID     int64
	}{
		{name: "OAuth first", preference: AccountTypePreferenceAuthFirst, stickyID: 412, wantID: 411},
		{name: "API key first", preference: AccountTypePreferenceAPIKeyFirst, stickyID: 411, wantID: 412},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groupID := int64(41)
			group := routingTestGroup(groupID)
			group.AccountTypePreference = tt.preference
			accounts := []Account{
				routingTestAccount(411, groupID, AccountTypeOAuth, 100),
				routingTestAccount(412, groupID, AccountTypeAPIKey, 0),
			}
			cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
			svc := routingTestOpenAIService(accounts, cache, schedulerTestConcurrencyCache{})
			sessionCacheKey := svc.openAISessionCacheKey(tt.name)
			cache.sessionBindings[sessionCacheKey] = tt.stickyID
			ctx := context.WithValue(context.Background(), ctxkey.Group, group)

			selection, decision, err := svc.SelectAccountWithScheduler(
				ctx, &groupID, "", tt.name, "", nil, OpenAIUpstreamTransportAny, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, tt.wantID, selection.Account.ID)
			require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
			require.Equal(t, tt.wantID, cache.sessionBindings[sessionCacheKey])
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

func TestGatewayAuthPreferenceMigratesExistingSession(t *testing.T) {
	groupID := int64(44)
	group := routingTestGroup(groupID)
	group.Platform = PlatformAnthropic
	group.AccountTypePreference = AccountTypePreferenceAuthFirst
	accounts := []Account{
		{
			ID: 441, Platform: PlatformAnthropic, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100,
			GroupIDs: []int64{groupID}, AccountGroups: []AccountGroup{{GroupID: groupID}},
		},
		{
			ID: 442, Platform: PlatformAnthropic, Type: AccountTypeAPIKey,
			Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
			GroupIDs: []int64{groupID}, AccountGroups: []AccountGroup{{GroupID: groupID}},
		},
	}

	t.Run("load aware", func(t *testing.T) {
		cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"existing-anthropic": 442}}
		cfg := &config.Config{}
		cfg.Gateway.Scheduling.LoadBatchEnabled = true
		svc := &GatewayService{
			accountRepo: routingTestGatewayAccountRepo{
				schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
			},
			cache:              cache,
			cfg:                cfg,
			concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
		}
		ctx := context.WithValue(context.Background(), ctxkey.Group, group)

		selection, err := svc.SelectAccountWithLoadAwareness(
			ctx, &groupID, "existing-anthropic", "", nil, "", 0,
		)
		require.NoError(t, err)
		require.NotNil(t, selection)
		require.Equal(t, int64(441), selection.Account.ID)
		require.Equal(t, int64(441), cache.sessionBindings["existing-anthropic"])
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	})

	t.Run("legacy single platform", func(t *testing.T) {
		cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"legacy-anthropic": 442}}
		svc := &GatewayService{
			accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
			groupRepo:   routingTestGroupRepo{group: group},
			cache:       cache,
		}
		ctx := context.WithValue(context.Background(), ctxkey.Group, group)
		ctx = context.WithValue(ctx, ctxkey.ForcePlatform, PlatformAnthropic)

		account, err := svc.selectAccountForModelWithPlatform(
			ctx, &groupID, "legacy-anthropic", "", nil, PlatformAnthropic,
		)
		require.NoError(t, err)
		require.Equal(t, int64(441), account.ID)
		require.Equal(t, int64(441), cache.sessionBindings["legacy-anthropic"])
	})

	t.Run("legacy mixed", func(t *testing.T) {
		cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"legacy-mixed": 442}}
		svc := &GatewayService{
			accountRepo: routingTestGatewayAccountRepo{
				schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
			},
			groupRepo: routingTestGroupRepo{group: group},
			cache:     cache,
		}
		ctx := context.WithValue(context.Background(), ctxkey.Group, group)

		account, err := svc.selectAccountWithMixedScheduling(
			ctx, &groupID, "legacy-mixed", "", nil, PlatformAnthropic,
		)
		require.NoError(t, err)
		require.Equal(t, int64(441), account.ID)
		require.Equal(t, int64(441), cache.sessionBindings["legacy-mixed"])
	})
}

func TestOpenAIFocusOverridesMovablePreviousResponse(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(42)
	focusID := int64(422)
	group := routingTestGroup(groupID)
	group.FocusAccountID = &focusID
	accounts := []Account{
		routingTestAccount(421, groupID, AccountTypeAPIKey, 0),
		routingTestAccount(focusID, groupID, AccountTypeAPIKey, 100),
	}
	cache := &schedulerTestGatewayCache{}
	svc := routingTestOpenAIService(accounts, cache, schedulerTestConcurrencyCache{})
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, "resp-old", 421, openaiStickySessionTTL))

	selection, decision, err := svc.SelectAccountWithSchedulerForCapability(
		ctx, &groupID, "resp-old", "existing-response-session", "", nil,
		OpenAIUpstreamTransportAny, "", false, true, false, PlatformOpenAI,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, focusID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIFocusFallsBackWhenConcurrencyIsFull(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(43)
	focusID := int64(431)
	group := routingTestGroup(groupID)
	group.FocusAccountID = &focusID
	group.AccountTypePreference = AccountTypePreferenceAuthFirst
	accounts := []Account{
		routingTestAccount(focusID, groupID, AccountTypeAPIKey, 0),
		routingTestAccount(432, groupID, AccountTypeOAuth, 0),
	}
	acquiredIDs := make([]int64, 0, 2)
	svc := routingTestOpenAIService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{
		acquireResults: map[int64]bool{focusID: false, 432: true},
		acquiredIDs:    &acquiredIDs,
	})
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	selection, _, err := svc.SelectAccountWithScheduler(
		ctx, &groupID, "", "focus-full", "", nil, OpenAIUpstreamTransportAny, false,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, int64(432), selection.Account.ID)
	require.GreaterOrEqual(t, len(acquiredIDs), 2)
	require.Equal(t, []int64{focusID, 432}, acquiredIDs[:2])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestOpenAIFocusPoolTriesOtherFocusBeforeNormalFallback(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(45)
	firstFocusID := int64(451)
	secondFocusID := int64(452)
	normalID := int64(453)
	group := routingTestGroup(groupID)
	group.FocusAccountIDs = []int64{firstFocusID, secondFocusID}
	accounts := []Account{
		routingTestAccount(firstFocusID, groupID, AccountTypeAPIKey, 0),
		routingTestAccount(secondFocusID, groupID, AccountTypeOAuth, 100),
		routingTestAccount(normalID, groupID, AccountTypeOAuth, 0),
	}

	t.Run("uses another focus account", func(t *testing.T) {
		acquiredIDs := make([]int64, 0, 3)
		svc := routingTestOpenAIService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{
			acquireResults: map[int64]bool{firstFocusID: false, secondFocusID: true, normalID: true},
			acquiredIDs:    &acquiredIDs,
		})
		ctx := context.WithValue(context.Background(), ctxkey.Group, group)

		selection, _, err := svc.SelectAccountWithScheduler(
			ctx, &groupID, "", "focus-pool", "", nil, OpenAIUpstreamTransportAny, false,
		)
		require.NoError(t, err)
		require.Equal(t, secondFocusID, selection.Account.ID)
		require.Equal(t, []int64{firstFocusID, secondFocusID}, acquiredIDs[:2])
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	})

	t.Run("falls back after the focus pool is exhausted", func(t *testing.T) {
		acquiredIDs := make([]int64, 0, 3)
		svc := routingTestOpenAIService(accounts, &schedulerTestGatewayCache{}, schedulerTestConcurrencyCache{
			acquireResults: map[int64]bool{firstFocusID: false, secondFocusID: false, normalID: true},
			acquiredIDs:    &acquiredIDs,
		})
		ctx := context.WithValue(context.Background(), ctxkey.Group, group)

		selection, _, err := svc.SelectAccountWithScheduler(
			ctx, &groupID, "", "focus-pool-fallback", "", nil, OpenAIUpstreamTransportAny, false,
		)
		require.NoError(t, err)
		require.Equal(t, normalID, selection.Account.ID)
		require.Equal(t, []int64{firstFocusID, secondFocusID, normalID}, acquiredIDs[:3])
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
	})
}

func TestOpenAIFocusPoolEscapesBusyStickyAccount(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	groupID := int64(46)
	firstFocusID := int64(461)
	secondFocusID := int64(462)
	group := routingTestGroup(groupID)
	group.FocusAccountIDs = []int64{firstFocusID, secondFocusID}
	accounts := []Account{
		routingTestAccount(firstFocusID, groupID, AccountTypeOAuth, 0),
		routingTestAccount(secondFocusID, groupID, AccountTypeOAuth, 100),
	}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{}}
	svc := routingTestOpenAIService(accounts, cache, schedulerTestConcurrencyCache{
		acquireResults: map[int64]bool{firstFocusID: false, secondFocusID: true},
	})
	sessionHash := "focus-sticky-busy"
	sessionKey := svc.openAISessionCacheKey(sessionHash)
	cache.sessionBindings[sessionKey] = firstFocusID
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	selection, decision, err := svc.SelectAccountWithScheduler(
		ctx, &groupID, "", sessionHash, "", nil, OpenAIUpstreamTransportAny, false,
	)
	require.NoError(t, err)
	require.Equal(t, secondFocusID, selection.Account.ID)
	require.Equal(t, openAIAccountScheduleLayerLoadBalance, decision.Layer)
	require.Equal(t, secondFocusID, cache.sessionBindings[sessionKey])
	require.Equal(t, 1, cache.deletedSessions[sessionKey])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestGatewayFocusPoolEscapesBusyStickyAccount(t *testing.T) {
	groupID := int64(47)
	firstFocusID := int64(471)
	secondFocusID := int64(472)
	group := routingTestGroup(groupID)
	group.Platform = PlatformAnthropic
	group.FocusAccountIDs = []int64{firstFocusID, secondFocusID}
	accounts := []Account{
		{ID: firstFocusID, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}},
		{ID: secondFocusID, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100, GroupIDs: []int64{groupID}},
	}
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"focus-sticky-busy": firstFocusID}}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = true
	svc := &GatewayService{
		accountRepo: routingTestGatewayAccountRepo{schedulerTestOpenAIAccountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts}},
		cache:       cache,
		cfg:         cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
			acquireResults: map[int64]bool{firstFocusID: false, secondFocusID: true},
		}),
	}
	ctx := context.WithValue(context.Background(), ctxkey.Group, group)

	selection, err := svc.SelectAccountWithLoadAwareness(
		ctx, &groupID, "focus-sticky-busy", "", nil, "", 0,
	)
	require.NoError(t, err)
	require.Equal(t, secondFocusID, selection.Account.ID)
	require.Equal(t, secondFocusID, cache.sessionBindings["focus-sticky-busy"])
	require.Equal(t, 1, cache.deletedSessions["focus-sticky-busy"])
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestFocusAccountMustBelongToGroup(t *testing.T) {
	account := &Account{
		ID:       5,
		GroupIDs: []int64{2},
		AccountGroups: []AccountGroup{
			{GroupID: 3},
		},
	}
	require.True(t, accountAssignedToGroup(account, 2))
	require.True(t, accountAssignedToGroup(account, 3))
	require.False(t, accountAssignedToGroup(account, 4))
}

func routingTestGroup(id int64) *Group {
	return &Group{ID: id, Platform: PlatformOpenAI, Status: StatusActive, Hydrated: true}
}

func routingTestAccount(id, groupID int64, accountType string, priority int) Account {
	return Account{
		ID: id, Platform: PlatformOpenAI, Type: accountType,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: priority,
		GroupIDs: []int64{groupID},
	}
}

func routingTestOpenAIService(accounts []Account, cache *schedulerTestGatewayCache, concurrency schedulerTestConcurrencyCache) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.LBTopK = 1
	cfg.Gateway.OpenAIWS.SchedulerScoreWeights.Priority = 1
	return &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              cache,
		cfg:                cfg,
		rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService("true"),
		concurrencyService: NewConcurrencyService(concurrency),
	}
}

type routingTestGatewayAccountRepo struct {
	schedulerTestOpenAIAccountRepo
}

func (r routingTestGatewayAccountRepo) ListSchedulableByGroupIDAndPlatforms(_ context.Context, groupID int64, platforms []string) ([]Account, error) {
	allowed := make(map[string]struct{}, len(platforms))
	for _, platform := range platforms {
		allowed[platform] = struct{}{}
	}
	result := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if _, ok := allowed[account.Platform]; ok && accountAssignedToGroup(&account, groupID) {
			result = append(result, account)
		}
	}
	return result, nil
}

type routingTestGroupRepo struct {
	GroupRepository
	group *Group
}

func (r routingTestGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	if r.group != nil && r.group.ID == id {
		return r.group, nil
	}
	return nil, ErrGroupNotFound
}
