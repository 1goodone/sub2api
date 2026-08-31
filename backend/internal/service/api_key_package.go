package service

import (
	"context"
	"math"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	APIKeyPackageCycleWeek  = "week"
	APIKeyPackageCycleMonth = "month"

	APIKeyPackagePlanGo     = "go"
	APIKeyPackagePlanPlus   = "plus"
	APIKeyPackagePlanPro5x  = "pro_5x"
	APIKeyPackagePlanPro20x = "pro_20x"
	APIKeyPackagePlanPro30x = "pro_30x"

	apiKeyPackageWeekHours  = 7 * 24
	apiKeyPackageMonthHours = 30 * 24
)

var apiKeyPackagePlans = []APIKeyPackagePlan{
	{ID: APIKeyPackagePlanGo, Name: "Go", Cycle: APIKeyPackageCycleMonth, AmountUSD: 70, Limit5hEnabled: true, Limit7dEnabled: true},
	{ID: APIKeyPackagePlanPlus, Name: "Plus", Cycle: APIKeyPackageCycleMonth, AmountUSD: 140, Limit5hEnabled: true, Limit7dEnabled: true},
	{ID: APIKeyPackagePlanPro5x, Name: "Pro 5x", Cycle: APIKeyPackageCycleMonth, AmountUSD: 700, Limit5hEnabled: true, Limit7dEnabled: true},
	{ID: APIKeyPackagePlanPro20x, Name: "Pro 20x", Cycle: APIKeyPackageCycleMonth, AmountUSD: 1400, Limit5hEnabled: true, Limit7dEnabled: true},
	{ID: APIKeyPackagePlanPro30x, Name: "Pro 30x", Cycle: APIKeyPackageCycleMonth, AmountUSD: 1800, Limit5hEnabled: true, Limit7dEnabled: true},
}

var (
	ErrAPIKeyPackageNotFound       = infraerrors.NotFound("API_KEY_PACKAGE_NOT_FOUND", "套餐不存在")
	ErrAPIKeyPackageInvalid        = infraerrors.BadRequest("API_KEY_PACKAGE_INVALID", "套餐配置无效")
	ErrAPIKeyPackageNotEmpty       = infraerrors.Conflict("API_KEY_PACKAGE_NOT_EMPTY", "套餐下仍有 Key，无法删除")
	ErrAPIKeyPackageKeyUnavailable = infraerrors.Conflict("API_KEY_PACKAGE_KEY_UNAVAILABLE", "Key 不存在或已绑定其他套餐")
	ErrAPIKeyPackageAlreadyActive  = infraerrors.Conflict("API_KEY_PACKAGE_ALREADY_ACTIVE", "用户已有生效套餐，请先取消当前套餐")
	ErrAPIKeyPackageKeyNotFound    = infraerrors.NotFound("API_KEY_PACKAGE_KEY_NOT_FOUND", "套餐中不存在该 Key")
	ErrAPIKeyPackageUpgradeOnly    = infraerrors.Conflict("API_KEY_PACKAGE_UPGRADE_ONLY", "只能升级到价格更高的套餐")
	ErrAPIKeyPackageKeyExpired     = infraerrors.Conflict("API_KEY_PACKAGE_KEY_EXPIRED", "套餐已到期，请先取消后重新选择")
)

