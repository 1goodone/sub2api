package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestExcelBPSRecoverySettingsRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	read := func() bool {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
		h.GetSettings(c)
		require.Equal(t, http.StatusOK, rec.Code)
		value := gjson.GetBytes(rec.Body.Bytes(), "data.excel_bps_recovery_enabled")
		require.True(t, value.Exists())
		return value.Bool()
	}
	require.False(t, read(), "new installations and upgrades must default off")
	for _, enabled := range []bool{true, false} {
		rec := doUpdateSettings(t, h, map[string]any{"excel_bps_recovery_enabled": enabled}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, enabled, gjson.GetBytes(rec.Body.Bytes(), "data.excel_bps_recovery_enabled").Bool())
		require.Equal(t, enabled, read())
		rec = doUpdateSettings(t, h, map[string]any{"site_name": "unchanged recovery switch"}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, enabled, read(), "older clients must preserve omitted switch")
		require.Equal(t, enabled, repo.values[service.SettingKeyExcelBPSRecoveryEnabled] == "true")
	}
}

func TestExcelBPSRecoverySettingsRejectNonBoolean(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyExcelBPSRecoveryEnabled: "true"})
	rec := doUpdateSettings(t, h, map[string]any{"excel_bps_recovery_enabled": "false"}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "true", repo.values[service.SettingKeyExcelBPSRecoveryEnabled])
}

func TestExcelBPSModelRateLimitSettingsRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{})
	read := func() int64 {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
		h.GetSettings(c)
		require.Equal(t, http.StatusOK, rec.Code)
		value := gjson.GetBytes(rec.Body.Bytes(), "data.excel_bps_model_rate_limit_cooldown_minutes")
		require.True(t, value.Exists())
		return value.Int()
	}
	require.Equal(t, int64(10), read())
	for _, minutes := range []int64{1, 30, 1440, 43200} {
		rec := doUpdateSettings(t, h, map[string]any{"excel_bps_model_rate_limit_cooldown_minutes": minutes}, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		require.Equal(t, minutes, gjson.GetBytes(rec.Body.Bytes(), "data.excel_bps_model_rate_limit_cooldown_minutes").Int())
		require.Equal(t, minutes, read())
		rec = doUpdateSettings(t, h, map[string]any{"site_name": "preserve BPS cooldown"}, nil)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, minutes, read(), "partial settings updates preserve the saved cooldown")
		require.NotEmpty(t, repo.values[service.SettingKeyExcelBPSModelRateLimitCooldownMinutes])
	}
}

func TestExcelBPSModelRateLimitSettingsRejectInvalid(t *testing.T) {
	for _, value := range []any{0, -1, 43201, 1.5, "30", true, []int{30}} {
		h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyExcelBPSModelRateLimitCooldownMinutes: "60"})
		rec := doUpdateSettings(t, h, map[string]any{"excel_bps_model_rate_limit_cooldown_minutes": value}, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, "invalid input: %v", value)
		require.Equal(t, "60", repo.values[service.SettingKeyExcelBPSModelRateLimitCooldownMinutes])
	}
}
