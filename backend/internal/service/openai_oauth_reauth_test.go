package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

type reauthTestEncryptor struct{}

func (reauthTestEncryptor) Encrypt(value string) (string, error) { return "encrypted:" + value, nil }
func (reauthTestEncryptor) Decrypt(value string) (string, error) {
	plain, ok := strings.CutPrefix(value, "encrypted:")
	if !ok {
		return "", errors.New("invalid ciphertext")
	}
	return plain, nil
}

type reauthTestAccountReader struct {
	account  *Account
	proxy    *Proxy
	proxyErr error
}

func (r *reauthTestAccountReader) GetAccount(context.Context, int64) (*Account, error) {
	if r.account == nil {
		return nil, ErrAccountNotFound
	}
	copy := *r.account
	copy.Credentials = cloneReauthMap(r.account.Credentials)
	return &copy, nil
}
func (r *reauthTestAccountReader) GetProxy(context.Context, int64) (*Proxy, error) {
	return r.proxy, r.proxyErr
}

type reauthTestOAuthClient struct {
	accountID string
	userID    string
	email     string
	exchanges int
}

func (c *reauthTestOAuthClient) ExchangeCode(context.Context, string, string, string, string, string) (*openai.TokenResponse, error) {
	c.exchanges++
	return &openai.TokenResponse{
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		IDToken: reauthTestJWT(map[string]any{
			"email": c.email,
			"https://api.openai.com/auth": map[string]any{
				"chatgpt_account_id": c.accountID,
				"chatgpt_user_id":    c.userID,
			},
		}),
		ExpiresIn: 3600,
	}, nil
}
func (*reauthTestOAuthClient) RefreshToken(context.Context, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not used")
}
func (*reauthTestOAuthClient) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	return nil, errors.New("not used")
}

type reauthTestRepo struct {
	config        *OpenAIOAuthReauthStoredConfig
	task          *OpenAIOAuthReauthTaskRecord
	markSucceeded int
}

