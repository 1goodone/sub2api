package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// excelManualGrantWriter stores a browser-completed BPS grant on an existing account.
// It is optional so worker-task mocks do not have to implement the manual path.
type excelManualGrantWriter interface {
	SaveManualExcelCredentials(context.Context, int64, map[string]any, string, ...ExcelBPSCredentialState) (bool, error)
}

func manualExcelEligible(account *Account) bool {
	return QualityBPSEligible(account) && !account.IsExcelOAuth() && !account.IsCredentialShadow()
}

// BeginExcelManualAuthorization starts PKCE for the account being reauthorized.
// The operator finishes Excel login in their own browser. This server only keeps
// the verifier and later exchanges the code through the account proxy.
func (s *OpenAIOAuthReauthService) BeginExcelManualAuthorization(ctx context.Context, id int64) (*OpenAIAuthURLResult, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	if s.oauth == nil || s.oauth.sessionStore == nil {
		return nil, infraerrors.ServiceUnavailable("OPENAI_EXCEL_MANUAL_UNAVAILABLE", "BPS authorization is unavailable")
	}
	account, err := s.accountFor(ctx, id)
	if err != nil {
		return nil, err
	}
	if !manualExcelEligible(account) {
		return nil, infraerrors.BadRequest("OPENAI_EXCEL_MANUAL_INELIGIBLE", "account is not eligible for manual BPS authorization")
	}
	expectedHash, err := hashReauthCredentials(account.Credentials)
	if err != nil {
		return nil, err
	}
	proxyURL, err := s.reauthProxyURL(ctx, account.ProxyID)
	if err != nil {
		return nil, infraerrors.BadRequest("OPENAI_EXCEL_PROXY_UNAVAILABLE", "account proxy is unavailable")
	}
	rawState, err := openai.GenerateState()
	if err != nil {
		return nil, err
	}
	verifier, err := openai.GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	sessionID, err := openai.GenerateSessionID()
	if err != nil {
		return nil, err
	}
	state := "bps." + rawState + ".PC"
	session := &openai.OAuthSession{
		State:             state,
		CodeVerifier:      verifier,
		ClientID:          openai.ExcelClientID,
		ProxyURL:          proxyURL,
		RedirectURI:       openai.ExcelRedirectURI,
		CreatedAt:         time.Now(),
		ExcelAccountID:    account.ID,
		ExcelExpectedHash: expectedHash,
	}
	s.oauth.sessionStore.Set(sessionID, session)
	return &OpenAIAuthURLResult{
		AuthURL:   openai.BuildExcelAuthorizationURL(state, openai.GenerateCodeChallenge(verifier)),
		SessionID: sessionID,
	}, nil
}

