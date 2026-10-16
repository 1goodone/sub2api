package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSettingsFirstResponseSaveResponseMatchesStoredAndReloadedState(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyOpenAITTFTMode: service.OpenAITTFTModeVisible,
	})
	for _, step := range []struct {
		name string
		body map[string]any
		want bool
	}{
		{"default", map[string]any{"site_name": "Gateway"}, false},
		{"enable", map[string]any{"stream_first_response_enabled": true}, true},
		{"omit enabled", map[string]any{"site_name": "Gateway"}, true},
		{"disable", map[string]any{"stream_first_response_enabled": false}, false},
		{"omit disabled", map[string]any{"site_name": "Gateway"}, false},
	} {
		t.Run(step.name, func(t *testing.T) {
			saved := doUpdateSettings(t, h, step.body, nil)
			require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
			require.Equal(t, step.want, gjson.Get(saved.Body.String(), "data.stream_first_response_enabled").Value())
			require.Equal(t, service.OpenAITTFTModeVisible, gjson.Get(saved.Body.String(), "data.openai_ttft_mode").String())
			require.Equal(t, step.want, repo.values[service.SettingKeyStreamFirstResponseEnabled] == "true")
			require.Equal(t, step.want, h.settingService.IsStreamFirstResponseEnabled(context.Background()))

			reloaded := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(reloaded)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
			h.GetSettings(c)
			require.Equal(t, http.StatusOK, reloaded.Code, reloaded.Body.String())
			require.Equal(t, step.want, gjson.Get(reloaded.Body.String(), "data.stream_first_response_enabled").Value())
			require.Equal(t, service.OpenAITTFTModeVisible, gjson.Get(reloaded.Body.String(), "data.openai_ttft_mode").String())
		})
	}
}
