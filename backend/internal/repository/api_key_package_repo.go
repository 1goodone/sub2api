package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/apikey"
	"github.com/Wei-Shaw/sub2api/ent/apikeypackage"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type apiKeyPackageRepository struct {
	client *dbent.Client
}

func NewAPIKeyPackageRepository(client *dbent.Client) service.APIKeyPackageRepository {
	return &apiKeyPackageRepository{client: client}
}

func (r *apiKeyPackageRepository) Create(ctx context.Context, pkg *service.APIKeyPackage) error {
	created, err := r.client.APIKeyPackage.Create().
		SetUserID(pkg.UserID).
		SetName(pkg.Name).
		SetCycle(pkg.Cycle).
		SetAmountUsd(pkg.AmountUSD).
		SetLimit5hEnabled(pkg.Limit5hEnabled).
		SetLimit7dEnabled(pkg.Limit7dEnabled).
		Save(ctx)
	if err != nil {
		return err
	}
	fillAPIKeyPackage(pkg, created)
	return nil
}

func (r *apiKeyPackageRepository) ListByUserID(ctx context.Context, userID int64) ([]service.APIKeyPackage, error) {
	rows, err := r.client.APIKeyPackage.Query().
		Where(apikeypackage.UserIDEQ(userID)).
		Order(dbent.Desc(apikeypackage.FieldCreatedAt), dbent.Desc(apikeypackage.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]service.APIKeyPackage, 0, len(rows))
	for _, row := range rows {
		pkg := apiKeyPackageEntityToService(row)
		pkg.KeyCount, err = r.CountKeys(ctx, pkg.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, *pkg)
	}
	return result, nil
}

func (r *apiKeyPackageRepository) GetByID(ctx context.Context, userID, id int64) (*service.APIKeyPackage, error) {
	row, err := r.client.APIKeyPackage.Query().
		Where(apikeypackage.IDEQ(id), apikeypackage.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyPackageNotFound
		}
		return nil, err
	}
	pkg := apiKeyPackageEntityToService(row)
	pkg.KeyCount, err = r.CountKeys(ctx, id)
	if err != nil {
		return nil, err
	}
	return pkg, nil
}

