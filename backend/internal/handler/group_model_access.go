package handler

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const groupDisabledModelMessage = "Model is disabled for this group"

func apiKeyAllowsModel(c *gin.Context, apiKey *service.APIKey, model string) bool {
	if apiKey == nil || apiKey.Group == nil || strings.TrimSpace(model) == "" || !apiKey.Group.IsModelDisabled(model) {
		return true
	}
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalPolicyDenied)
	return false
}

func filterAPIKeyModels(apiKey *service.APIKey, models []string) []string {
	if apiKey == nil || apiKey.Group == nil {
		return models
	}
	return apiKey.Group.FilterDisabledModels(models)
}
