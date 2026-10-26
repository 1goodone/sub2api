package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBPSModelSharedCooldownSettingHotReload(t *testing.T) {
	s, a, _ := modelRecoveryTestService()
	settings, err := s.settingService.GetAllSettings(t.Context())
	require.NoError(t, err)
	settings.ExcelBPSRecoveryEnabled = true
	settings.ExcelBPSModelRateLimitCooldownMinutes = 25
	require.NoError(t, s.settingService.UpdateSettings(t.Context(), settings))
	ctx := context.WithValue(t.Context(), excelBPSRecoveryKey{}, true)
	before := time.Now()
	s.coolDownExcelBPSModel(ctx, a, "gpt-6-astra")
	entry := s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-6-astra"}]
	require.WithinDuration(t, before.Add(25*time.Minute), entry.nextProbeAt, time.Second)
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-6-astra", nil, nil, nil, 429)
	require.WithinDuration(t, entry.nextProbeAt, bpsRateLimitState(t, s, a, "gpt-6-astra").until, time.Second, "both policies use the existing saved value")
	deadline := entry.nextProbeAt
	settings.ExcelBPSModelRateLimitCooldownMinutes = 5
	require.NoError(t, s.settingService.UpdateSettings(t.Context(), settings))
	require.Equal(t, deadline, entry.nextProbeAt, "saving does not rewrite existing cooldowns")
	s.coolDownExcelBPSModel(ctx, a, "gpt-6-astra")
	require.Equal(t, deadline, s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-6-astra"}].nextProbeAt, "concurrent errors cannot shorten a later deadline")
	before = time.Now()
	s.coolDownExcelBPSModel(ctx, a, "gpt-5.6-sol")
	require.WithinDuration(t, before.Add(5*time.Minute), s.excelBPSModelCooldowns.entries[excelBPSModelKey{a.ID, "gpt-5.6-sol"}].nextProbeAt, time.Second)
	s.coolDownExcelBPSAttempt(ctx, a, "gpt-5.6-sol", nil, nil, nil, 429)
	require.WithinDuration(t, before.Add(5*time.Minute), bpsRateLimitState(t, s, a, "gpt-5.6-sol").until, time.Second)
	for _, invalid := range []int{-1, 43201} {
		settings.ExcelBPSModelRateLimitCooldownMinutes = invalid
		require.Error(t, s.settingService.UpdateSettings(t.Context(), settings))
		require.Equal(t, 5*time.Minute, s.settingService.GetExcelBPSModelRateLimitCooldown(t.Context()))
	}
}
