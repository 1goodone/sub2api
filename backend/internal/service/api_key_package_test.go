package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPackageAPIKeyBindingConversion(t *testing.T) {
	now := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		cycle  string
		days   int
		want5h float64
		want7d float64
	}{
		{APIKeyPackageCycleWeek, 7, 2.98, 100},
		{APIKeyPackageCycleMonth, 30, 0.69, 23.33},
	}
	for _, tt := range tests {
		t.Run(tt.cycle, func(t *testing.T) {
			pkg := &APIKeyPackage{ID: 9, UserID: 3, Cycle: tt.cycle, AmountUSD: 100, Limit5hEnabled: true, Limit7dEnabled: true}
			binding := packageAPIKeyBinding(pkg, 17, now)
			require.Equal(t, int64(3), binding.UserID)
			require.Equal(t, int64(9), binding.PackageID)
			require.Equal(t, int64(17), binding.KeyID)
			require.Equal(t, 100.0, binding.Quota)
			require.Equal(t, now.AddDate(0, 0, tt.days), binding.ExpiresAt)
			require.Equal(t, tt.want5h, binding.RateLimit5h)
			require.Equal(t, tt.want7d, binding.RateLimit7d)
		})
	}
}

func TestPackageAPIKeyBindingLimitSwitches(t *testing.T) {
	for mask := 0; mask < 4; mask++ {
		pkg := &APIKeyPackage{
			ID:             1,
			Cycle:          APIKeyPackageCycleWeek,
			AmountUSD:      100,
			Limit5hEnabled: mask&1 != 0,
			Limit7dEnabled: mask&2 != 0,
		}
		binding := packageAPIKeyBinding(pkg, 2, time.Now())
		require.Equal(t, pkg.Limit5hEnabled, binding.RateLimit5h > 0, "mask %d 5h", mask)
		require.Equal(t, pkg.Limit7dEnabled, binding.RateLimit7d > 0, "mask %d 7d", mask)
	}
}

func TestAPIKeyPackagePlanCatalog(t *testing.T) {
	plans := ListAPIKeyPackagePlans()
	require.Equal(t, []string{
		APIKeyPackagePlanGo,
		APIKeyPackagePlanPlus,
		APIKeyPackagePlanPro5x,
		APIKeyPackagePlanPro20x,
		APIKeyPackagePlanPro30x,
	}, []string{plans[0].ID, plans[1].ID, plans[2].ID, plans[3].ID, plans[4].ID})
	require.Equal(t, []float64{70, 140, 700, 1400, 1800}, []float64{
		plans[0].AmountUSD, plans[1].AmountUSD, plans[2].AmountUSD, plans[3].AmountUSD, plans[4].AmountUSD,
	})
	for _, plan := range plans {
		require.Equal(t, APIKeyPackageCycleMonth, plan.Cycle)
		require.True(t, plan.Limit5hEnabled)
		require.True(t, plan.Limit7dEnabled)
	}
}

func TestNewAPIKeyPackageUsesServerCatalog(t *testing.T) {
	var req SaveAPIKeyPackageRequest
	require.NoError(t, json.Unmarshal([]byte(`{"plan_id":"plus","amount_usd":0.01,"cycle":"week","limit_5h_enabled":false}`), &req))
	pkg, err := newAPIKeyPackage(3, req)
	require.NoError(t, err)
	require.Equal(t, APIKeyPackagePlanPlus, pkg.PlanID)
	require.Equal(t, "Plus", pkg.Name)
	require.Equal(t, APIKeyPackageCycleMonth, pkg.Cycle)
	require.Equal(t, 140.0, pkg.AmountUSD)
	require.True(t, pkg.Limit5hEnabled)
	require.True(t, pkg.Limit7dEnabled)

	for _, req := range []SaveAPIKeyPackageRequest{{}, {PlanID: "enterprise"}} {
		_, err := newAPIKeyPackage(3, req)
		require.ErrorIs(t, err, ErrAPIKeyPackageInvalid)
	}
}

