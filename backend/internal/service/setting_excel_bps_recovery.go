package service

import (
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	SettingKeyExcelBPSModelRateLimitCooldownMinutes = "excel_bps_model_rate_limit_cooldown_minutes"
	DefaultExcelBPSModelRateLimitCooldownMinutes    = 10
	MaxExcelBPSModelRateLimitCooldownMinutes        = 30 * 24 * 60
)

func parseExcelBPSModelRateLimitCooldownMinutes(raw string) int {
	minutes, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || minutes < 1 || minutes > MaxExcelBPSModelRateLimitCooldownMinutes {
		return DefaultExcelBPSModelRateLimitCooldownMinutes
	}
	return minutes
}

// GetExcelBPSModelRateLimitCooldown is shared by BPS model rate limits and
// unavailable-model recovery probes. Saved changes preserve existing deadlines.
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
