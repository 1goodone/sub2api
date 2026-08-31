package repository

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyPackageRepositoryBindEnforcesOwnershipAndSinglePackage(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	repo := &apiKeyPackageRepository{client: client}

	owner, err := client.User.Create().SetEmail("owner@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	other, err := client.User.Create().SetEmail("other@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	first, err := client.APIKeyPackage.Create().SetUserID(owner.ID).SetName("Go").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(70).SetLimit5hEnabled(true).SetLimit7dEnabled(true).Save(ctx)
	require.NoError(t, err)
	second, err := client.APIKeyPackage.Create().SetUserID(owner.ID).SetName("Plus").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(140).SetLimit5hEnabled(true).SetLimit7dEnabled(true).Save(ctx)
	require.NoError(t, err)
	ownedKey, err := client.APIKey.Create().SetUserID(owner.ID).SetKey("sk-owned").SetName("Owned").Save(ctx)
	require.NoError(t, err)
	secondOwnedKey, err := client.APIKey.Create().SetUserID(owner.ID).SetKey("sk-owned-second").SetName("Owned second").Save(ctx)
	require.NoError(t, err)
	otherKey, err := client.APIKey.Create().SetUserID(other.ID).SetKey("sk-other").SetName("Other").Save(ctx)
	require.NoError(t, err)

	binding := service.APIKeyPackageKeyBinding{
		UserID: owner.ID, PackageID: first.ID, KeyID: ownedKey.ID,
		Quota: 100, ExpiresAt: time.Now().Add(7 * 24 * time.Hour), RateLimit5h: 2.98, RateLimit7d: 100,
	}
	binding.KeyID = otherKey.ID
	_, err = repo.BindKey(ctx, binding)
	require.ErrorIs(t, err, service.ErrAPIKeyPackageKeyUnavailable)

	binding.KeyID = ownedKey.ID
	bound, err := repo.BindKey(ctx, binding)
	require.NoError(t, err)
	require.Equal(t, &first.ID, bound.PackageID)

	binding.PackageID = second.ID
	binding.KeyID = secondOwnedKey.ID
	_, err = repo.BindKey(ctx, binding)
	require.ErrorIs(t, err, service.ErrAPIKeyPackageAlreadyActive)

	stored, err := client.APIKey.Get(ctx, ownedKey.ID)
	require.NoError(t, err)
	require.Equal(t, &first.ID, stored.PackageID)
	require.Nil(t, otherKey.PackageID)
}

func TestAPIKeyPackageRepositoryBindIgnoresLegacyPackage(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	repo := &apiKeyPackageRepository{client: client}

	owner, err := client.User.Create().SetEmail("legacy-owner@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	legacy, err := client.APIKeyPackage.Create().SetUserID(owner.ID).SetName("Legacy").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(99).Save(ctx)
	require.NoError(t, err)
	fixed, err := client.APIKeyPackage.Create().SetUserID(owner.ID).SetName("Go").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(70).SetLimit5hEnabled(true).SetLimit7dEnabled(true).Save(ctx)
	require.NoError(t, err)
	_, err = client.APIKey.Create().SetUserID(owner.ID).SetPackageID(legacy.ID).SetKey("sk-legacy-bound").SetName("Legacy bound").Save(ctx)
	require.NoError(t, err)
	available, err := client.APIKey.Create().SetUserID(owner.ID).SetKey("sk-fixed-available").SetName("Fixed available").Save(ctx)
	require.NoError(t, err)

	bound, err := repo.BindKey(ctx, service.APIKeyPackageKeyBinding{
		UserID: owner.ID, PackageID: fixed.ID, KeyID: available.ID,
		Quota: 70, ExpiresAt: time.Now().Add(30 * 24 * time.Hour), RateLimit5h: 0.49, RateLimit7d: 16.33,
	})
	require.NoError(t, err)
	require.Equal(t, &fixed.ID, bound.PackageID)
}

func TestAPIKeyPackageRepositoryUnbindKeepsKeyAndClearsOnlyPackageFields(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	repo := &apiKeyPackageRepository{client: client}

	user, err := client.User.Create().SetEmail("unbind@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	pkg, err := client.APIKeyPackage.Create().SetUserID(user.ID).SetName("Weekly").SetCycle(service.APIKeyPackageCycleWeek).SetAmountUsd(50).Save(ctx)
	require.NoError(t, err)
	window1d := time.Now().Add(-time.Hour)
	key, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetKey("sk-keep-me").
		SetName("Keep me").
		SetRateLimit1d(9).
		SetUsage1d(3).
		SetWindow1dStart(window1d).
		Save(ctx)
	require.NoError(t, err)

	_, err = repo.BindKey(ctx, service.APIKeyPackageKeyBinding{
		UserID: user.ID, PackageID: pkg.ID, KeyID: key.ID,
		Quota: 50, ExpiresAt: time.Now().Add(7 * 24 * time.Hour), RateLimit5h: 1.49, RateLimit7d: 50,
	})
	require.NoError(t, err)

	unbound, err := repo.UnbindKey(ctx, user.ID, pkg.ID, key.ID)
	require.NoError(t, err)
	require.Nil(t, unbound.PackageID)
	require.Equal(t, "sk-keep-me", unbound.Key)

	stored, err := client.APIKey.Query().Where(apikey.IDEQ(key.ID)).Only(ctx)
	require.NoError(t, err)
	require.Nil(t, stored.PackageID)
	require.Nil(t, stored.DeletedAt)
	require.Zero(t, stored.Quota)
	require.Zero(t, stored.QuotaUsed)
	require.Nil(t, stored.ExpiresAt)
	require.Zero(t, stored.RateLimit5h)
	require.Zero(t, stored.RateLimit7d)
	require.Zero(t, stored.Usage5h)
	require.Zero(t, stored.Usage7d)
	require.Nil(t, stored.Window5hStart)
	require.Nil(t, stored.Window7dStart)
	require.Equal(t, 9.0, stored.RateLimit1d)
	require.Equal(t, 3.0, stored.Usage1d)
	require.WithinDuration(t, window1d, *stored.Window1dStart, time.Second)

	count, err := repo.CountKeys(ctx, pkg.ID)
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestAPIKeyPackageRepositoryUpgradePreservesPeriodAndUsage(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	repo := &apiKeyPackageRepository{client: client}

	user, err := client.User.Create().SetEmail("upgrade-package@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	source, err := client.APIKeyPackage.Create().SetUserID(user.ID).SetName("Plus").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(140).Save(ctx)
	require.NoError(t, err)
	target, err := client.APIKeyPackage.Create().SetUserID(user.ID).SetName("Pro 5x").SetCycle(service.APIKeyPackageCycleMonth).SetAmountUsd(700).Save(ctx)
	require.NoError(t, err)
	now := time.Now().Truncate(time.Second)
	expiresAt := now.Add(20 * 24 * time.Hour)
	window5h := now.Add(-2 * time.Hour)
	window7d := now.Add(-3 * 24 * time.Hour)
	key, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetPackageID(source.ID).
		SetKey("sk-upgrade-preserve").
		SetName("Upgrade preserve").
		SetStatus(service.StatusAPIKeyQuotaExhausted).
		SetQuota(140).
		SetQuotaUsed(120).
		SetExpiresAt(expiresAt).
		SetRateLimit5h(0.97).
		SetRateLimit1d(9).
		SetRateLimit7d(32.67).
		SetUsage5h(0.75).
		SetUsage1d(3).
		SetUsage7d(20).
		SetWindow5hStart(window5h).
		SetWindow7dStart(window7d).
		Save(ctx)
	require.NoError(t, err)

	upgraded, err := repo.UpgradeKey(ctx, service.APIKeyPackageKeyUpgrade{
		UserID: user.ID, SourcePackageID: source.ID, TargetPackageID: target.ID, KeyID: key.ID,
		Quota: 700, RateLimit5h: 4.86, RateLimit7d: 163.33, At: now,
	})
	require.NoError(t, err)
	require.Equal(t, &target.ID, upgraded.PackageID)

	stored, err := client.APIKey.Get(ctx, key.ID)
	require.NoError(t, err)
	require.Equal(t, &target.ID, stored.PackageID)
	require.Equal(t, 700.0, stored.Quota)
	require.Equal(t, 120.0, stored.QuotaUsed)
	require.WithinDuration(t, expiresAt, *stored.ExpiresAt, time.Second)
	require.Equal(t, 4.86, stored.RateLimit5h)
	require.Equal(t, 9.0, stored.RateLimit1d)
	require.Equal(t, 163.33, stored.RateLimit7d)
	require.Equal(t, 0.75, stored.Usage5h)
	require.Equal(t, 3.0, stored.Usage1d)
	require.Equal(t, 20.0, stored.Usage7d)
	require.WithinDuration(t, window5h, *stored.Window5hStart, time.Second)
	require.WithinDuration(t, window7d, *stored.Window7dStart, time.Second)
	require.Equal(t, service.StatusAPIKeyActive, stored.Status)

	expired, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetPackageID(source.ID).
		SetKey("sk-upgrade-expired").
		SetName("Expired").
		SetQuota(140).
		SetExpiresAt(now.Add(-time.Minute)).
		Save(ctx)
	require.NoError(t, err)
	_, err = repo.UpgradeKey(ctx, service.APIKeyPackageKeyUpgrade{
		UserID: user.ID, SourcePackageID: source.ID, TargetPackageID: target.ID, KeyID: expired.ID,
		Quota: 700, RateLimit5h: 4.86, RateLimit7d: 163.33, At: now,
	})
	require.ErrorIs(t, err, service.ErrAPIKeyPackageKeyExpired)
	storedExpired, err := client.APIKey.Get(ctx, expired.ID)
	require.NoError(t, err)
	require.Equal(t, &source.ID, storedExpired.PackageID)
	require.Equal(t, 140.0, storedExpired.Quota)
}

func TestAPIKeyPackageRepositoryUpdateSynchronizesBoundKeys(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	repo := &apiKeyPackageRepository{client: client}

	user, err := client.User.Create().SetEmail("update-package@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	pkg, err := client.APIKeyPackage.Create().
		SetUserID(user.ID).
		SetName("Weekly").
		SetCycle(service.APIKeyPackageCycleWeek).
		SetAmountUsd(100).
		SetLimit5hEnabled(true).
		SetLimit7dEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	window5h := time.Now().Add(-time.Hour).Truncate(time.Second)
	window7d := time.Now().Add(-24 * time.Hour).Truncate(time.Second)
	exhausted, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetPackageID(pkg.ID).
		SetKey("sk-package-exhausted").
		SetName("Exhausted").
		SetStatus(service.StatusAPIKeyActive).
		SetQuota(100).
		SetQuotaUsed(30).
		SetUsage5h(1.5).
		SetUsage7d(20).
		SetWindow5hStart(window5h).
		SetWindow7dStart(window7d).
		Save(ctx)
	require.NoError(t, err)
	inactive, err := client.APIKey.Create().
		SetUserID(user.ID).
		SetPackageID(pkg.ID).
		SetKey("sk-package-inactive").
		SetName("Inactive").
		SetStatus("inactive").
		SetQuotaUsed(30).
		Save(ctx)
	require.NoError(t, err)

	expiresAt := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	updated := &service.APIKeyPackage{
		ID: pkg.ID, UserID: user.ID, Name: "Monthly", Cycle: service.APIKeyPackageCycleMonth,
		AmountUSD: 20, Limit5hEnabled: true, Limit7dEnabled: true,
	}
	keys, err := repo.Update(ctx, updated, service.APIKeyPackageKeyBinding{
		Quota: 20, ExpiresAt: expiresAt, RateLimit5h: 0.14, RateLimit7d: 4.67,
	})
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, 2, updated.KeyCount)

	stored, err := client.APIKey.Get(ctx, exhausted.ID)
	require.NoError(t, err)
	require.Equal(t, 20.0, stored.Quota)
	require.Equal(t, 30.0, stored.QuotaUsed)
	require.WithinDuration(t, expiresAt, *stored.ExpiresAt, time.Second)
	require.Equal(t, 0.14, stored.RateLimit5h)
	require.Equal(t, 4.67, stored.RateLimit7d)
	require.Equal(t, 1.5, stored.Usage5h)
	require.Equal(t, 20.0, stored.Usage7d)
	require.WithinDuration(t, window5h, *stored.Window5hStart, time.Second)
	require.WithinDuration(t, window7d, *stored.Window7dStart, time.Second)
	require.Equal(t, service.StatusAPIKeyQuotaExhausted, stored.Status)

	storedInactive, err := client.APIKey.Get(ctx, inactive.ID)
	require.NoError(t, err)
	require.Equal(t, "inactive", storedInactive.Status)

	updated.AmountUSD = 100
	updated.Limit7dEnabled = false
	_, err = repo.Update(ctx, updated, service.APIKeyPackageKeyBinding{
		Quota: 100, ExpiresAt: expiresAt.AddDate(0, 0, 1), RateLimit5h: 0.69,
	})
	require.NoError(t, err)
	stored, err = client.APIKey.Get(ctx, exhausted.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusAPIKeyActive, stored.Status)
	require.Equal(t, 30.0, stored.QuotaUsed)
	require.Equal(t, 1.5, stored.Usage5h)
	require.NotNil(t, stored.Window5hStart)
	require.Zero(t, stored.Usage7d)
	require.Nil(t, stored.Window7dStart)

	updated.Limit5hEnabled = false
	_, err = repo.Update(ctx, updated, service.APIKeyPackageKeyBinding{
		Quota: 100, ExpiresAt: expiresAt.AddDate(0, 0, 2),
	})
	require.NoError(t, err)
	updated.Limit5hEnabled = true
	_, err = repo.Update(ctx, updated, service.APIKeyPackageKeyBinding{
		Quota: 100, ExpiresAt: expiresAt.AddDate(0, 0, 3), RateLimit5h: 0.69,
	})
	require.NoError(t, err)
	stored, err = client.APIKey.Get(ctx, exhausted.ID)
	require.NoError(t, err)
	require.Zero(t, stored.Usage5h)
	require.Nil(t, stored.Window5hStart)
}

func TestAPIKeyPackageRepositorySoftDeleteClearsPackageReference(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	packageRepo := &apiKeyPackageRepository{client: client}
	keyRepo := &apiKeyRepository{client: client}

	user, err := client.User.Create().SetEmail("delete-package-key@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	pkg, err := client.APIKeyPackage.Create().SetUserID(user.ID).SetName("Delete me").SetCycle(service.APIKeyPackageCycleWeek).SetAmountUsd(10).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetPackageID(pkg.ID).SetKey("sk-delete-package-key").SetName("Delete key").Save(ctx)
	require.NoError(t, err)

	require.NoError(t, keyRepo.deleteWithTombstone(ctx, client, key.ID, "__deleted__package_test"))
	stored, err := client.APIKey.Get(mixins.SkipSoftDelete(ctx), key.ID)
	require.NoError(t, err)
	require.Nil(t, stored.PackageID)
	require.NotNil(t, stored.DeletedAt)
	require.NoError(t, packageRepo.Delete(ctx, user.ID, pkg.ID))
}

func TestAPIKeyPackageRepositoryDeleteClearsPackageReference(t *testing.T) {
	ctx := t.Context()
	client := newSecuritySecretTestClient(t)
	packageRepo := &apiKeyPackageRepository{client: client}
	keyRepo := &apiKeyRepository{client: client}

	user, err := client.User.Create().SetEmail("delete-direct-package-key@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	pkg, err := client.APIKeyPackage.Create().SetUserID(user.ID).SetName("Delete direct").SetCycle(service.APIKeyPackageCycleWeek).SetAmountUsd(10).Save(ctx)
	require.NoError(t, err)
	key, err := client.APIKey.Create().SetUserID(user.ID).SetPackageID(pkg.ID).SetKey("sk-delete-direct-package-key").SetName("Delete direct key").Save(ctx)
	require.NoError(t, err)

	require.NoError(t, keyRepo.Delete(ctx, key.ID))
	stored, err := client.APIKey.Get(mixins.SkipSoftDelete(ctx), key.ID)
	require.NoError(t, err)
	require.Nil(t, stored.PackageID)
	require.NotNil(t, stored.DeletedAt)
	require.NoError(t, packageRepo.Delete(ctx, user.ID, pkg.ID))
}