type apiKeyPackageRepoStub struct {
	pkg       *APIKeyPackage
	packages  []APIKeyPackage
	created   int
	keyCount  int
	deleted   bool
	bound     *APIKeyPackageKeyBinding
	upgraded  *APIKeyPackageKeyUpgrade
	updated   *APIKeyPackage
	updateKey APIKey
	unboundID int64
	key       *APIKey
}

func (s *apiKeyPackageRepoStub) Create(_ context.Context, pkg *APIKeyPackage) error {
	s.created++
	if pkg.ID == 0 {
		pkg.ID = 99
	}
	return nil
}
func (s *apiKeyPackageRepoStub) ListByUserID(context.Context, int64) ([]APIKeyPackage, error) {
	return s.packages, nil
}

func TestAPIKeyPackageCreateReusesExistingFixedPlan(t *testing.T) {
	existing := APIKeyPackage{
		ID: 4, UserID: 7, Name: "Plus", Cycle: APIKeyPackageCycleMonth, AmountUSD: 140,
		Limit5hEnabled: true, Limit7dEnabled: true,
	}
	repo := &apiKeyPackageRepoStub{packages: []APIKeyPackage{existing}}
	svc := NewAPIKeyPackageService(repo, nil)

	pkg, err := svc.Create(t.Context(), 7, SaveAPIKeyPackageRequest{PlanID: APIKeyPackagePlanPlus})
	require.NoError(t, err)
	require.Equal(t, int64(4), pkg.ID)
	require.Equal(t, APIKeyPackagePlanPlus, pkg.PlanID)
	require.Zero(t, repo.created)

	created, err := svc.Create(t.Context(), 7, SaveAPIKeyPackageRequest{PlanID: APIKeyPackagePlanGo})
	require.NoError(t, err)
	require.Equal(t, int64(99), created.ID)
	require.Equal(t, 1, repo.created)
}
func (s *apiKeyPackageRepoStub) GetByID(_ context.Context, userID, id int64) (*APIKeyPackage, error) {
	if s.pkg == nil || s.pkg.ID != id || s.pkg.UserID != userID {
		return nil, ErrAPIKeyPackageNotFound
	}
	clone := *s.pkg
	return &clone, nil
}
func (s *apiKeyPackageRepoStub) Update(_ context.Context, pkg *APIKeyPackage, _ APIKeyPackageKeyBinding) ([]APIKey, error) {
	clone := *pkg
	s.updated = &clone
	if s.updateKey.ID == 0 {
		return nil, nil
	}
	return []APIKey{s.updateKey}, nil
}
func (s *apiKeyPackageRepoStub) Delete(_ context.Context, userID, id int64) error {
	if s.pkg == nil || s.pkg.ID != id || s.pkg.UserID != userID {
		return ErrAPIKeyPackageNotFound
	}
	s.deleted = true
	return nil
}
func (s *apiKeyPackageRepoStub) CountKeys(context.Context, int64) (int, error) {
	return s.keyCount, nil
}
func (s *apiKeyPackageRepoStub) BindKey(_ context.Context, binding APIKeyPackageKeyBinding) (*APIKey, error) {
	s.bound = &binding
	return s.key, nil
}
func (s *apiKeyPackageRepoStub) UpgradeKey(_ context.Context, upgrade APIKeyPackageKeyUpgrade) (*APIKey, error) {
	s.upgraded = &upgrade
	return s.key, nil
}
func (s *apiKeyPackageRepoStub) UnbindKey(_ context.Context, _, _, keyID int64) (*APIKey, error) {
	s.unboundID = keyID
	return s.key, nil
}