type APIKeyPackage struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	PlanID         string    `json:"plan_id"`
	Name           string    `json:"name"`
	Cycle          string    `json:"cycle"`
	AmountUSD      float64   `json:"amount_usd"`
	Limit5hEnabled bool      `json:"limit_5h_enabled"`
	Limit7dEnabled bool      `json:"limit_7d_enabled"`
	KeyCount       int       `json:"key_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SaveAPIKeyPackageRequest struct {
	PlanID string `json:"plan_id"`
}

type APIKeyPackagePlan struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Cycle          string  `json:"cycle"`
	AmountUSD      float64 `json:"amount_usd"`
	Limit5hEnabled bool    `json:"limit_5h_enabled"`
	Limit7dEnabled bool    `json:"limit_7d_enabled"`
}

type BindPackageAPIKeyRequest struct {
	KeyID int64 `json:"key_id"`
}

type UpgradePackageAPIKeyRequest struct {
	PlanID string `json:"plan_id"`
}

type APIKeyPackageKeyBinding struct {
	UserID      int64
	PackageID   int64
	KeyID       int64
	Quota       float64
	ExpiresAt   time.Time
	RateLimit5h float64
	RateLimit7d float64
}

type APIKeyPackageKeyUpgrade struct {
	UserID          int64
	SourcePackageID int64
	TargetPackageID int64
	KeyID           int64
	Quota           float64
	RateLimit5h     float64
	RateLimit7d     float64
	At              time.Time
}

type APIKeyPackageRepository interface {
	Create(ctx context.Context, pkg *APIKeyPackage) error
	ListByUserID(ctx context.Context, userID int64) ([]APIKeyPackage, error)
	GetByID(ctx context.Context, userID, id int64) (*APIKeyPackage, error)
	Update(ctx context.Context, pkg *APIKeyPackage, binding APIKeyPackageKeyBinding) ([]APIKey, error)
	Delete(ctx context.Context, userID, id int64) error
	CountKeys(ctx context.Context, packageID int64) (int, error)
	BindKey(ctx context.Context, binding APIKeyPackageKeyBinding) (*APIKey, error)
	UpgradeKey(ctx context.Context, upgrade APIKeyPackageKeyUpgrade) (*APIKey, error)
	UnbindKey(ctx context.Context, userID, packageID, keyID int64) (*APIKey, error)
}

type APIKeyPackageService struct {
	repo      APIKeyPackageRepository
	apiKeySvc *APIKeyService
}

func NewAPIKeyPackageService(repo APIKeyPackageRepository, apiKeySvc *APIKeyService) *APIKeyPackageService {
	return &APIKeyPackageService{repo: repo, apiKeySvc: apiKeySvc}
}

func ListAPIKeyPackagePlans() []APIKeyPackagePlan {
	return append([]APIKeyPackagePlan(nil), apiKeyPackagePlans...)
}

func (s *APIKeyPackageService) List(ctx context.Context, userID int64) ([]APIKeyPackage, error) {
	packages, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range packages {
		packages[i].PlanID = apiKeyPackagePlanID(&packages[i])
	}
	return packages, nil
}

func (s *APIKeyPackageService) Create(ctx context.Context, userID int64, req SaveAPIKeyPackageRequest) (*APIKeyPackage, error) {
	pkg, err := newAPIKeyPackage(userID, req)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range existing {
		if apiKeyPackagePlanID(&existing[i]) == pkg.PlanID {
			existing[i].PlanID = pkg.PlanID
			return &existing[i], nil
		}
	}
	if err := s.repo.Create(ctx, pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func (s *APIKeyPackageService) Update(ctx context.Context, userID, id int64, req SaveAPIKeyPackageRequest) (*APIKeyPackage, error) {
	pkg, err := s.repo.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if pkg.KeyCount > 0 {
		return nil, ErrAPIKeyPackageUpgradeOnly
	}
	updated, err := newAPIKeyPackage(userID, req)
	if err != nil {
		return nil, err
	}
	updated.ID = pkg.ID
	updated.CreatedAt = pkg.CreatedAt
	updated.KeyCount = pkg.KeyCount
	keys, err := s.repo.Update(ctx, updated, packageAPIKeyBinding(updated, 0, time.Now()))
	if err != nil {
		return nil, err
	}
	for i := range keys {
		s.invalidateKeyCaches(ctx, &keys[i])
	}
	return updated, nil
}

func (s *APIKeyPackageService) Delete(ctx context.Context, userID, id int64) error {
	if _, err := s.repo.GetByID(ctx, userID, id); err != nil {
		return err
	}
	count, err := s.repo.CountKeys(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrAPIKeyPackageNotEmpty
	}
	return s.repo.Delete(ctx, userID, id)
}

func (s *APIKeyPackageService) ListKeys(ctx context.Context, userID, packageID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	if _, err := s.repo.GetByID(ctx, userID, packageID); err != nil {
		return nil, nil, err
	}
	return s.apiKeySvc.List(ctx, userID, params, APIKeyListFilters{PackageID: &packageID})
}

func (s *APIKeyPackageService) ListAvailableKeys(ctx context.Context, userID, packageID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	if _, err := s.repo.GetByID(ctx, userID, packageID); err != nil {
		return nil, nil, err
	}
	return s.apiKeySvc.List(ctx, userID, params, APIKeyListFilters{PackageUnassigned: true})
}

func (s *APIKeyPackageService) ListAllAvailableKeys(ctx context.Context, userID int64, params pagination.PaginationParams) ([]APIKey, *pagination.PaginationResult, error) {
	return s.apiKeySvc.List(ctx, userID, params, APIKeyListFilters{PackageUnassigned: true})
}

func (s *APIKeyPackageService) BindKey(ctx context.Context, userID, packageID int64, req BindPackageAPIKeyRequest) (*APIKey, error) {
	if req.KeyID <= 0 {
		return nil, ErrAPIKeyPackageInvalid
	}
	pkg, err := s.repo.GetByID(ctx, userID, packageID)
	if err != nil {
		return nil, err
	}
	pkg.PlanID = apiKeyPackagePlanID(pkg)
	if pkg.PlanID == "" {
		return nil, ErrAPIKeyPackageInvalid
	}
	packages, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range packages {
		if apiKeyPackagePlanID(&packages[i]) == "" {
			continue
		}
		if packages[i].KeyCount > 0 {
			return nil, ErrAPIKeyPackageAlreadyActive
		}
	}
	key, err := s.repo.BindKey(ctx, packageAPIKeyBinding(pkg, req.KeyID, time.Now()))
	if err != nil {
		return nil, err
	}
	s.invalidateKeyCaches(ctx, key)
	return key, nil
}

func (s *APIKeyPackageService) UpgradeKey(ctx context.Context, userID, packageID, keyID int64, req UpgradePackageAPIKeyRequest) (*APIKey, error) {
	if keyID <= 0 {
		return nil, ErrAPIKeyPackageInvalid
	}
	currentPackage, err := s.repo.GetByID(ctx, userID, packageID)
	if err != nil {
		return nil, err
	}
	currentPlan, currentOK := apiKeyPackagePlanByID(apiKeyPackagePlanID(currentPackage))
	targetPlan, targetOK := apiKeyPackagePlanByID(strings.ToLower(strings.TrimSpace(req.PlanID)))
	if !targetOK {
		return nil, ErrAPIKeyPackageInvalid
	}
	if !currentOK || targetPlan.AmountUSD <= currentPlan.AmountUSD {
		return nil, ErrAPIKeyPackageUpgradeOnly
	}

	targetPackage, err := s.Create(ctx, userID, SaveAPIKeyPackageRequest{PlanID: targetPlan.ID})
	if err != nil {
		return nil, err
	}
	key, err := s.repo.UpgradeKey(ctx, packageAPIKeyUpgrade(targetPackage, packageID, keyID, time.Now()))
	if err != nil {
		return nil, err
	}
	s.invalidateKeyCaches(ctx, key)
	return key, nil
}

func (s *APIKeyPackageService) UnbindKey(ctx context.Context, userID, packageID, keyID int64) (*APIKey, error) {
	if keyID <= 0 {
		return nil, ErrAPIKeyPackageInvalid
	}
	if _, err := s.repo.GetByID(ctx, userID, packageID); err != nil {
		return nil, err
	}
	key, err := s.repo.UnbindKey(ctx, userID, packageID, keyID)
	if err != nil {
		return nil, err
	}
	s.invalidateKeyCaches(ctx, key)
	return key, nil
}

func newAPIKeyPackage(userID int64, req SaveAPIKeyPackageRequest) (*APIKeyPackage, error) {
	planID := strings.ToLower(strings.TrimSpace(req.PlanID))
	plan, ok := apiKeyPackagePlanByID(planID)
	if !ok {
		return nil, ErrAPIKeyPackageInvalid
	}
	return &APIKeyPackage{
		UserID:         userID,
		PlanID:         plan.ID,
		Name:           plan.Name,
		Cycle:          plan.Cycle,
		AmountUSD:      plan.AmountUSD,
		Limit5hEnabled: plan.Limit5hEnabled,
		Limit7dEnabled: plan.Limit7dEnabled,
	}, nil
}

func apiKeyPackagePlanByID(planID string) (APIKeyPackagePlan, bool) {
	for _, plan := range apiKeyPackagePlans {
		if plan.ID == planID {
			return plan, true
		}
	}
	return APIKeyPackagePlan{}, false
}

func apiKeyPackagePlanID(pkg *APIKeyPackage) string {
	for _, plan := range apiKeyPackagePlans {
		if pkg.Name == plan.Name &&
			pkg.Cycle == plan.Cycle &&
			roundPackageAmount(pkg.AmountUSD) == plan.AmountUSD &&
			pkg.Limit5hEnabled == plan.Limit5hEnabled &&
			pkg.Limit7dEnabled == plan.Limit7dEnabled {
			return plan.ID
		}
	}
	return ""
}

func IsFixedAPIKeyPackage(pkg *APIKeyPackage) bool {
	return apiKeyPackagePlanID(pkg) != ""
}

func packageAPIKeyBinding(pkg *APIKeyPackage, keyID int64, now time.Time) APIKeyPackageKeyBinding {
	cycleHours := float64(apiKeyPackageMonthHours)
	expiresAt := now.AddDate(0, 0, 30)
	if pkg.Cycle == APIKeyPackageCycleWeek {
		cycleHours = apiKeyPackageWeekHours
		expiresAt = now.AddDate(0, 0, 7)
	}
	limit := func(enabled bool, hours float64) float64 {
		if !enabled {
			return 0
		}
		return math.Max(0.01, roundPackageAmount(pkg.AmountUSD*hours/cycleHours))
	}
	return APIKeyPackageKeyBinding{
		UserID:      pkg.UserID,
		PackageID:   pkg.ID,
		KeyID:       keyID,
		Quota:       roundPackageAmount(pkg.AmountUSD),
		ExpiresAt:   expiresAt,
		RateLimit5h: limit(pkg.Limit5hEnabled, 5),
		RateLimit7d: limit(pkg.Limit7dEnabled, 7*24),
	}
}

func packageAPIKeyUpgrade(pkg *APIKeyPackage, sourcePackageID, keyID int64, now time.Time) APIKeyPackageKeyUpgrade {
	binding := packageAPIKeyBinding(pkg, keyID, now)
	return APIKeyPackageKeyUpgrade{
		UserID:          pkg.UserID,
		SourcePackageID: sourcePackageID,
		TargetPackageID: pkg.ID,
		KeyID:           keyID,
		Quota:           binding.Quota,
		RateLimit5h:     binding.RateLimit5h,
		RateLimit7d:     binding.RateLimit7d,
		At:              now,
	}
}

func roundPackageAmount(value float64) float64 {
	return math.Round((value+math.Copysign(1e-12, value))*100) / 100
}

func (s *APIKeyPackageService) invalidateKeyCaches(ctx context.Context, key *APIKey) {
	if s.apiKeySvc == nil || key == nil {
		return
	}
	s.apiKeySvc.InvalidateAuthCacheByKey(ctx, key.Key)
	if s.apiKeySvc.rateLimitCacheInvalid != nil {
		_ = s.apiKeySvc.rateLimitCacheInvalid.InvalidateAPIKeyRateLimit(ctx, key.ID)
	}
}
