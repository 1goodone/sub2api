package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type siwcTestRepo struct {
	*reauthTestRepo
	host, ciphertext string
	writes           int
}

func (r *siwcTestRepo) SIWCRegistration(_ context.Context, _ int64, host string) (string, string, error) {
	if r.host == "" {
		r.host = host
	}
	return r.host, r.ciphertext, nil
}
func (r *siwcTestRepo) GetSIWCCredentials(context.Context, int64) (string, error) {
	return r.ciphertext, nil
}
func (r *siwcTestRepo) CreateSIWCTask(ctx context.Context, id int64, hash string) (*OpenAIOAuthReauthTaskRecord, error) {
	task, err := r.CreateTask(ctx, id, hash)
	if task != nil {
		task.OAuthProfile = "siwc"
	}
	return task, err
}
func (r *siwcTestRepo) SaveSIWCCredentials(_ context.Context, _ int64, _ map[string]any, expected, replacement string, _ bool, _ *OpenAIOAuthReauthTaskRecord) (bool, error) {
	if r.ciphertext != expected {
		return false, nil
	}
	r.writes++
	r.ciphertext = replacement
	return true, nil
}

type siwcTestClient struct {
	*reauthTestOAuthClient
	calls               int
	issued              string
	nonce               string
	accessToken         string
	idToken             string
	email               string
	subject             string
	omitRefreshIdentity bool
}