func TestAPIKeyPackageDeleteOwnershipAndNonEmptyProtection(t *testing.T) {
	repo := &apiKeyPackageRepoStub{pkg: &APIKeyPackage{ID: 2, UserID: 7}, keyCount: 1}
	svc := NewAPIKeyPackageService(repo, nil)

	require.ErrorIs(t, svc.Delete(t.Context(), 8, 2), ErrAPIKeyPackageNotFound)
	require.ErrorIs(t, svc.Delete(t.Context(), 7, 2), ErrAPIKeyPackageNotEmpty)
	require.False(t, repo.deleted)

	repo.keyCount = 0
	require.NoError(t, svc.Delete(t.Context(), 7, 2))
	require.True(t, repo.deleted)
}

func TestAPIKeyPackageBindsAndUnbindsExistingKey(t *testing.T) {
	pkg := APIKeyPackage{ID: 2, UserID: 7, Name: "Plus", Cycle: APIKeyPackageCycleMonth, AmountUSD: 140, Limit5hEnabled: true, Limit7dEnabled: true}
	repo := &apiKeyPackageRepoStub{
		pkg:      &pkg,
		packages: []APIKeyPackage{pkg},
		key:      &APIKey{ID: 11, UserID: 7, Key: "sk-existing-key"},
	}
	svc := NewAPIKeyPackageService(repo, nil)

	bound, err := svc.BindKey(t.Context(), 7, 2, BindPackageAPIKeyRequest{KeyID: 11})
	require.NoError(t, err)
	require.Equal(t, int64(11), bound.ID)
	require.NotNil(t, repo.bound)
	require.Equal(t, 0.97, repo.bound.RateLimit5h)
	require.Equal(t, 32.67, repo.bound.RateLimit7d)
	require.WithinDuration(t, time.Now().AddDate(0, 0, 30), repo.bound.ExpiresAt, time.Second)

	unbound, err := svc.UnbindKey(t.Context(), 7, 2, 11)
	require.NoError(t, err)
	require.Equal(t, int64(11), unbound.ID)
	require.Equal(t, int64(11), repo.unboundID)
}

func TestAPIKeyPackageRejectsSecondActiveFixedPlan(t *testing.T) {
	target := APIKeyPackage{ID: 2, UserID: 7, Name: "Pro 5x", Cycle: APIKeyPackageCycleMonth, AmountUSD: 700, Limit5hEnabled: true, Limit7dEnabled: true}
	repo := &apiKeyPackageRepoStub{
		pkg: &target,
		packages: []APIKeyPackage{
			{ID: 1, UserID: 7, Name: "Plus", Cycle: APIKeyPackageCycleMonth, AmountUSD: 140, Limit5hEnabled: true, Limit7dEnabled: true, KeyCount: 1},
			target,
		},
	}
	svc := NewAPIKeyPackageService(repo, nil)

	_, err := svc.BindKey(t.Context(), 7, 2, BindPackageAPIKeyRequest{KeyID: 11})
	require.ErrorIs(t, err, ErrAPIKeyPackageAlreadyActive)
	require.Nil(t, repo.bound)
}

func TestAPIKeyPackageIgnoresActiveLegacyPlanWhenBinding(t *testing.T) {
	target := APIKeyPackage{ID: 2, UserID: 7, Name: "Go", Cycle: APIKeyPackageCycleMonth, AmountUSD: 70, Limit5hEnabled: true, Limit7dEnabled: true}
	repo := &apiKeyPackageRepoStub{
		pkg: &target,
		packages: []APIKeyPackage{
			{ID: 1, UserID: 7, Name: "Legacy", Cycle: APIKeyPackageCycleMonth, AmountUSD: 99, Limit5hEnabled: true, Limit7dEnabled: true, KeyCount: 1},
			target,
		},
		key: &APIKey{ID: 11, UserID: 7, Key: "sk-existing-key"},
	}
	svc := NewAPIKeyPackageService(repo, nil)

	_, err := svc.BindKey(t.Context(), 7, 2, BindPackageAPIKeyRequest{KeyID: 11})
	require.NoError(t, err)
	require.NotNil(t, repo.bound)
}

