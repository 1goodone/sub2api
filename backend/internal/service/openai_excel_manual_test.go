package service

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type excelManualTestRepo struct {
	*reauthTestRepo
	expected map[string]any
	saved    string
	ok       bool
	calls    int
}

func (r *excelManualTestRepo) SaveManualExcelCredentials(_ context.Context, _ int64, expected map[string]any, ciphertext string, _ ...ExcelBPSCredentialState) (bool, error) {
	r.calls++
	r.expected = cloneReauthMap(expected)
	r.saved = ciphertext
	return r.ok, nil
}

type excelManualOAuthClient struct {
	code     string
	redirect string
	proxy    string
	clientID string
	calls    int
}

func (c *excelManualOAuthClient) ExchangeCode(_ context.Context, code, _, redirect, proxy, clientID string) (*openai.TokenResponse, error) {
	c.calls++
	c.code = code
	c.redirect = redirect
	c.proxy = proxy
	c.clientID = clientID
	exp := time.Now().Add(time.Hour).Unix()
	return &openai.TokenResponse{
		AccessToken: reauthTestJWT(map[string]any{
			"exp": exp, "client_id": openai.ExcelClientID,
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": "acct-1", "chatgpt_user_id": "user-1",
			},
		}),
		RefreshToken: "excel-refresh",
		IDToken: reauthTestJWT(map[string]any{
			"email": "user@example.com", "aud": []string{openai.ExcelClientID}, "exp": exp,
		}),
		ExpiresIn: 3600,
	}, nil
}
func (*excelManualOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, nil
}
func (*excelManualOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, nil
}

func TestExcelManualAuthorizationStoresGrantWithoutReplacingCodex(t *testing.T) {
	svc, reader, repo, _, _, _ := newReauthTestService("acct-1")
	proxyID := int64(7)
	reader.account.ProxyID = &proxyID
	reader.account.Credentials["plan_type"] = "plus"
	reader.proxy = &Proxy{ID: proxyID, Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: StatusActive}
	writer := &excelManualTestRepo{reauthTestRepo: repo, ok: true}
	svc.repo = writer
	client := &excelManualOAuthClient{}
	svc.oauth.oauthClient = client

	result, err := svc.BeginExcelManualAuthorization(context.Background(), 42)
	require.NoError(t, err)
	authURL, err := url.Parse(result.AuthURL)
	require.NoError(t, err)
	require.Equal(t, "https", authURL.Scheme)
	require.Equal(t, "auth.openai.com", authURL.Host)
	require.Equal(t, "/api/accounts/authorize", authURL.Path)
	require.Equal(t, openai.ExcelClientID, authURL.Query().Get("client_id"))
	require.Equal(t, openai.ExcelRedirectURI, authURL.Query().Get("redirect_uri"))
	state := authURL.Query().Get("state")
	require.True(t, strings.HasPrefix(state, "bps.") && strings.HasSuffix(state, ".PC"))

	_, err = svc.oauth.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{
		SessionID: result.SessionID, Code: "generic-code", State: state,
	})
	require.Error(t, err)
	require.Equal(t, "OPENAI_EXCEL_BOUND_SESSION", infraerrors.Reason(err))
	require.Zero(t, client.calls)

	_, _, _, err = parseExcelManualCallback("http://localhost:1455/auth/callback?code=excel-code&state=" + url.QueryEscape(state))
	require.Error(t, err)
	require.Equal(t, "OPENAI_EXCEL_CALLBACK_INVALID", infraerrors.Reason(err))

	callback := openai.ExcelRedirectURI + "?code=excel-code&state=" + url.QueryEscape(state)
	require.NoError(t, svc.CompleteExcelManualAuthorization(context.Background(), 42, result.SessionID, callback))
	require.Equal(t, 1, client.calls)
	require.Equal(t, "excel-code", client.code)
	require.Equal(t, openai.ExcelClientID, client.clientID)
	require.Equal(t, openai.ExcelRedirectURI, client.redirect)
	require.Equal(t, "http://127.0.0.1:8080", client.proxy)
	require.Equal(t, "old-access", writer.expected["access_token"])
	require.Equal(t, "preserved", writer.expected["custom_setting"])
	require.Equal(t, "old-access", reader.account.Credentials["access_token"])
	plain, err := reauthTestEncryptor{}.Decrypt(writer.saved)
	require.NoError(t, err)
	require.Contains(t, plain, openai.ExcelClientID)
	require.Contains(t, plain, "excel-refresh")
	require.NotContains(t, plain, "old-refresh")

	err = svc.CompleteExcelManualAuthorization(context.Background(), 42, result.SessionID, callback)
	require.Equal(t, "OPENAI_EXCEL_SESSION_INVALID", infraerrors.Reason(err))
	require.Equal(t, 1, client.calls)
}

func TestExcelManualAuthorizationRejectsIneligibleAccountsAndCredentialChanges(t *testing.T) {
	svc, reader, repo, _, _, _ := newReauthTestService("acct-1")
	reader.account.Credentials["plan_type"] = "free"
	_, err := svc.BeginExcelManualAuthorization(context.Background(), 42)
	require.Equal(t, "OPENAI_EXCEL_MANUAL_INELIGIBLE", infraerrors.Reason(err))

	reader.account.Credentials["plan_type"] = "plus"
	reader.account.Credentials["client_id"] = openai.ExcelClientID
	_, err = svc.BeginExcelManualAuthorization(context.Background(), 42)
	require.Equal(t, "OPENAI_EXCEL_MANUAL_INELIGIBLE", infraerrors.Reason(err))

	delete(reader.account.Credentials, "client_id")
	writer := &excelManualTestRepo{reauthTestRepo: repo, ok: true}
	svc.repo = writer
	client := &excelManualOAuthClient{}
	svc.oauth.oauthClient = client
	result, err := svc.BeginExcelManualAuthorization(context.Background(), 42)
	require.NoError(t, err)
	reader.account.Credentials["custom_setting"] = "changed"
	state := reauthStateFromURL(t, result.AuthURL)
	err = svc.CompleteExcelManualAuthorization(context.Background(), 42, result.SessionID, openai.ExcelRedirectURI+"?code=excel-code&state="+url.QueryEscape(state))
	require.Equal(t, "OPENAI_EXCEL_ACCOUNT_CHANGED", infraerrors.Reason(err))
	require.Zero(t, client.calls)
	require.Zero(t, writer.calls)
}
