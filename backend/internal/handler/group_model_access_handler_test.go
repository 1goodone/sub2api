package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayMessagesRejectsDisabledModelBeforeRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-blocked","max_tokens":1,"messages":[]}`))
	groupID := int64(3)
	apiKey := &service.APIKey{
		ID:      1,
		UserID:  2,
		GroupID: &groupID,
		Group: &service.Group{
			ID:             groupID,
			Platform:       service.PlatformAnthropic,
			DisabledModels: []string{"CLAUDE-BLOCKED"},
		},
	}
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: apiKey.UserID, Concurrency: 1})

	(&GatewayHandler{cfg: &config.Config{}}).Messages(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), groupDisabledModelMessage)
	reason, ok := c.Get(service.OpsClientBusinessLimitedReasonKey)
	require.True(t, ok)
	require.Equal(t, service.OpsClientBusinessLimitedReasonLocalPolicyDenied, reason)
}

func TestGeminiGetModelRejectsDisabledModelBeforeUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/gemini-blocked", nil)
	c.Params = gin.Params{{Key: "model", Value: "gemini-blocked"}}
	groupID := int64(3)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		ID:      1,
		GroupID: &groupID,
		Group: &service.Group{
			ID:             groupID,
			Platform:       service.PlatformGemini,
			DisabledModels: []string{"models/GEMINI-BLOCKED"},
		},
	})

	(&GatewayHandler{}).GeminiV1BetaGetModel(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), groupDisabledModelMessage)
}

func TestAntigravityModelsHidesDisabledModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	models := antigravity.DefaultModels()
	require.NotEmpty(t, models)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/antigravity/v1/models", nil)
	c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{
		Group: &service.Group{DisabledModels: []string{models[0].ID}},
	})

	(&GatewayHandler{}).AntigravityModels(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	for _, model := range response.Data {
		require.NotEqual(t, models[0].ID, model.ID)
	}
}