func TestAPIKeyPackageRejectsBindingLegacyCustomPlan(t *testing.T) {
	repo := &apiKeyPackageRepoStub{
		pkg: &APIKeyPackage{ID: 2, UserID: 7, Name: "Custom", Cycle: APIKeyPackageCycleMonth, AmountUSD: 1},
	}
	svc := NewAPIKeyPackageService(repo, nil)
	_, err := svc.BindKey(t.Context(), 7, 2, BindPackageAPIKeyRequest{KeyID: 11})
	require.ErrorIs(t, err, ErrAPIKeyPackageInvalid)
}

func TestAPIKeyPackageRejectsInvalidBindingIDs(t *testing.T) {
	svc := NewAPIKeyPackageService(&apiKeyPackageRepoStub{}, nil)
	_, err := svc.BindKey(t.Context(), 7, 2, BindPackageAPIKeyRequest{})
	require.ErrorIs(t, err, ErrAPIKeyPackageInvalid)
	_, err = svc.UnbindKey(t.Context(), 7, 2, 0)
	require.ErrorIs(t, err, ErrAPIKeyPackageInvalid)
}

func TestAPIKeyPackageUpgradeOnlyAllowsHigherFixedPlan(t *testing.T) {
	current := APIKeyPackage{
		ID: 2, UserID: 7, Name: "Plus", Cycle: APIKeyPackageCycleMonth, AmountUSD: 140,
		Limit5hEnabled: true, Limit7dEnabled: true, KeyCount: 1,
	}
	target := APIKeyPackage{
		ID: 8, UserID: 7, Name: "Pro 5x", Cycle: APIKeyPackageCycleMonth, AmountUSD: 700,
		Limit5hEnabled: true, Limit7dEnabled: true,
	}
	repo := &apiKeyPackageRepoStub{
		pkg:      &current,
		packages: []APIKeyPackage{current, target},
		key:      &APIKey{ID: 11, UserID: 7, Key: "sk-upgraded"},
	}
	authCache := &packageAuthCacheStub{}
	rateLimitCache := &packageRateLimitCacheStub{}
	svc := NewAPIKeyPackageService(repo, &APIKeyService{cache: authCache, rateLimitCacheInvalid: rateLimitCache})

	key, err := svc.UpgradeKey(t.Context(), 7, 2, 11, UpgradePackageAPIKeyRequest{PlanID: APIKeyPackagePlanPro5x})
	require.NoError(t, err)
	require.Equal(t, int64(11), key.ID)
	require.NotNil(t, repo.upgraded)
	require.Equal(t, int64(2), repo.upgraded.SourcePackageID)
	require.Equal(t, int64(8), repo.upgraded.TargetPackageID)
	require.Equal(t, 700.0, repo.upgraded.Quota)
	require.Equal(t, 4.86, repo.upgraded.RateLimit5h)
	require.Equal(t, 163.33, repo.upgraded.RateLimit7d)
	require.Equal(t, []int64{11}, rateLimitCache.ids)
	require.Len(t, authCache.deleted, 1)
	require.Equal(t, authCache.deleted, authCache.published)

	for _, planID := range []string{APIKeyPackagePlanGo, APIKeyPackagePlanPlus} {
		_, err = svc.UpgradeKey(t.Context(), 7, 2, 11, UpgradePackageAPIKeyRequest{PlanID: planID})
		require.ErrorIs(t, err, ErrAPIKeyPackageUpgradeOnly)
	}
	_, err = svc.UpgradeKey(t.Context(), 7, 2, 11, UpgradePackageAPIKeyRequest{PlanID: "enterprise"})
	require.ErrorIs(t, err, ErrAPIKeyPackageInvalid)

	repo.pkg = &APIKeyPackage{ID: 2, UserID: 7, Name: "Legacy", Cycle: APIKeyPackageCycleMonth, AmountUSD: 100, KeyCount: 1}
	_, err = svc.UpgradeKey(t.Context(), 7, 2, 11, UpgradePackageAPIKeyRequest{PlanID: APIKeyPackagePlanPro5x})
	require.ErrorIs(t, err, ErrAPIKeyPackageUpgradeOnly)
}