// CompleteExcelManualAuthorization exchanges one official bps.openai.com callback
// and upserts the Excel grant. Codex credentials and account status stay unchanged.
func (s *OpenAIOAuthReauthService) CompleteExcelManualAuthorization(ctx context.Context, id int64, sessionID, callback string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if err := s.ensureDurableEncryption(); err != nil {
		return err
	}
	writer, ok := s.repo.(excelManualGrantWriter)
	if !ok || s.oauth == nil || s.oauth.oauthClient == nil || s.oauth.sessionStore == nil {
		return infraerrors.ServiceUnavailable("OPENAI_EXCEL_MANUAL_UNAVAILABLE", "BPS authorization is unavailable")
	}
	session, ok := s.oauth.sessionStore.Get(strings.TrimSpace(sessionID))
	if !ok || session.ExcelAccountID != id || session.ClientID != openai.ExcelClientID || session.RedirectURI != openai.ExcelRedirectURI {
		return infraerrors.BadRequest("OPENAI_EXCEL_SESSION_INVALID", "BPS session expired or belongs to another account")
	}
	code, state, oauthErr, err := parseExcelManualCallback(callback)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return infraerrors.BadRequest("OPENAI_EXCEL_STATE_INVALID", "BPS callback state does not match this authorization")
	}
	consumed, ok := s.oauth.sessionStore.Take(sessionID)
	if !ok || consumed.ExcelAccountID != id || subtle.ConstantTimeCompare([]byte(consumed.State), []byte(session.State)) != 1 {
		return infraerrors.Conflict("OPENAI_EXCEL_SESSION_CONSUMED", "BPS session was already used")
	}
	if oauthErr != "" {
		return infraerrors.BadRequest("OPENAI_EXCEL_CONSENT_REQUIRED", "BPS authorization was declined or requires another attempt")
	}
	account, err := s.accountFor(ctx, id)
	if err != nil {
		return err
	}
	if !manualExcelEligible(account) {
		return infraerrors.BadRequest("OPENAI_EXCEL_MANUAL_INELIGIBLE", "account is not eligible for manual BPS authorization")
	}
	currentHash, err := hashReauthCredentials(account.Credentials)
	if err != nil || subtle.ConstantTimeCompare([]byte(currentHash), []byte(consumed.ExcelExpectedHash)) != 1 {
		return infraerrors.Conflict("OPENAI_EXCEL_ACCOUNT_CHANGED", "account credentials changed during BPS authorization; retry")
	}
	exchangeCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	token, err := s.oauth.oauthClient.ExchangeCode(exchangeCtx, code, consumed.CodeVerifier, consumed.RedirectURI, consumed.ProxyURL, openai.ExcelClientID)
	if err != nil {
		return infraerrors.BadRequest("OPENAI_EXCEL_EXCHANGE_FAILED", "BPS authorization code exchange failed")
	}
	info, err := excelManualTokenInfo(token)
	if err != nil {
		return infraerrors.BadRequest("OPENAI_EXCEL_TOKEN_INVALID", "Excel authorization did not return a usable BPS session")
	}
	if err = validateReauthToken(account, info); err != nil {
		return err
	}
	raw, err := json.Marshal(s.oauth.BuildAccountCredentials(info))
	if err != nil {
		return err
	}
	ciphertext, err := s.encryptor.Encrypt(string(raw))
	if err != nil {
		return infraerrors.ServiceUnavailable("OPENAI_EXCEL_ENCRYPT_FAILED", "failed to encrypt the BPS session")
	}
	persistCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer stop()
	saved, err := writer.SaveManualExcelCredentials(persistCtx, id, account.Credentials, ciphertext, ExcelBPSGrantState(info.ExpiresAt))
	if err != nil {
		return err
	}
	if !saved {
		return infraerrors.Conflict("OPENAI_EXCEL_ACCOUNT_CHANGED", "account credentials changed during BPS authorization; retry")
	}
	return nil
}

func excelManualTokenInfo(token *openai.TokenResponse) (*OpenAITokenInfo, error) {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.RefreshToken) == "" || strings.TrimSpace(token.IDToken) == "" {
		return nil, infraerrors.BadRequest("OPENAI_EXCEL_TOKEN_INVALID", "Excel authorization did not return a usable BPS session")
	}
	return excelOAuthTokenInfo(map[string]any{
		"access_token":  token.AccessToken,
		"refresh_token": token.RefreshToken,
		"id_token":      token.IDToken,
		"client_id":     openai.ExcelClientID,
	})
}

func parseExcelManualCallback(callback string) (code, state, oauthErr string, err error) {
	if len(callback) == 0 || len(callback) > maxOpenAIOAuthReauthURL {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "invalid BPS callback")
	}
	u, parseErr := url.Parse(strings.TrimSpace(callback))
	want, wantErr := url.Parse(openai.ExcelRedirectURI)
	if parseErr != nil || wantErr != nil || u == nil || want == nil {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "invalid BPS callback")
	}
	if u.Scheme != "https" || !strings.EqualFold(u.Host, want.Host) || u.Path != want.Path || u.RawPath != "" || u.User != nil || u.Fragment != "" {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "invalid BPS callback")
	}
	q, queryErr := url.ParseQuery(u.RawQuery)
	if queryErr != nil {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "invalid BPS callback")
	}
	for _, key := range []string{"code", "state", "error"} {
		if len(q[key]) > 1 {
			return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "duplicate BPS callback parameter")
		}
	}
	state = strings.TrimSpace(q.Get("state"))
	code = strings.TrimSpace(q.Get("code"))
	oauthErr = strings.TrimSpace(q.Get("error"))
	if state == "" || len(state) > 512 || !strings.HasPrefix(state, "bps.") || !strings.HasSuffix(state, ".PC") {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "invalid BPS callback")
	}
	if oauthErr == "" && (code == "" || len(code) > 2048) {
		return "", "", "", infraerrors.BadRequest("OPENAI_EXCEL_CALLBACK_INVALID", "BPS callback is missing code or state")
	}
	return code, state, oauthErr, nil
}
