package service

import (
	"context"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service/basispoints"
)

const (
	SettingKeyExcelBPSImageRelayEnabled = "excel_bps_image_relay_enabled"
	SettingKeyExcelBPSImageBaseURL      = "excel_bps_image_base_url"
)

type ExcelBPSImageRelaySettings struct {
	Enabled bool
	BaseURL string
}

func normalizeExcelBPSImageRelaySettings(enabled bool, baseURL string) (ExcelBPSImageRelaySettings, error) {
	baseURL = strings.TrimSpace(baseURL)
	if enabled || baseURL != "" {
		if err := basispoints.ValidateImageRelayOrigin(baseURL); err != nil {
			return ExcelBPSImageRelaySettings{}, infraerrors.BadRequest("INVALID_EXCEL_BPS_IMAGE_BASE_URL", err.Error())
		}
	}
	return ExcelBPSImageRelaySettings{Enabled: enabled, BaseURL: strings.TrimRight(baseURL, "/")}, nil
}

// Read only these two settings for each request so saves take effect immediately,
// including on other instances sharing the settings database.
func (s *SettingService) GetExcelBPSImageRelaySettings(ctx context.Context) (ExcelBPSImageRelaySettings, error) {
	if s == nil || s.settingRepo == nil {
		return ExcelBPSImageRelaySettings{}, nil
	}
	dbCtx, cancel := context.WithTimeout(ctx, gatewayForwardingDBTimeout)
	defer cancel()
	values, err := s.settingRepo.GetMultiple(dbCtx, []string{SettingKeyExcelBPSImageRelayEnabled, SettingKeyExcelBPSImageBaseURL})
	if err != nil {
		return ExcelBPSImageRelaySettings{}, infraerrors.ServiceUnavailable("EXCEL_BPS_IMAGE_SETTINGS_UNAVAILABLE", "Excel BPS image settings are unavailable")
	}
	return normalizeExcelBPSImageRelaySettings(values[SettingKeyExcelBPSImageRelayEnabled] == "true", values[SettingKeyExcelBPSImageBaseURL])
}