func (c *siwcTestClient) SIWCToken(_ context.Context, form url.Values, _, nonce string) (*openai.TokenResponse, *openai.SIWCIdentity, error) {
	c.calls++
	c.issued = form.Get("client_id")
	c.nonce = nonce
	access := c.accessToken
	if access == "" {
		access = reauthTestJWT(map[string]any{"exp": time.Now().Add(time.Hour).Unix(), "https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-1", "chatgpt_user_id": "user-1"}})
	}
	idToken := c.idToken
	if idToken == "" {
		idToken = "verified-identity"
	}
	if form.Get("grant_type") == "refresh_token" && c.omitRefreshIdentity {
		return &openai.TokenResponse{AccessToken: access, RefreshToken: "siwc-refresh", Scope: openai.SIWCDirectScope, ExpiresIn: 3600}, nil, nil
	}
	email := c.email
	if email == "" {
		email = "user@example.com"
	}
	token := &openai.TokenResponse{AccessToken: access, RefreshToken: "siwc-refresh", IDToken: idToken, Scope: openai.SIWCDirectScope, ExpiresIn: 3600}
	identity := &openai.SIWCIdentity{Email: email}
	identity.Subject = "subscriber"
	if c.email != "" {
		identity.Email = c.email
	}
	if c.subject != "" {
		identity.Subject = c.subject
	}
	return token, identity, nil
}
func TestSIWCBindingPreservesCodexAndReusesIssuedRegistration(t *testing.T) {
	svc, reader, repo, _, oldClient, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	store := &siwcTestRepo{reauthTestRepo: repo}
	svc.repo = store
	client := &siwcTestClient{reauthTestOAuthClient: oldClient}
	svc.oauth.oauthClient = client
	before := cloneReauthMap(reader.account.Credentials)
	auth, err := svc.BeginSIWCAuthorization(context.Background(), 42)
	require.NoError(t, err)
	u, err := url.Parse(auth.AuthURL)
	require.NoError(t, err)
	wrong := "http://127.0.0.1:1455/auth/callback?code=code&state=wrong&client_id=issued"
	require.Error(t, svc.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, wrong, nil))
	require.Zero(t, client.calls)
	callback := "http://127.0.0.1:1455/auth/callback?code=code&state=" + url.QueryEscape(u.Query().Get("state")) + "&client_id=issued"
	pasted := "无法访问此网站\n" + strings.Replace(callback, "127.0.0.1", "localhost", 1) + "\nERR_CONNECTION_REFUSED"
	require.NoError(t, svc.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, pasted, nil))
	require.Equal(t, "issued", client.issued)
	require.Equal(t, u.Query().Get("nonce"), client.nonce)
	require.Equal(t, before, reader.account.Credentials)
	require.Equal(t, 1, store.writes)
	require.Error(t, svc.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, callback, nil))
	require.Equal(t, 1, client.calls)
	again, err := svc.BeginSIWCAuthorization(context.Background(), 42)
	require.NoError(t, err)
	next, err := url.Parse(again.AuthURL)
	require.NoError(t, err)
	require.Equal(t, "issued", next.Query().Get("client_id"))
	require.Equal(t, u.Query().Get("ext_agent_host_id"), next.Query().Get("ext_agent_host_id"))
	require.Empty(t, next.Query().Get("agent_name_hint"))
}
func TestSIWCOpaqueTokenUsesMatchingAccountWorkspace(t *testing.T) {
	svc, reader, repo, _, oldClient, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	store := &siwcTestRepo{reauthTestRepo: repo}
	svc.repo = store
	client := &siwcTestClient{reauthTestOAuthClient: oldClient, accessToken: "opaque-siwc-access", idToken: "opaque-id-token"}
	svc.oauth.oauthClient = client
	auth, err := svc.BeginSIWCAuthorization(context.Background(), 42)
	require.NoError(t, err)
	state := reauthStateFromURL(t, auth.AuthURL)
	callback := "http://127.0.0.1:1455/auth/callback?code=code&state=" + url.QueryEscape(state) + "&client_id=issued"
	require.NoError(t, svc.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, callback, nil))
	plain, err := svc.encryptor.Decrypt(store.ciphertext)
	require.NoError(t, err)
	require.Equal(t, "acct-1", gjson.Get(plain, "workspace").String())
	require.Equal(t, "opaque-siwc-access", gjson.Get(plain, "access_token").String())
	require.Equal(t, reader.account.Credentials["email"], gjson.Get(plain, "email").String())

	mismatch, _, repo2, _, oldClient2, _ := newReauthTestService("acct-1")
	defer mismatch.oauth.Stop()
	store2 := &siwcTestRepo{reauthTestRepo: repo2}
	mismatch.repo = store2
	mismatch.oauth.oauthClient = &siwcTestClient{reauthTestOAuthClient: oldClient2, accessToken: "opaque-siwc-access", email: "other@example.com"}
	auth, err = mismatch.BeginSIWCAuthorization(context.Background(), 42)
	require.NoError(t, err)
	state = reauthStateFromURL(t, auth.AuthURL)
	callback = "http://127.0.0.1:1455/auth/callback?code=code&state=" + url.QueryEscape(state) + "&client_id=issued"
	require.Error(t, mismatch.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, callback, nil))
	require.Zero(t, store2.writes)
}

func TestSIWCBindingRejectsDifferentEmailAndReturningSubscriber(t *testing.T) {
	for _, returning := range []bool{false, true} {
		t.Run(map[bool]string{false: "different_email", true: "different_returning_subscriber"}[returning], func(t *testing.T) {
			svc, _, repo, _, oldClient, _ := newReauthTestService("acct-1")
			defer svc.oauth.Stop()
			store := &siwcTestRepo{reauthTestRepo: repo}
			svc.repo = store
			client := &siwcTestClient{reauthTestOAuthClient: oldClient, email: "other@example.com"}
			if returning {
				client.email = "user@example.com"
				previous, err := json.Marshal(siwcGrant{ClientID: "issued", Subject: "original-subscriber"})
				require.NoError(t, err)
				store.ciphertext = "encrypted:" + string(previous)
			}
			svc.oauth.oauthClient = client
			before := store.ciphertext
			auth, err := svc.BeginSIWCAuthorization(context.Background(), 42)
			require.NoError(t, err)
			u, err := url.Parse(auth.AuthURL)
			require.NoError(t, err)
			callback := "http://127.0.0.1:1455/auth/callback?code=code&state=" + url.QueryEscape(u.Query().Get("state")) + "&client_id=issued"
			require.Error(t, svc.CompleteSIWCAuthorization(context.Background(), 42, auth.SessionID, callback, nil))
			require.Zero(t, store.writes)
			require.Equal(t, before, store.ciphertext)
		})
	}
}

