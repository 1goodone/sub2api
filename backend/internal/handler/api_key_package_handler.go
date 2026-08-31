package handler

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type APIKeyPackageHandler struct {
	service *service.APIKeyPackageService
}

func NewAPIKeyPackageHandler(packageService *service.APIKeyPackageService) *APIKeyPackageHandler {
	return &APIKeyPackageHandler{service: packageService}
}

func (h *APIKeyPackageHandler) ListPlans(c *gin.Context) {
	response.Success(c, service.ListAPIKeyPackagePlans())
}

func (h *APIKeyPackageHandler) List(c *gin.Context) {
	userID, ok := packageUserID(c)
	if !ok {
		return
	}
	packages, err := h.service.List(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, packages)
}

func (h *APIKeyPackageHandler) ListAllAvailableKeys(c *gin.Context) {
	userID, ok := packageUserID(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	keys, result, err := h.service.ListAllAvailableKeys(c.Request.Context(), userID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func (h *APIKeyPackageHandler) Create(c *gin.Context) {
	userID, ok := packageUserID(c)
	if !ok {
		return
	}
	var req service.SaveAPIKeyPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	executeUserIdempotentJSON(c, "user.api_key_packages.create", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.service.Create(ctx, userID, req)
	})
}

func (h *APIKeyPackageHandler) Update(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	var req service.SaveAPIKeyPackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	pkg, err := h.service.Update(c.Request.Context(), userID, packageID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, pkg)
}

func (h *APIKeyPackageHandler) Delete(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), userID, packageID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, nil)
}

func (h *APIKeyPackageHandler) ListKeys(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	keys, result, err := h.service.ListKeys(c.Request.Context(), userID, packageID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func (h *APIKeyPackageHandler) ListAvailableKeys(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "created_at", SortOrder: "desc"}
	keys, result, err := h.service.ListAvailableKeys(c.Request.Context(), userID, packageID, params)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]dto.APIKey, 0, len(keys))
	for i := range keys {
		out = append(out, *dto.APIKeyFromService(&keys[i]))
	}
	response.Paginated(c, out, result.Total, page, pageSize)
}

func (h *APIKeyPackageHandler) BindKey(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	var req service.BindPackageAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.KeyID <= 0 {
		response.BadRequest(c, "Invalid request: key_id is required")
		return
	}
	payload := struct {
		PackageID int64 `json:"package_id"`
		KeyID     int64 `json:"key_id"`
	}{PackageID: packageID, KeyID: req.KeyID}
	executeUserIdempotentJSON(c, "user.api_key_packages.bind_key", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		key, err := h.service.BindKey(ctx, userID, packageID, req)
		if err != nil {
			return nil, err
		}
		return dto.APIKeyFromService(key), nil
	})
}

func (h *APIKeyPackageHandler) UpgradeKey(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	keyID, err := strconv.ParseInt(c.Param("keyId"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	var req service.UpgradePackageAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanID == "" {
		response.BadRequest(c, "Invalid request: plan_id is required")
		return
	}
	payload := struct {
		PackageID int64  `json:"package_id"`
		KeyID     int64  `json:"key_id"`
		PlanID    string `json:"plan_id"`
	}{PackageID: packageID, KeyID: keyID, PlanID: req.PlanID}
	executeUserIdempotentJSON(c, "user.api_key_packages.upgrade_key", payload, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		key, err := h.service.UpgradeKey(ctx, userID, packageID, keyID, req)
		if err != nil {
			return nil, err
		}
		return dto.APIKeyFromService(key), nil
	})
}

func (h *APIKeyPackageHandler) UnbindKey(c *gin.Context) {
	userID, packageID, ok := packageRequestIDs(c)
	if !ok {
		return
	}
	keyID, err := strconv.ParseInt(c.Param("keyId"), 10, 64)
	if err != nil || keyID <= 0 {
		response.BadRequest(c, "Invalid key ID")
		return
	}
	key, err := h.service.UnbindKey(c.Request.Context(), userID, packageID, keyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.APIKeyFromService(key))
}

func packageUserID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func packageRequestIDs(c *gin.Context) (int64, int64, bool) {
	userID, ok := packageUserID(c)
	if !ok {
		return 0, 0, false
	}
	packageID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || packageID <= 0 {
		response.BadRequest(c, "Invalid package ID")
		return 0, 0, false
	}
	return userID, packageID, true
}
