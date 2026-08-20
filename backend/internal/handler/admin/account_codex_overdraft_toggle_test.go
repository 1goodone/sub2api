package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type codexOverdraftToggleAdminService struct {
	*stubAdminService
	account *service.Account
	updates map[string]any
}

func (s *codexOverdraftToggleAdminService) GetAccount(context.Context, int64) (*service.Account, error) {
	return s.account, nil
}

func (s *codexOverdraftToggleAdminService) UpdateAccountExtra(_ context.Context, _ int64, updates map[string]any) error {
	s.updates = updates
	if s.account.Extra == nil {
		s.account.Extra = make(map[string]any)
	}
	for key, value := range updates {
		s.account.Extra[key] = value
	}
	return nil
}

func codexOverdraftToggleRouter(account *service.Account) (*gin.Engine, *codexOverdraftToggleAdminService) {
	gin.SetMode(gin.TestMode)
	adminService := &codexOverdraftToggleAdminService{stubAdminService: newStubAdminService(), account: account}
	handler := NewAccountHandler(adminService, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	router := gin.New()
	router.PUT("/admin/accounts/:id/codex-quota-overdraft", handler.SetCodexQuotaOverdraft)
	return router, adminService
}

func TestSetCodexQuotaOverdraftTogglesOnlyEligibleAccount(t *testing.T) {
	account := &service.Account{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	router, adminService := codexOverdraftToggleRouter(account)

	for _, enabled := range []bool{true, false} {
		recorder := httptest.NewRecorder()
		body := []byte(`{"enabled":true}`)
		if !enabled {
			body = []byte(`{"enabled":false}`)
		}
		request := httptest.NewRequest(http.MethodPut, "/admin/accounts/42/codex-quota-overdraft", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Equal(t, enabled, adminService.updates[service.CodexQuotaOverdraftEnabledExtraKey])
		require.Equal(t, enabled, account.Extra[service.CodexQuotaOverdraftEnabledExtraKey])
	}
}

func TestSetCodexQuotaOverdraftRejectsIneligibleAccounts(t *testing.T) {
	for _, account := range []*service.Account{
		{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey},
		{ID: 42, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, ParentAccountID: func() *int64 { value := int64(1); return &value }()},
		{ID: 42, Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth},
	} {
		router, adminService := codexOverdraftToggleRouter(account)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/admin/accounts/42/codex-quota-overdraft", bytes.NewBufferString(`{"enabled":true}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Empty(t, adminService.updates)
	}
}
