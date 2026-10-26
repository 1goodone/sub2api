package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestBPSModelUnavailableSettingDefaultAndValidation(t *testing.T) {
	var missing *SettingService
	require.Equal(t, 10*time.Minute, missing.GetExcelBPSModelUnavailableCooldown(t.Context()))
	for _, tc := range []struct {
		raw     string
		minutes int
	}{
		{"", 10}, {"1", 1}, {" 7 ", 7}, {"10", 10}, {"43200", 43200}, {"0", 10}, {"-1", 10}, {"43201", 10}, {"1.5", 10}, {"bad", 10},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			repo := &excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSModelUnavailableCooldownMinutes: tc.raw}}
			settings := NewSettingService(repo, &config.Config{})
			require.Equal(t, time.Duration(tc.minutes)*time.Minute, settings.GetExcelBPSModelUnavailableCooldown(t.Context()))
			all, err := settings.GetAllSettings(t.Context())
			require.NoError(t, err)
			require.Equal(t, tc.minutes, all.ExcelBPSModelUnavailableCooldownMinutes)
			require.Equal(t, 1440, all.ExcelBPSModelRateLimitCooldownMinutes, "model limits keep their independent default")
			repo.err = errors.New("database unavailable")
			require.Equal(t, 10*time.Minute, settings.GetExcelBPSModelUnavailableCooldown(t.Context()))
		})
	}
}

func TestBPSModelUnavailableSettingHotReload(t *testing.T) {
	s, a, _ := modelRecoveryTestService()
	settings, err := s.settingService.GetAllSettings(t.Context())
	require.NoError(t, err)
	settings.ExcelBPSRecoveryEnabled = true
	settings.ExcelBPSModelUnavailableCooldownMinutes = 25
	require.NoError(t, s.settingService.UpdateSettings(t.Context(), settings))
	ctx := context.WithValue(t.Context(), excelBPSRecoveryKey{}, true)
	before := time.Now()
	s.coolDownExcelBPSModel(ctx, a, "gpt-6-astra")
	entry := s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-6-astra"}]
	require.WithinDuration(t, before.Add(25*time.Minute), entry.nextProbeAt, time.Second)
	deadline := entry.nextProbeAt
	settings.ExcelBPSModelUnavailableCooldownMinutes = 5
	require.NoError(t, s.settingService.UpdateSettings(t.Context(), settings))
	require.Equal(t, deadline, entry.nextProbeAt, "saving does not rewrite existing cooldowns")
	s.coolDownExcelBPSModel(ctx, a, "gpt-6-astra")
	require.Equal(t, deadline, s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-6-astra"}].nextProbeAt, "concurrent errors cannot shorten a later deadline")
	before = time.Now()
	s.coolDownExcelBPSModel(ctx, a, "gpt-5.6-sol")
	require.WithinDuration(t, before.Add(5*time.Minute), s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-5.6-sol"}].nextProbeAt, time.Second)
	for _, invalid := range []int{-1, 43201} {
		settings.ExcelBPSModelUnavailableCooldownMinutes = invalid
		require.Error(t, s.settingService.UpdateSettings(t.Context(), settings))
		require.Equal(t, 5*time.Minute, s.settingService.GetExcelBPSModelUnavailableCooldown(t.Context()))
	}
}
