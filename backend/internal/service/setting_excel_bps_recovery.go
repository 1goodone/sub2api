package service

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	SettingKeyExcelBPSModelRateLimitCooldownMinutes = "excel_bps_model_rate_limit_cooldown_minutes"
	DefaultExcelBPSModelRateLimitCooldownMinutes    = 24 * 60
	MaxExcelBPSModelRateLimitCooldownMinutes        = 30 * 24 * 60
)

func parseExcelBPSModelRateLimitCooldownMinutes(raw string) int {
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes < 1 || minutes > MaxExcelBPSModelRateLimitCooldownMinutes {
		return DefaultExcelBPSModelRateLimitCooldownMinutes
	}
	return minutes
}

// GetExcelBPSModelRateLimitCooldown reads the saved policy only on a model-limit
// failure. Changes apply without restarting; existing deadlines remain intact.
func (s *SettingService) GetExcelBPSModelRateLimitCooldown(ctx context.Context) time.Duration {
	minutes := DefaultExcelBPSModelRateLimitCooldownMinutes
	if s != nil && s.settingRepo != nil {
		dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayForwardingDBTimeout)
		defer cancel()
		if raw, err := s.settingRepo.GetValue(dbCtx, SettingKeyExcelBPSModelRateLimitCooldownMinutes); err == nil {
			minutes = parseExcelBPSModelRateLimitCooldownMinutes(raw)
		}
	}
	return time.Duration(minutes) * time.Minute
}