func (r *reauthTestRepo) UpsertConfig(_ context.Context, accountID int64, email, ciphertext string) error {
	r.config = &OpenAIOAuthReauthStoredConfig{AccountID: accountID, LoginEmail: email, OTPURLCiphertext: ciphertext, UpdatedAt: time.Now()}
	return nil
}
func (r *reauthTestRepo) GetConfig(context.Context, int64) (*OpenAIOAuthReauthStoredConfig, error) {
	return r.config, nil
}
func (r *reauthTestRepo) CreateTask(_ context.Context, accountID int64, hash string) (*OpenAIOAuthReauthTaskRecord, error) {
	r.task = &OpenAIOAuthReauthTaskRecord{ID: 71, AccountID: accountID, Status: OpenAIOAuthReauthStatusQueued, Stage: OpenAIOAuthReauthStageQueued, ExpectedCredentialsHash: hash, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	return r.task, nil
}
func (r *reauthTestRepo) GetLatestTask(context.Context, int64) (*OpenAIOAuthReauthTaskRecord, error) {
	return r.task, nil
}
func (r *reauthTestRepo) GetTask(context.Context, int64) (*OpenAIOAuthReauthTaskRecord, error) {
	return r.task, nil
}
func (r *reauthTestRepo) ClaimNextTask(_ context.Context, workerID string, _ time.Duration) (*OpenAIOAuthReauthTaskRecord, error) {
	if r.task == nil || r.task.Status != OpenAIOAuthReauthStatusQueued {
		return nil, nil
	}
	r.task.Status = OpenAIOAuthReauthStatusRunning
	r.task.Stage = OpenAIOAuthReauthStageStarting
	r.task.WorkerID = workerID
	r.task.Attempt++
	return r.task, nil
}
func (r *reauthTestRepo) SetSession(_ context.Context, _ int64, workerID, sessionID string) error {
	if r.task == nil || r.task.WorkerID != workerID {
		return errors.New("worker mismatch")
	}
	r.task.AuthSessionID = sessionID
	return nil
}
func (r *reauthTestRepo) UpdateStage(_ context.Context, _ int64, workerID, stage string) error {
	if r.task == nil || r.task.WorkerID != workerID {
		return errors.New("worker mismatch")
	}
	r.task.Stage = stage
	return nil
}
func (r *reauthTestRepo) BeginCallback(_ context.Context, _ int64, workerID string) (*OpenAIOAuthReauthTaskRecord, bool, error) {
	if r.task == nil || r.task.WorkerID != workerID || r.task.Status != OpenAIOAuthReauthStatusRunning || r.task.AuthSessionID == "" {
		return nil, false, errors.New("task not active")
	}
	r.task.Status = OpenAIOAuthReauthStatusCallbackProcessing
	return r.task, true, nil
}
func (r *reauthTestRepo) MarkSucceeded(_ context.Context, _ int64, workerID string) error {
	r.markSucceeded++
	if r.task == nil || r.task.WorkerID != workerID || r.task.Status != OpenAIOAuthReauthStatusCallbackProcessing {
		return errors.New("task not active")
	}
	r.task.Status = OpenAIOAuthReauthStatusSucceeded
	r.task.Stage = OpenAIOAuthReauthStageSucceeded
	return nil
}
func (r *reauthTestRepo) MarkFailed(_ context.Context, _ int64, workerID, reason string) error {
	if r.task == nil || r.task.WorkerID != workerID {
		return errors.New("worker mismatch")
	}
	r.task.Status = OpenAIOAuthReauthStatusFailed
	r.task.Stage = OpenAIOAuthReauthStageFailed
	r.task.Error = reason
	return nil
}

type reauthTestUpdater struct {
	repo        *reauthTestRepo
	applied     bool
	credentials map[string]any
}

type reauthTestRuntimeBlocker struct {
	clearedAccountID int64
}

func (*reauthTestRuntimeBlocker) BlockAccountScheduling(*Account, time.Time, string) {}
func (b *reauthTestRuntimeBlocker) ClearAccountSchedulingBlock(accountID int64) {
	b.clearedAccountID = accountID
}

func (u *reauthTestUpdater) ApplyOpenAIOAuthReauth(_ context.Context, taskID int64, workerID string, accountID int64, _ map[string]any, credentials, _ map[string]any) (bool, error) {
	if u.repo.task == nil || u.repo.task.ID != taskID || u.repo.task.AccountID != accountID || u.repo.task.WorkerID != workerID || u.repo.task.Status != OpenAIOAuthReauthStatusCallbackProcessing {
		return false, nil
	}
	u.applied = true
	u.credentials = cloneReauthMap(credentials)
	u.repo.task.Status = OpenAIOAuthReauthStatusSucceeded
	u.repo.task.Stage = OpenAIOAuthReauthStageSucceeded
	return true, nil
}

func TestOpenAIOAuthReauthProtocolFlowAppliesTokensAndFinalizesAtomically(t *testing.T) {
	svc, reader, repo, updater, oauthClient, runtimeBlocker := newReauthTestService("acct-1")
	ctx := context.Background()

	config, err := svc.SaveConfig(ctx, 42, "User@Example.com", "https://mail.example.com/path-secret/code?key=query-secret&keyword=ChatGPT")
	require.NoError(t, err)
	require.Equal(t, "user@example.com", config.LoginEmail)
	require.Equal(t, "https://mail.example.com", config.URLMasked)

	_, err = svc.CreateTask(ctx, 42)
	require.NoError(t, err)
	claim, err := svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)
	require.NotNil(t, claim)
	state := reauthStateFromURL(t, claim.AuthURL)

	task, err := svc.SubmitCallback(ctx, claim.TaskID, "worker-a", "http://localhost:1455/auth/callback?code=test-code&state="+url.QueryEscape(state))
	require.NoError(t, err)
	require.Equal(t, OpenAIOAuthReauthStatusSucceeded, task.Status)
	require.True(t, updater.applied)
	require.Equal(t, "new-access", updater.credentials["access_token"])
	require.Equal(t, "new-refresh", updater.credentials["refresh_token"])
	require.Equal(t, "preserved", updater.credentials["custom_setting"])
	require.NotZero(t, updater.credentials["_token_version"])
	require.Equal(t, 0, repo.markSucceeded, "the atomic updater already finalized the task")
	require.Equal(t, 1, oauthClient.exchanges)
	require.Equal(t, int64(42), runtimeBlocker.clearedAccountID)
	require.Equal(t, "old-access", reader.account.Credentials["access_token"], "the test reader retains the pre-login snapshot")
}