func TestAPIKeyPackageUpdateRejectsPackagesWithKeys(t *testing.T) {
	repo := &apiKeyPackageRepoStub{pkg: &APIKeyPackage{ID: 2, UserID: 7, KeyCount: 1}}
	svc := NewAPIKeyPackageService(repo, nil)

	_, err := svc.Update(t.Context(), 7, 2, SaveAPIKeyPackageRequest{PlanID: APIKeyPackagePlanPro5x})
	require.ErrorIs(t, err, ErrAPIKeyPackageUpgradeOnly)
	require.Nil(t, repo.updated)
}

func TestPackageAPIKeyBindingClampsEnabledLimitsToOneCent(t *testing.T) {
	pkg := &APIKeyPackage{
		Cycle:          APIKeyPackageCycleMonth,
		AmountUSD:      0.01,
		Limit5hEnabled: true,
		Limit7dEnabled: true,
	}
	binding := packageAPIKeyBinding(pkg, 1, time.Now())
	require.Equal(t, 0.01, binding.RateLimit5h)
	require.Equal(t, 0.01, binding.RateLimit7d)
}

type packageAuthCacheStub struct {
	deleted   []string
	published []string
}

func (*packageAuthCacheStub) GetCreateAttemptCount(context.Context, int64) (int, error) {
	return 0, nil
}
func (*packageAuthCacheStub) IncrementCreateAttemptCount(context.Context, int64) error { return nil }
func (*packageAuthCacheStub) DeleteCreateAttemptCount(context.Context, int64) error    { return nil }
func (*packageAuthCacheStub) IncrementDailyUsage(context.Context, string) error        { return nil }
func (*packageAuthCacheStub) SetDailyUsageExpiry(context.Context, string, time.Duration) error {
	return nil
}
func (*packageAuthCacheStub) GetAuthCache(context.Context, string) (*APIKeyAuthCacheEntry, error) {
	return nil, nil
}
func (*packageAuthCacheStub) SetAuthCache(context.Context, string, *APIKeyAuthCacheEntry, time.Duration) error {
	return nil
}
func (s *packageAuthCacheStub) DeleteAuthCache(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}
func (s *packageAuthCacheStub) PublishAuthCacheInvalidation(_ context.Context, key string) error {
	s.published = append(s.published, key)
	return nil
}
func (*packageAuthCacheStub) SubscribeAuthCacheInvalidation(context.Context, func(string)) error {
	return nil
}

type packageRateLimitCacheStub struct{ ids []int64 }

func (s *packageRateLimitCacheStub) InvalidateAPIKeyRateLimit(_ context.Context, id int64) error {
	s.ids = append(s.ids, id)
	return nil
}

func TestAPIKeyPackageUpdateInvalidatesBoundKeyCaches(t *testing.T) {
	repo := &apiKeyPackageRepoStub{
		pkg:       &APIKeyPackage{ID: 2, UserID: 7, Cycle: APIKeyPackageCycleWeek, AmountUSD: 50},
		updateKey: APIKey{ID: 11, UserID: 7, Key: "sk-updated"},
	}
	authCache := &packageAuthCacheStub{}
	rateLimitCache := &packageRateLimitCacheStub{}
	apiKeySvc := &APIKeyService{cache: authCache, rateLimitCacheInvalid: rateLimitCache}
	svc := NewAPIKeyPackageService(repo, apiKeySvc)

	updated, err := svc.Update(t.Context(), 7, 2, SaveAPIKeyPackageRequest{
		PlanID: APIKeyPackagePlanPlus,
	})
	require.NoError(t, err)
	require.Equal(t, "Plus", updated.Name)
	require.NotNil(t, repo.updated)
	require.Equal(t, []int64{11}, rateLimitCache.ids)
	require.Len(t, authCache.deleted, 1)
	require.Equal(t, authCache.deleted, authCache.published)
}
