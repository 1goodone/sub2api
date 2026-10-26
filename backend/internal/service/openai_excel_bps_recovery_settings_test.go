package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func openAIExcelBPSRecoveryTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	svc := openAIClientToolsTestService(upstream)
	svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSRecoveryEnabled: "true"}}, svc.cfg)
	return svc
}

func TestExcelBPSRecoverySwitchDefaultsOffAndHotReloads(t *testing.T) {
	ctx := context.Background()
	repo := &excelBPSImageSettingsRepo{}
	svc := openAIClientToolsTestService(nil)
	svc.settingService = NewSettingService(repo, svc.cfg)
	require.False(t, svc.excelBPSRecoveryEnabled(ctx))
	settings, err := svc.settingService.GetAllSettings(ctx)
	require.NoError(t, err)
	require.False(t, settings.ExcelBPSRecoveryEnabled)
	settings.ExcelBPSRecoveryEnabled = true
	require.NoError(t, svc.settingService.UpdateSettings(ctx, settings))
	require.True(t, svc.excelBPSRecoveryEnabled(ctx))
	inFlight := svc.withExcelBPSRecovery(ctx)
	routed := WithExcelBPSScheduling(ctx, nil)
	require.True(t, svc.excelBPSRecoveryEnabled(routed))
	settings.ExcelBPSRecoveryEnabled = false
	require.NoError(t, svc.settingService.UpdateSettings(ctx, settings))
	require.False(t, svc.excelBPSRecoveryEnabled(ctx))
	require.True(t, svc.excelBPSRecoveryEnabled(inFlight))
	require.True(t, svc.excelBPSRecoveryEnabled(routed), "subsequent account attempts share the original request snapshot")
	require.False(t, svc.excelBPSRecoveryEnabled(WithExcelBPSScheduling(ctx, nil)))
	repo.err = errors.New("database unavailable")
	require.False(t, svc.excelBPSRecoveryEnabled(ctx))
}

func TestExcelBPSRecoverySwitchControlsFailover(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			status int
			wire   string
		}{
			{"server", 500, `{"error":{"code":"server_error"}}`},
			{"model", 403, `{"error":{"code":"model_not_allowed"}}`},
			{"missing model", 404, `{"error":{"code":"model_not_found"}}`},
			{"stream rate limit", 200, bpsFailureWire(t, "response.failed", "rate_limit_exceeded", "", 0)},
			{"stream EOF", 200, ""},
		} {
			t.Run(tc.name+"/"+strconv.FormatBool(enabled), func(t *testing.T) {
				upstream := &httpUpstreamRecorder{resp: bpsCompletionResponse(tc.status, tc.wire)}
				svc := openAIClientToolsTestService(upstream)
				svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSRecoveryEnabled: strconv.FormatBool(enabled)}}, svc.cfg)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/responses", nil)
				_, err := svc.Forward(context.Background(), c, excelAccount(), []byte(`{"model":"gpt-6-astra","input":"hello","stream":true}`))
				require.Error(t, err)
				if tc.status == 403 || tc.status == 404 {
					require.Equal(t, enabled, svc.isExcelBPSModelCoolingDown(context.Background(), excelAccount(), "gpt-6-astra"))
				}
				var failure *UpstreamFailoverError
				if enabled {
					require.ErrorAs(t, err, &failure)
					require.Empty(t, rec.Body.String())
				} else {
					require.NotErrorAs(t, err, &failure)
					require.NotEmpty(t, rec.Body.String())
				}
			})
		}
	}
}

func TestExcelBPSRecoveryDisabledPreservesLegacyCredentialPolicy(t *testing.T) {
	svc := openAIClientToolsTestService(nil)
	repo, _ := setupExcelBPSAuth(svc)
	account := excelAccount()
	account.Credentials["refresh_token"] = "test-refresh"
	svc.handleExcelBPSUnauthorized(context.Background(), account, 401, nil, []byte(`{}`), "token")
	require.Zero(t, repo.errorCalls)
	require.Equal(t, 1, repo.tempCalls)
	svc.pauseExcelBPSCredentialError(context.Background(), excelAccount(), errors.New("access_token not found"))
	require.Zero(t, repo.errorCalls)
}

func TestExcelBPSRecoveryDisabledDoesNotRetryCorrection(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: bpsCompletionResponse(429, "rejected")}
	svc := openAIClientToolsTestService(upstream)
	req, err := newExcelBPSRequest(context.Background(), []byte(`{"input":"correction"}`), "token", "account")
	require.NoError(t, err)
	resp, err := svc.doExcelBPSRepairRequest(req, "", excelAccount())
	require.NoError(t, err)
	resp.Body.Close()
	require.Len(t, upstream.requests, 1)
}

func TestExcelBPSRecoverySwitchControlsRouteLock(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		svc := openAIClientToolsTestService(nil)
		svc.settingService = NewSettingService(&excelBPSImageSettingsRepo{values: map[string]string{SettingKeyExcelBPSRecoveryEnabled: strconv.FormatBool(enabled)}}, svc.cfg)
		ctx := svc.withExcelBPSRecovery(WithExcelBPSScheduling(context.Background(), nil))
		native := excelAccount()
		native.Extra["openai_excel_bps"] = false
		require.False(t, excelBPSFailoverRouteBlocked(ctx, native, "gpt-6-astra"))
		lockExcelBPSFailoverRoute(ctx, nil, excelAccount(), "gpt-6-astra")
		require.Equal(t, enabled, excelBPSFailoverRouteBlocked(ctx, native, "gpt-6-astra"))
		require.False(t, excelBPSFailoverRouteBlocked(ctx, excelAccount(), "gpt-6-astra"))
	}
}

func TestExcelBPSRecoverySwitchOffDoesNotResumePausedAccount(t *testing.T) {
	svc := openAIExcelBPSRecoveryTestService(nil)
	repo, _ := setupExcelBPSAuth(svc)
	account := excelAccount()
	svc.pauseExcelBPSCredentialError(context.Background(), account, errors.New("token_revoked"))
	require.Equal(t, 1, repo.errorCalls)
	svc.settingService.settingRepo.(*excelBPSImageSettingsRepo).values[SettingKeyExcelBPSRecoveryEnabled] = "false"
	require.False(t, svc.excelBPSRecoveryEnabled(context.Background()))
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	svc.pauseExcelBPSCredentialError(context.Background(), account, errors.New("token_revoked"))
	require.Equal(t, 1, repo.errorCalls)
}