func TestOpenAIOAuthReauthRejectsConcurrentCredentialChange(t *testing.T) {
	svc, reader, repo, updater, oauthClient, runtimeBlocker := newReauthTestService("acct-1")
	ctx := context.Background()
	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err := svc.CreateTask(ctx, 42)
	require.NoError(t, err)
	claim, err := svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)
	reader.account.Credentials["access_token"] = "concurrent-update"

	_, err = svc.SubmitCallback(ctx, claim.TaskID, "worker-a", "http://localhost:1455/auth/callback?code=test&state="+url.QueryEscape(reauthStateFromURL(t, claim.AuthURL)))
	require.Error(t, err)
	require.Equal(t, OpenAIOAuthReauthStatusFailed, repo.task.Status)
	require.False(t, updater.applied)
	require.Equal(t, 0, oauthClient.exchanges)
	require.Zero(t, runtimeBlocker.clearedAccountID)
	require.Equal(t, "concurrent-update", reader.account.Credentials["access_token"])
}

func TestOpenAIOAuthReauthRejectsDifferentOpenAIIdentity(t *testing.T) {
	svc, _, repo, updater, oauthClient, runtimeBlocker := newReauthTestService("different-account")
	ctx := context.Background()
	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err := svc.CreateTask(ctx, 42)
	require.NoError(t, err)
	claim, err := svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)

	_, err = svc.SubmitCallback(ctx, claim.TaskID, "worker-a", "http://localhost:1455/auth/callback?code=test&state="+url.QueryEscape(reauthStateFromURL(t, claim.AuthURL)))
	require.Error(t, err)
	require.Equal(t, 409, infraerrors.Code(err))
	require.Equal(t, OpenAIOAuthReauthStatusFailed, repo.task.Status)
	require.False(t, updater.applied)
	require.Equal(t, 1, oauthClient.exchanges)
	require.Zero(t, runtimeBlocker.clearedAccountID)
}

func TestOpenAIOAuthReauthRejectsDifferentUserInSameWorkspace(t *testing.T) {
	svc, _, repo, updater, oauthClient, runtimeBlocker := newReauthTestService("acct-1")
	oauthClient.userID = "user-2"
	ctx := context.Background()
	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err := svc.CreateTask(ctx, 42)
	require.NoError(t, err)
	claim, err := svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)

	_, err = svc.SubmitCallback(ctx, claim.TaskID, "worker-a", "http://localhost:1455/auth/callback?code=test&state="+url.QueryEscape(reauthStateFromURL(t, claim.AuthURL)))
	require.Error(t, err)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
	require.Equal(t, OpenAIOAuthReauthStatusFailed, repo.task.Status)
	require.False(t, updater.applied)
	require.Equal(t, 1, oauthClient.exchanges)
	require.Zero(t, runtimeBlocker.clearedAccountID)
}

func TestOpenAIOAuthReauthDoesNotBypassMissingConfiguredProxy(t *testing.T) {
	svc, reader, repo, _, _, _ := newReauthTestService("acct-1")
	ctx := context.Background()
	proxyID := int64(9)
	reader.account.ProxyID = &proxyID
	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err := svc.CreateTask(ctx, 42)
	require.NoError(t, err)

	claim, err := svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)
	require.Nil(t, claim)
	require.Equal(t, OpenAIOAuthReauthStatusFailed, repo.task.Status)
	require.Equal(t, "configured account proxy is unavailable", repo.task.Error)
}

func TestOpenAIOAuthReauthWorkerCannotSetBackendOwnedStage(t *testing.T) {
	svc, _, repo, _, _, _ := newReauthTestService("acct-1")
	ctx := context.Background()
	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err := svc.CreateTask(ctx, 42)
	require.NoError(t, err)
	_, err = svc.ClaimTask(ctx, "worker-a")
	require.NoError(t, err)

	err = svc.UpdateStage(ctx, repo.task.ID, "worker-a", OpenAIOAuthReauthStageSucceeded)
	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Equal(t, OpenAIOAuthReauthStageStarting, repo.task.Stage)
}

