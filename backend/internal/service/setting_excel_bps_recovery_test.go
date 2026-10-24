package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBPSModelRateLimitSettingsDefaultsAndValidation(t *testing.T) {
	ctx := context.Background()
	var missing *SettingService
	require.Equal(t, 24*time.Hour, missing.GetExcelBPSModelRateLimitCooldown(ctx))
	for _, tc := range []struct {
		raw     string
		minutes int
	}{
		{"", 1440}, {"1", 1}, {"30", 30}, {"1440", 1440}, {"43200", 43200}, {" 60 ", 60},
		{"0", 1440}, {"-1", 1440}, {"43201", 1440}, {"1.5", 1440}, {"bad", 1440}, {"9223372036854775807", 1440},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			repo := &excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSModelRateLimitCooldownMinutes: tc.raw}}
			settings := NewSettingService(repo, &config.Config{})
			require.Equal(t, time.Duration(tc.minutes)*time.Minute, settings.GetExcelBPSModelRateLimitCooldown(ctx))
			all, err := settings.GetAllSettings(ctx)
			require.NoError(t, err)
			require.Equal(t, tc.minutes, all.ExcelBPSModelRateLimitCooldownMinutes)
			repo.err = errors.New("database unavailable")
			require.Equal(t, 24*time.Hour, settings.GetExcelBPSModelRateLimitCooldown(ctx))
		})
	}
}

func TestBPSModelRateLimitSettingsSaveAndHotReload(t *testing.T) {
	ctx := context.WithValue(context.Background(), excelBPSRecoveryKey{}, true)
	repo := &excelBPSImageSettingsRepo{}
	settings := NewSettingService(repo, &config.Config{})
	s := &OpenAIGatewayService{settingService: settings}
	a := excelAccount()
	all, err := settings.GetAllSettings(ctx)
	require.NoError(t, err)
	all.ExcelBPSRecoveryEnabled = true
	all.ExcelBPSModelRateLimitCooldownMinutes = 180
	require.NoError(t, settings.UpdateSettings(ctx, all))
	require.Equal(t, "180", repo.values[SettingKeyExcelBPSModelRateLimitCooldownMinutes])
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, nil, nil, 429)
	original := bpsRateLimitState(t, s, a, "gpt-6.1-sol")
	require.WithinDuration(t, time.Now().Add(180*time.Minute), original.until, 2*time.Second)
	all.ExcelBPSModelRateLimitCooldownMinutes = 30
	require.NoError(t, settings.UpdateSettings(ctx, all))
	require.Equal(t, 30*time.Minute, settings.GetExcelBPSModelRateLimitCooldown(ctx))
	require.Equal(t, original, bpsRateLimitState(t, s, a, "gpt-6.1-sol"), "settings updates preserve existing deadlines")
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6.1-sol", nil, nil, nil, 429)
	require.Equal(t, original.until, bpsRateLimitState(t, s, a, "gpt-6.1-sol").until, "a shorter new policy must not shorten an active restriction")
	other := *a
	other.ID++
	s.coolDownExcelBPSAttempt(ctx, &other, "gpt-6.1-sol", nil, nil, nil, 429)
	require.WithinDuration(t, time.Now().Add(30*time.Minute), bpsRateLimitState(t, s, &other, "gpt-6.1-sol").until, 2*time.Second)
	require.False(t, s.isExcelBPSCoolingDown(ctx, a, "gpt-6-astra"))
	require.False(t, s.isExcelBPSCoolingDown(context.WithValue(ctx, excelBPSSchedulingKey{}, false), a, "gpt-6.1-sol"))
	all.ExcelBPSRecoveryEnabled = false
	require.NoError(t, settings.UpdateSettings(context.Background(), all))
	require.False(t, s.isExcelBPSCoolingDown(context.Background(), a, "gpt-6.1-sol"))
	for _, invalid := range []int{-1, 43201} {
		all.ExcelBPSModelRateLimitCooldownMinutes = invalid
		require.Error(t, settings.UpdateSettings(ctx, all))
		require.Equal(t, "30", repo.values[SettingKeyExcelBPSModelRateLimitCooldownMinutes])
	}
}

func TestBPSModelRateLimitConfiguredForwardDuration(t *testing.T) {
	for _, status := range []int{200, 429} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			wire := `{"error":{"code":"rate_limit_exceeded","resets_in_seconds":5}}`
			if status == 200 {
				wire = bpsFailureWire(t, "response.failed", "rate_limit_exceeded", "", 0)
			}
			s := openAIExcelBPSRecoveryTestService(&httpUpstreamRecorder{resp: bpsCompletionResponse(status, wire)})
			repo := s.settingService.settingRepo.(*excelBPSImageSettingsRepo)
			repo.values[SettingKeyExcelBPSModelRateLimitCooldownMinutes] = "45"
			a := excelAccount()
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			_, err := s.Forward(context.Background(), c, a, []byte(`{"model":"gpt-6.1-sol","input":"hello","stream":true}`))
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.True(t, failover.ShouldRetryNextAccount())
			require.Empty(t, rec.Body.String())
			require.WithinDuration(t, time.Now().Add(45*time.Minute), bpsRateLimitState(t, s, a, "gpt-6.1-sol").until, 2*time.Second)
		})
	}
}