func TestSIWCBoundSessionCannotUseGenericExchange(t *testing.T) {
	svc, _, repo, _, _, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	svc.repo = &siwcTestRepo{reauthTestRepo: repo}
	auth, err := svc.BeginSIWCAuthorization(context.Background(), 42)
	require.NoError(t, err)
	u, err := url.Parse(auth.AuthURL)
	require.NoError(t, err)
	_, err = svc.oauth.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{SessionID: auth.SessionID, Code: "code", State: u.Query().Get("state")})
	require.Error(t, err)
}
func TestSIWCRequestUsesOfficialEndpointWithoutCodexSecrets(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	ctx.Request.Header.Set("Cookie", "must-not-forward")
	ctx.Request.Header.Set("X-Codex-Turn-State", "must-not-forward")
	service := &OpenAIGatewayService{}
	req, err := service.buildSIWCRequest(context.Background(), ctx, []byte(`{"model":"gpt-6.1-sol","stream":false,"store":true,"previous_response_id":"old","input":[{"role":"system","content":"rules"}],"tools":[{"type":"function","name":"local","parameters":{"type":"object"}}]}`), "siwc-access")
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses", req.URL.String())
	require.Equal(t, "Bearer siwc-access", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("Cookie"))
	require.Empty(t, req.Header.Get("X-Codex-Turn-State"))
	var payload map[string]any
	require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
	require.Equal(t, true, payload["stream"])
	require.Equal(t, false, payload["store"])
	require.NotContains(t, payload, "previous_response_id")
	normalized, err := normalizeSIWCBody([]byte(`{"input":[{"role":"system","content":"rules"}]}`))
	require.NoError(t, err)
	require.Equal(t, "developer", gjson.GetBytes(normalized, "input.0.role").String())
	_, err = normalizeSIWCBody([]byte(`{"tools":[{"type":"tool_search"}]}`))
	require.Error(t, err)
	ctx.Request = httptest.NewRequest("POST", "/v1/responses/compact", nil)
	_, err = service.buildSIWCRequest(context.Background(), ctx, []byte(`{}`), "siwc-access")
	require.Error(t, err)
}
func TestSIWCSelectionDisablesOtherProtocolCapabilities(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{OpenAISIWCEnabledKey: true, "openai_excel_bps": true, "openai_oauth_responses_websockets_v2_enabled": true}}
	require.True(t, account.IsSIWCEnabled())
	require.False(t, account.UsesOpenAICodexProtocol())
	require.False(t, account.IsExcelBPSEnabled())
	require.False(t, account.IsOpenAIResponsesWebSocketV2Enabled())
	require.False(t, isOpenAICodexTicketAccount(account))
	require.True(t, shouldEstimateOpenAIInputTokensLocally(account))
	active := openAITurnRouteFingerprint(account)
	account.Extra[OpenAISIWCEnabledKey] = false
	require.NotEqual(t, active, openAITurnRouteFingerprint(account))
	require.True(t, account.UsesOpenAICodexProtocol())
}