func TestOpenAIOAuthReauthRequiresFixedEncryptionKey(t *testing.T) {
	svc, _, repo, _, _, _ := newReauthTestService("acct-1")
	svc.encryptionKeyConfigured = false
	ctx := context.Background()

	_, err := svc.SaveConfig(ctx, 42, "user@example.com", "https://mail.example.com/code")
	require.Error(t, err)
	require.Equal(t, "OPENAI_REAUTH_ENCRYPTION_KEY_REQUIRED", infraerrors.Reason(err))

	require.NoError(t, repo.UpsertConfig(ctx, 42, "user@example.com", "encrypted:https://mail.example.com/code"))
	_, err = svc.CreateTask(ctx, 42)
	require.Error(t, err)
	require.Equal(t, "OPENAI_REAUTH_ENCRYPTION_KEY_REQUIRED", infraerrors.Reason(err))
}

func TestOpenAIOAuthReauthRejectsOAuthModesWithoutRefreshTokens(t *testing.T) {
	for _, tt := range []struct {
		name     string
		authMode string
	}{
		{name: "personal access token", authMode: OpenAIAuthModePersonalAccessToken},
		{name: "agent identity", authMode: OpenAIAuthModeAgentIdentity},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, reader, repo, _, _, _ := newReauthTestService("acct-1")
			reader.account.Credentials[openAIAuthModeCredentialKey] = tt.authMode

			_, err := svc.SaveConfig(context.Background(), 42, "user@example.com", "https://mail.example.com/code")
			require.Error(t, err)
			require.Equal(t, "OPENAI_REAUTH_ACCOUNT_INVALID", infraerrors.Reason(err))
			require.Nil(t, repo.config)

			_, err = svc.CreateTask(context.Background(), 42)
			require.Error(t, err)
			require.Equal(t, "OPENAI_REAUTH_ACCOUNT_INVALID", infraerrors.Reason(err))
			require.Nil(t, repo.task)
		})
	}
}

func TestParseReauthCallbackRequiresExactCodexRedirect(t *testing.T) {
	code, state, err := parseReauthCallback("http://localhost:1455/auth/callback?code=test-code&state=test-state")
	require.NoError(t, err)
	require.Equal(t, "test-code", code)
	require.Equal(t, "test-state", state)

	for _, tt := range []struct {
		name     string
		callback string
	}{
		{name: "https", callback: "https://localhost:1455/auth/callback?code=test&state=test"},
		{name: "IPv4 loopback", callback: "http://127.0.0.1:1455/auth/callback?code=test&state=test"},
		{name: "IPv6 loopback", callback: "http://[::1]:1455/auth/callback?code=test&state=test"},
		{name: "wrong port", callback: "http://localhost:1456/auth/callback?code=test&state=test"},
		{name: "wrong path", callback: "http://localhost:1455/other?code=test&state=test"},
		{name: "escaped path", callback: "http://localhost:1455/auth%2Fcallback?code=test&state=test"},
		{name: "fragment", callback: "http://localhost:1455/auth/callback?code=test&state=test#fragment"},
		{name: "userinfo", callback: "http://user@localhost:1455/auth/callback?code=test&state=test"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseReauthCallback(tt.callback)
			require.Error(t, err)
			require.Equal(t, "OPENAI_REAUTH_CALLBACK_INVALID", infraerrors.Reason(err))
		})
	}
}

func newReauthTestService(tokenAccountID string) (*OpenAIOAuthReauthService, *reauthTestAccountReader, *reauthTestRepo, *reauthTestUpdater, *reauthTestOAuthClient, *reauthTestRuntimeBlocker) {
	reader := &reauthTestAccountReader{account: &Account{
		ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "old-access", "refresh_token": "old-refresh",
			"chatgpt_account_id": "acct-1", "email": "user@example.com",
			"chatgpt_user_id": "user-1",
			"custom_setting":  "preserved",
		},
	}}
	repo := &reauthTestRepo{}
	updater := &reauthTestUpdater{repo: repo}
	oauthClient := &reauthTestOAuthClient{accountID: tokenAccountID, userID: "user-1", email: "user@example.com"}
	runtimeBlocker := &reauthTestRuntimeBlocker{}
	oauthService := NewOpenAIOAuthService(nil, oauthClient)
	return NewOpenAIOAuthReauthService(repo, reader, updater, oauthService, reauthTestEncryptor{}, true, nil, runtimeBlocker), reader, repo, updater, oauthClient, runtimeBlocker
}

func reauthStateFromURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	state := u.Query().Get("state")
	require.NotEmpty(t, state)
	return state
}

func reauthTestJWT(claims map[string]any) string {
	payload, _ := json.Marshal(claims)
	return base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func cloneReauthMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
