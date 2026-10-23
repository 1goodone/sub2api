package handler

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type excelBPSRecoverySettingsRepo struct {
	service.SettingRepository
	enabled bool
}

func (r *excelBPSRecoverySettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string)
	for _, key := range keys {
		if key == service.SettingKeyExcelBPSRecoveryEnabled {
			values[key] = strconv.FormatBool(r.enabled)
		}
	}
	return values, nil
}

func (r *excelBPSRecoverySettingsRepo) GetValue(ctx context.Context, key string) (string, error) {
	values, _ := r.GetMultiple(ctx, []string{key})
	if value, ok := values[key]; ok {
		return value, nil
	}
	return "", service.ErrSettingNotFound
}

func newExcelBPSRecoverySettings(enabled bool) *service.SettingService {
	return service.NewSettingService(&excelBPSRecoverySettingsRepo{enabled: enabled}, &config.Config{})
}