func TestSIWCRefreshAnd401MarkerLeaveCodexGrantUnchanged(t *testing.T) {
	svc, reader, repo, _, oldClient, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	store := &siwcTestRepo{reauthTestRepo: repo}
	svc.repo = store
	client := &siwcTestClient{reauthTestOAuthClient: oldClient}
	svc.oauth.oauthClient = client
	original := cloneReauthMap(reader.account.Credentials)
	expired := siwcGrant{AccessToken: "expired-access", RefreshToken: "rotating-refresh", ClientID: "issued", Subject: "subscriber", Email: "user@example.com", Workspace: "acct-1", Scope: openai.SIWCDirectScope, ExpiresAt: time.Now().Add(-time.Minute).Unix()}
	raw, err := json.Marshal(expired)
	require.NoError(t, err)
	store.ciphertext = "encrypted:" + string(raw)
	token, err := svc.SIWCAccessToken(context.Background(), reader.account, nil)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Equal(t, 1, client.calls)
	require.Equal(t, original, reader.account.Credentials)
	svc.rejectSIWCAccessToken(context.Background(), reader.account, "stale-request-token", nil)
	_, saved, err := svc.readSIWCGrant(context.Background(), 42)
	require.NoError(t, err)
	require.Positive(t, saved.ExpiresAt)
	svc.rejectSIWCAccessToken(context.Background(), reader.account, token, nil)
	_, saved, err = svc.readSIWCGrant(context.Background(), 42)
	require.NoError(t, err)
	require.Zero(t, saved.ExpiresAt)
	require.Equal(t, "siwc-refresh", saved.RefreshToken)
	require.Equal(t, original, reader.account.Credentials)
}

func TestSIWCRefreshOpaqueMetadataAndOptionalIdentity(t *testing.T) {
	for _, scenario := range []string{"no_id_token", "changed_subscriber", "changed_email"} {
		t.Run(scenario, func(t *testing.T) {
			svc, reader, repo, _, oldClient, _ := newReauthTestService("acct-1")
			defer svc.oauth.Stop()
			store := &siwcTestRepo{reauthTestRepo: repo}
			svc.repo = store
			client := &siwcTestClient{reauthTestOAuthClient: oldClient, omitRefreshIdentity: scenario == "no_id_token"}
			if scenario == "changed_subscriber" {
				client.subject = "other-subscriber"
			}
			if scenario == "changed_email" {
				client.email = "other@example.com"
			}
			svc.oauth.oauthClient = client
			raw, err := json.Marshal(siwcGrant{AccessToken: "expired", RefreshToken: "old-refresh", IDToken: "retained-id", ClientID: "issued", Subject: "subscriber", Email: "user@example.com", Workspace: "acct-1", Scope: openai.SIWCDirectScope})
			require.NoError(t, err)
			store.ciphertext = "encrypted:" + string(raw)
			before := store.ciphertext
			token, err := svc.SIWCAccessToken(context.Background(), reader.account, nil)
			if scenario != "no_id_token" {
				require.Error(t, err)
				require.Empty(t, token)
				require.Equal(t, before, store.ciphertext)
				require.Zero(t, store.writes)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, token)
			_, saved, err := svc.readSIWCGrant(context.Background(), 42)
			require.NoError(t, err)
			require.Equal(t, "siwc-refresh", saved.RefreshToken)
			require.Equal(t, "retained-id", saved.IDToken)
			require.Equal(t, "subscriber", saved.Subject)
			require.Positive(t, saved.ExpiresAt)
		})
	}
}