func (r *apiKeyPackageRepository) Update(ctx context.Context, pkg *service.APIKeyPackage, binding service.APIKeyPackageKeyBinding) ([]service.APIKey, error) {
	client := r.client
	tx := dbent.TxFromContext(ctx)
	ownsTx := false
	if tx != nil {
		client = tx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if errors.Is(err, dbent.ErrTxStarted) {
			tx = nil
		} else if err != nil {
			return nil, err
		} else {
			ownsTx = true
			client = tx.Client()
			defer func() { _ = tx.Rollback() }()
		}
	}

	current, err := client.APIKeyPackage.Query().
		Where(apikeypackage.IDEQ(pkg.ID), apikeypackage.UserIDEQ(pkg.UserID)).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyPackageNotFound
		}
		return nil, err
	}
	keys, err := client.APIKey.Query().
		Where(apikey.PackageIDEQ(pkg.ID), apikey.UserIDEQ(pkg.UserID), apikey.DeletedAtIsNil()).
		All(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := client.APIKeyPackage.UpdateOne(current).
		SetName(pkg.Name).
		SetCycle(pkg.Cycle).
		SetAmountUsd(pkg.AmountUSD).
		SetLimit5hEnabled(pkg.Limit5hEnabled).
		SetLimit7dEnabled(pkg.Limit7dEnabled).
		Save(ctx)
	if err != nil {
		return nil, err
	}

	keyUpdate := client.APIKey.Update().
		Where(apikey.PackageIDEQ(pkg.ID), apikey.UserIDEQ(pkg.UserID), apikey.DeletedAtIsNil()).
		SetQuota(binding.Quota).
		SetExpiresAt(binding.ExpiresAt).
		SetRateLimit5h(binding.RateLimit5h).
		SetRateLimit7d(binding.RateLimit7d).
		SetUpdatedAt(time.Now())
	if !pkg.Limit5hEnabled || !current.Limit5hEnabled {
		keyUpdate.SetUsage5h(0).ClearWindow5hStart()
	}
	if !pkg.Limit7dEnabled || !current.Limit7dEnabled {
		keyUpdate.SetUsage7d(0).ClearWindow7dStart()
	}
	affected, err := keyUpdate.Save(ctx)
	if err != nil {
		return nil, err
	}
	if affected != len(keys) {
		return nil, fmt.Errorf("update package keys: expected %d rows, updated %d", len(keys), affected)
	}

	derivedStatuses := []string{service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted, service.StatusAPIKeyExpired}
	if _, err := client.APIKey.Update().
		Where(
			apikey.PackageIDEQ(pkg.ID),
			apikey.UserIDEQ(pkg.UserID),
			apikey.DeletedAtIsNil(),
			apikey.StatusIn(derivedStatuses...),
			apikey.QuotaUsedGTE(binding.Quota),
		).
		SetStatus(service.StatusAPIKeyQuotaExhausted).
		Save(ctx); err != nil {
		return nil, err
	}
	if _, err := client.APIKey.Update().
		Where(
			apikey.PackageIDEQ(pkg.ID),
			apikey.UserIDEQ(pkg.UserID),
			apikey.DeletedAtIsNil(),
			apikey.StatusIn(derivedStatuses...),
			apikey.QuotaUsedLT(binding.Quota),
		).
		SetStatus(service.StatusAPIKeyActive).
		Save(ctx); err != nil {
		return nil, err
	}

	if ownsTx {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	fillAPIKeyPackage(pkg, updated)
	pkg.KeyCount = len(keys)
	affectedKeys := make([]service.APIKey, len(keys))
	for i, key := range keys {
		affectedKeys[i] = service.APIKey{ID: key.ID, UserID: key.UserID, Key: key.Key}
	}
	return affectedKeys, nil
}

func (r *apiKeyPackageRepository) Delete(ctx context.Context, userID, id int64) error {
	deleted, err := r.client.APIKeyPackage.Delete().
		Where(apikeypackage.IDEQ(id), apikeypackage.UserIDEQ(userID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return service.ErrAPIKeyPackageNotFound
	}
	return nil
}

func (r *apiKeyPackageRepository) CountKeys(ctx context.Context, packageID int64) (int, error) {
	return r.client.APIKey.Query().
		Where(apikey.PackageIDEQ(packageID), apikey.DeletedAtIsNil()).
		Count(ctx)
}

func (r *apiKeyPackageRepository) BindKey(ctx context.Context, binding service.APIKeyPackageKeyBinding) (*service.APIKey, error) {
	client := r.client
	tx := dbent.TxFromContext(ctx)
	ownsTx := false
	if tx != nil {
		client = tx.Client()
	} else {
		var err error
		tx, err = r.client.Tx(ctx)
		if err != nil {
			return nil, err
		}
		ownsTx = true
		client = tx.Client()
		defer func() { _ = tx.Rollback() }()
	}

	now := time.Now()
	if _, err := client.User.UpdateOneID(binding.UserID).SetUpdatedAt(now).Save(ctx); err != nil {
		return nil, err
	}
	packageRows, err := client.APIKeyPackage.Query().
		Where(apikeypackage.UserIDEQ(binding.UserID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	fixedPackageIDs := make([]int64, 0, len(packageRows))
	for _, packageRow := range packageRows {
		if service.IsFixedAPIKeyPackage(apiKeyPackageEntityToService(packageRow)) {
			fixedPackageIDs = append(fixedPackageIDs, packageRow.ID)
		}
	}
	if len(fixedPackageIDs) > 0 {
		active, err := client.APIKey.Query().
			Where(
				apikey.UserIDEQ(binding.UserID),
				apikey.PackageIDIn(fixedPackageIDs...),
				apikey.DeletedAtIsNil(),
			).
			Exist(ctx)
		if err != nil {
			return nil, err
		}
		if active {
			return nil, service.ErrAPIKeyPackageAlreadyActive
		}
	}

	row, err := client.APIKey.Query().
		Where(
			apikey.IDEQ(binding.KeyID),
			apikey.UserIDEQ(binding.UserID),
			apikey.PackageIDIsNil(),
			apikey.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyPackageKeyUnavailable
		}
		return nil, err
	}

	update := client.APIKey.Update().
		Where(
			apikey.IDEQ(binding.KeyID),
			apikey.UserIDEQ(binding.UserID),
			apikey.PackageIDIsNil(),
			apikey.DeletedAtIsNil(),
		).
		SetPackageID(binding.PackageID).
		SetQuota(binding.Quota).
		SetQuotaUsed(0).
		SetExpiresAt(binding.ExpiresAt).
		SetRateLimit5h(binding.RateLimit5h).
		SetRateLimit7d(binding.RateLimit7d).
		SetUsage5h(0).
		SetUsage7d(0).
		ClearWindow5hStart().
		ClearWindow7dStart().
		SetUpdatedAt(now)
	if row.Status == service.StatusAPIKeyExpired || row.Status == service.StatusAPIKeyQuotaExhausted {
		update.SetStatus(service.StatusActive)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, service.ErrAPIKeyPackageKeyUnavailable
	}
	key, err := getPackageKeyWithClient(ctx, client, binding.UserID, binding.KeyID)
	if err != nil {
		return nil, err
	}
	if ownsTx {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return key, nil
}

func (r *apiKeyPackageRepository) UpgradeKey(ctx context.Context, upgrade service.APIKeyPackageKeyUpgrade) (*service.APIKey, error) {
	row, err := r.client.APIKey.Query().
		Where(
			apikey.IDEQ(upgrade.KeyID),
			apikey.UserIDEQ(upgrade.UserID),
			apikey.PackageIDEQ(upgrade.SourcePackageID),
			apikey.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyPackageKeyNotFound
		}
		return nil, err
	}
	if row.ExpiresAt == nil || !row.ExpiresAt.After(upgrade.At) {
		return nil, service.ErrAPIKeyPackageKeyExpired
	}

	update := r.client.APIKey.Update().
		Where(
			apikey.IDEQ(upgrade.KeyID),
			apikey.UserIDEQ(upgrade.UserID),
			apikey.PackageIDEQ(upgrade.SourcePackageID),
			apikey.DeletedAtIsNil(),
			apikey.ExpiresAtGT(upgrade.At),
		).
		SetPackageID(upgrade.TargetPackageID).
		SetQuota(upgrade.Quota).
		SetRateLimit5h(upgrade.RateLimit5h).
		SetRateLimit7d(upgrade.RateLimit7d).
		SetUpdatedAt(upgrade.At)
	if row.Status == service.StatusAPIKeyActive || row.Status == service.StatusAPIKeyQuotaExhausted || row.Status == service.StatusAPIKeyExpired {
		if row.QuotaUsed >= upgrade.Quota {
			update.SetStatus(service.StatusAPIKeyQuotaExhausted)
		} else {
			update.SetStatus(service.StatusAPIKeyActive)
		}
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, service.ErrAPIKeyPackageKeyNotFound
	}
	return r.getPackageKey(ctx, upgrade.UserID, upgrade.KeyID)
}

func (r *apiKeyPackageRepository) UnbindKey(ctx context.Context, userID, packageID, keyID int64) (*service.APIKey, error) {
	row, err := r.client.APIKey.Query().
		Where(
			apikey.IDEQ(keyID),
			apikey.UserIDEQ(userID),
			apikey.PackageIDEQ(packageID),
			apikey.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyPackageKeyNotFound
		}
		return nil, err
	}

	update := r.client.APIKey.Update().
		Where(
			apikey.IDEQ(keyID),
			apikey.UserIDEQ(userID),
			apikey.PackageIDEQ(packageID),
			apikey.DeletedAtIsNil(),
		).
		ClearPackageID().
		SetQuota(0).
		SetQuotaUsed(0).
		ClearExpiresAt().
		SetRateLimit5h(0).
		SetRateLimit7d(0).
		SetUsage5h(0).
		SetUsage7d(0).
		ClearWindow5hStart().
		ClearWindow7dStart().
		SetUpdatedAt(time.Now())
	if row.Status == service.StatusAPIKeyExpired || row.Status == service.StatusAPIKeyQuotaExhausted {
		update.SetStatus(service.StatusActive)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, service.ErrAPIKeyPackageKeyNotFound
	}
	return r.getPackageKey(ctx, userID, keyID)
}

func (r *apiKeyPackageRepository) getPackageKey(ctx context.Context, userID, keyID int64) (*service.APIKey, error) {
	return getPackageKeyWithClient(ctx, r.client, userID, keyID)
}

func getPackageKeyWithClient(ctx context.Context, client *dbent.Client, userID, keyID int64) (*service.APIKey, error) {
	row, err := client.APIKey.Query().
		Where(apikey.IDEQ(keyID), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		WithGroup().
		Only(ctx)
	if err != nil {
		if dbent.IsNotFound(err) {
			return nil, service.ErrAPIKeyNotFound
		}
		return nil, err
	}
	return apiKeyEntityToService(row), nil
}

func apiKeyPackageEntityToService(row *dbent.APIKeyPackage) *service.APIKeyPackage {
	pkg := &service.APIKeyPackage{}
	fillAPIKeyPackage(pkg, row)
	return pkg
}

func fillAPIKeyPackage(pkg *service.APIKeyPackage, row *dbent.APIKeyPackage) {
	pkg.ID = row.ID
	pkg.UserID = row.UserID
	pkg.Name = row.Name
	pkg.Cycle = row.Cycle
	pkg.AmountUSD = mathRoundMoney(row.AmountUsd)
	pkg.Limit5hEnabled = row.Limit5hEnabled
	pkg.Limit7dEnabled = row.Limit7dEnabled
	pkg.CreatedAt = row.CreatedAt
	pkg.UpdatedAt = row.UpdatedAt
}

func mathRoundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}