func TestSIWCAllHTTPEntryPointsReachDirectRouteWithIndependentToken(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/v1/chat/completions", "/v1/messages"} {
		t.Run(path, func(t *testing.T) {
			reauth, reader, repo, _, _, _ := newReauthTestService("acct-1")
			defer reauth.oauth.Stop()
			store := &siwcTestRepo{reauthTestRepo: repo}
			reauth.repo = store
			grant := siwcGrant{AccessToken: "independent-siwc", RefreshToken: "siwc-refresh", ClientID: "issued", Subject: "subscriber", Workspace: "acct-1", Email: "user@example.com", Scope: openai.SIWCDirectScope, ExpiresAt: time.Now().Add(time.Hour).Unix()}
			raw, err := json.Marshal(grant)
			require.NoError(t, err)
			store.ciphertext = "encrypted:" + string(raw)
			reader.account.Extra = map[string]any{OpenAISIWCEnabledKey: true}
			upstream := &httpUpstreamRecorder{err: errors.New("synthetic transport failure")}
			gateway := openAIClientToolsTestService(upstream)
			gateway.excelOAuthReauth = reauth
			body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
			if path != "/v1/responses" {
				body = []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"max_tokens":64,"stream":false}`)
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", path, bytes.NewReader(body))
			c.Request.Header.Set("Cookie", "private-cookie")
			c.Request.Header.Set("X-Codex-Turn-State", "private-state")
			switch path {
			case "/v1/responses":
				_, err = gateway.Forward(context.Background(), c, reader.account, body)
			case "/v1/chat/completions":
				_, err = gateway.ForwardAsChatCompletions(context.Background(), c, reader.account, body, "", "")
			case "/v1/messages":
				_, err = gateway.ForwardAsAnthropic(context.Background(), c, reader.account, body, "", "")
			}
			require.Error(t, err)
			require.Len(t, upstream.requests, 1)
			require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
			require.Equal(t, "Bearer independent-siwc", upstream.lastReq.Header.Get("Authorization"))
			require.Empty(t, upstream.lastReq.Header.Get("Cookie"))
			require.Empty(t, upstream.lastReq.Header.Get("X-Codex-Turn-State"))
			require.Empty(t, upstream.lastReq.Header.Get("ChatGPT-Account-ID"))
			require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
			require.False(t, gjson.GetBytes(upstream.lastBody, "store").Bool())
			require.Equal(t, "old-access", reader.account.Credentials["access_token"])
		})
	}
}

type siwcListRepo struct {
	*siwcTestRepo
	rows []SIWCAccountSnapshot
	ids  []int64
}

func (r *siwcListRepo) ListSIWCStatuses(_ context.Context, ids []int64) ([]SIWCAccountSnapshot, error) {
	r.ids = append([]int64(nil), ids...)
	return r.rows, nil
}

type siwcExtraWriter struct {
	*reauthTestAccountReader
	updates map[string]any
}

func (w *siwcExtraWriter) UpdateAccountExtra(_ context.Context, id int64, updates map[string]any) error {
	if w.account == nil || id != w.account.ID {
		return errors.New("wrong account")
	}
	w.updates = updates
	return nil
}

func TestListSIWCStatusesKeepsStoredGrantRefreshable(t *testing.T) {
	svc, _, repo, _, _, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	store := &siwcListRepo{siwcTestRepo: &siwcTestRepo{reauthTestRepo: repo}, rows: []SIWCAccountSnapshot{{
		AccountID: 42, Bound: true, Enabled: true,
		Task: &OpenAIOAuthReauthTaskRecord{OAuthProfile: "siwc", ID: 7, AccountID: 42, Status: "succeeded", Stage: "succeeded"},
	}}}
	svc.repo = store
	items, err := svc.ListSIWCStatuses(context.Background(), []int64{42, 42, 0, -1})
	require.NoError(t, err)
	require.Equal(t, []int64{42}, store.ids)
	require.Len(t, items, 1)
	require.True(t, items[0].Bound)
	require.True(t, items[0].Refreshable)
	require.Equal(t, "succeeded", items[0].Task.Status)
	_, err = svc.ListSIWCStatuses(context.Background(), make([]int64, 201))
	require.Error(t, err)
}

func TestSetSIWCEnabledMergesExtraKey(t *testing.T) {
	svc, reader, _, _, _, _ := newReauthTestService("acct-1")
	defer svc.oauth.Stop()
	writer := &siwcExtraWriter{reauthTestAccountReader: reader}
	svc.accounts = writer
	require.NoError(t, svc.SetSIWCEnabled(context.Background(), 42, true))
	require.Equal(t, map[string]any{OpenAISIWCEnabledKey: true}, writer.updates)
}
