package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	OpenAIOAuthReauthStatusQueued             = "queued"
	OpenAIOAuthReauthStatusRunning            = "running"
	OpenAIOAuthReauthStatusCallbackProcessing = "callback_processing"
	OpenAIOAuthReauthStatusSucceeded          = "succeeded"
	OpenAIOAuthReauthStatusFailed             = "failed"

	OpenAIOAuthReauthStageQueued              = "queued"
	OpenAIOAuthReauthStageStarting            = "starting"
	OpenAIOAuthReauthStageProtocolConnecting  = "protocol_connecting"
	OpenAIOAuthReauthStageEmailSubmitted      = "email_submitted"
	OpenAIOAuthReauthStageWaitingOTP          = "waiting_otp"
	OpenAIOAuthReauthStageOTPSubmitted        = "otp_submitted"
	OpenAIOAuthReauthStageWaitingCallback     = "waiting_callback"
	OpenAIOAuthReauthStageExchangingToken     = "exchanging_token"
	OpenAIOAuthReauthStageApplyingCredentials = "applying_credentials"
	OpenAIOAuthReauthStageSucceeded           = "succeeded"
	OpenAIOAuthReauthStageFailed              = "failed"
)

const (
	openAIOAuthReauthStaleAfter = 30 * time.Minute
	maxOpenAIOAuthReauthURL     = 4096
	maxOpenAIOAuthReauthError   = 500
)

// OpenAIOAuthReauthConfig is the non-secret representation of a saved OTP
// mailbox configuration. The actual URL is encrypted at rest and is never
// returned by the admin API.
type OpenAIOAuthReauthConfig struct {
	AccountID  int64     `json:"account_id"`
	LoginEmail string    `json:"login_email"`
	URLMasked  string    `json:"otp_url_masked,omitempty"`
	Configured bool      `json:"configured"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
}

// OpenAIOAuthReauthTask is the safe task status exposed to administrators.
type OpenAIOAuthReauthTask struct {
	ID         int64      `json:"id"`
	AccountID  int64      `json:"account_id"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Error      string     `json:"error,omitempty"`
	Attempt    int        `json:"attempt"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// OpenAIOAuthReauthStatus combines saved configuration and the latest task.
type OpenAIOAuthReauthStatus struct {
	Config *OpenAIOAuthReauthConfig `json:"config,omitempty"`
	Task   *OpenAIOAuthReauthTask   `json:"task,omitempty"`
}

// OpenAIOAuthReauthRepository is deliberately small so the task state can be
// tested without a live database. The production implementation uses raw SQL
// because the task table is intentionally outside the generated Ent schema.
type OpenAIOAuthReauthRepository interface {
	UpsertConfig(ctx context.Context, accountID int64, loginEmail, otpURLCiphertext string) error
	GetConfig(ctx context.Context, accountID int64) (*OpenAIOAuthReauthStoredConfig, error)
	CreateTask(ctx context.Context, accountID int64, expectedCredentialsHash string) (*OpenAIOAuthReauthTaskRecord, error)
	GetLatestTask(ctx context.Context, accountID int64) (*OpenAIOAuthReauthTaskRecord, error)
	GetTask(ctx context.Context, taskID int64) (*OpenAIOAuthReauthTaskRecord, error)
	ClaimNextTask(ctx context.Context, workerID string, staleAfter time.Duration) (*OpenAIOAuthReauthTaskRecord, error)
	SetSession(ctx context.Context, taskID int64, workerID, sessionID string) error
	UpdateStage(ctx context.Context, taskID int64, workerID, stage string) error
	BeginCallback(ctx context.Context, taskID int64, workerID string) (*OpenAIOAuthReauthTaskRecord, bool, error)
	MarkSucceeded(ctx context.Context, taskID int64, workerID string) error
	MarkFailed(ctx context.Context, taskID int64, workerID, reason string) error
}

type OpenAIOAuthReauthStoredConfig struct {
	AccountID        int64
	LoginEmail       string
	OTPURLCiphertext string
	UpdatedAt        time.Time
}

// OpenAIOAuthReauthTaskRecord contains fields needed by the worker protocol.
// It is never serialized directly to an administrator response.
type OpenAIOAuthReauthTaskRecord struct {
	ID                      int64
	AccountID               int64
	Status                  string
	Stage                   string
	WorkerID                string
	AuthSessionID           string
	ExpectedCredentialsHash string
	Error                   string
	Attempt                 int
	CreatedAt               time.Time
	UpdatedAt               time.Time
	FinishedAt              *time.Time
}

// OpenAIOAuthReauthCredentialUpdater is implemented by the account repository
// in production. It performs the credential swap with an expected-value guard
// so a concurrent admin edit cannot be overwritten by a stale re-auth task.
type OpenAIOAuthReauthCredentialUpdater interface {
	ApplyOpenAIOAuthReauth(ctx context.Context, taskID int64, workerID string, accountID int64, expectedCredentials, credentials, extra map[string]any) (bool, error)
}

type OpenAIOAuthReauthAccountReader interface {
	GetAccount(ctx context.Context, id int64) (*Account, error)
	GetProxy(ctx context.Context, id int64) (*Proxy, error)
}

type OpenAIOAuthReauthService struct {
	repo                    OpenAIOAuthReauthRepository
	accounts                OpenAIOAuthReauthAccountReader
	credentialUpdater       OpenAIOAuthReauthCredentialUpdater
	oauth                   *OpenAIOAuthService
	encryptor               SecretEncryptor
	encryptionKeyConfigured bool
	tokenCacheInvalidator   TokenCacheInvalidator
	runtimeBlocker          AccountRuntimeBlocker
}

func NewOpenAIOAuthReauthService(
	repo OpenAIOAuthReauthRepository,
	accounts OpenAIOAuthReauthAccountReader,
	credentialUpdater OpenAIOAuthReauthCredentialUpdater,
	oauth *OpenAIOAuthService,
	encryptor SecretEncryptor,
	encryptionKeyConfigured bool,
	tokenCacheInvalidator TokenCacheInvalidator,
	runtimeBlocker AccountRuntimeBlocker,
) *OpenAIOAuthReauthService {
	return &OpenAIOAuthReauthService{
		repo:                    repo,
		accounts:                accounts,
		credentialUpdater:       credentialUpdater,
		oauth:                   oauth,
		encryptor:               encryptor,
		encryptionKeyConfigured: encryptionKeyConfigured,
		tokenCacheInvalidator:   tokenCacheInvalidator,
		runtimeBlocker:          runtimeBlocker,
	}
}

func normalizeReauthEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 320 {
		return "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_EMAIL_INVALID", "a valid login email is required")
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsed.Address, email) || !strings.Contains(email, "@") {
		return "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_EMAIL_INVALID", "a valid login email is required")
	}
	return email, nil
}

// ValidateOpenAIOAuthReauthURL accepts HTTPS endpoints. Plain HTTP is only
// allowed for loopback addresses so local fake-mail tests remain possible
// without turning the saved mailbox URL into an unrestricted SSRF primitive.
func ValidateOpenAIOAuthReauthURL(raw string) (*url.URL, error) {
	if len(strings.TrimSpace(raw)) == 0 || len(raw) > maxOpenAIOAuthReauthURL {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_OTP_URL_INVALID", "a valid OTP API URL is required")
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_OTP_URL_INVALID", "OTP API URL must be an absolute URL without credentials or fragments")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if scheme != "https" && !(scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")) {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_OTP_URL_INVALID", "OTP API URL must use HTTPS (HTTP is allowed only for localhost tests)")
	}
	return u, nil
}

func MaskOpenAIOAuthReauthURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return "configured"
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

func (s *OpenAIOAuthReauthService) ensureReady() error {
	if s == nil || s.repo == nil || s.accounts == nil || s.credentialUpdater == nil || s.oauth == nil || s.encryptor == nil {
		return infraerrors.New(http.StatusServiceUnavailable, "OPENAI_REAUTH_UNAVAILABLE", "OpenAI automatic re-login is not available")
	}
	return nil
}

func (s *OpenAIOAuthReauthService) ensureDurableEncryption() error {
	if !s.encryptionKeyConfigured {
		return infraerrors.New(
			http.StatusBadRequest,
			"OPENAI_REAUTH_ENCRYPTION_KEY_REQUIRED",
			"set a fixed TOTP encryption key before saving an OTP mailbox URL",
		)
	}
	return nil
}

func (s *OpenAIOAuthReauthService) accountFor(ctx context.Context, accountID int64) (*Account, error) {
	if accountID <= 0 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_ACCOUNT_INVALID", "invalid account id")
	}
	account, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil || account == nil {
		return nil, infraerrors.New(http.StatusNotFound, "OPENAI_REAUTH_ACCOUNT_NOT_FOUND", "account not found")
	}
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth || account.IsCredentialShadow() ||
		account.IsOpenAIPersonalAccessToken() || account.IsOpenAIAgentIdentity() {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_ACCOUNT_INVALID", "only an OpenAI OAuth parent account can be re-authenticated")
	}
	return account, nil
}

func (s *OpenAIOAuthReauthService) SaveConfig(ctx context.Context, accountID int64, loginEmail, otpURL string) (*OpenAIOAuthReauthConfig, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	if _, err := s.accountFor(ctx, accountID); err != nil {
		return nil, err
	}
	email, err := normalizeReauthEmail(loginEmail)
	if err != nil {
		return nil, err
	}
	u, err := ValidateOpenAIOAuthReauthURL(otpURL)
	if err != nil {
		return nil, err
	}
	ciphertext, err := s.encryptor.Encrypt(u.String())
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_ENCRYPT_FAILED", "failed to encrypt OTP API URL")
	}
	if err := s.repo.UpsertConfig(ctx, accountID, email, ciphertext); err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_SAVE_FAILED", "failed to save OTP mailbox configuration")
	}
	return s.configView(ctx, &OpenAIOAuthReauthStoredConfig{AccountID: accountID, LoginEmail: email, OTPURLCiphertext: ciphertext, UpdatedAt: time.Now()})
}

func (s *OpenAIOAuthReauthService) configView(_ context.Context, stored *OpenAIOAuthReauthStoredConfig) (*OpenAIOAuthReauthConfig, error) {
	if stored == nil {
		return nil, nil
	}
	plain, err := s.encryptor.Decrypt(stored.OTPURLCiphertext)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_DECRYPT_FAILED", "saved OTP mailbox configuration cannot be decrypted")
	}
	return &OpenAIOAuthReauthConfig{
		AccountID:  stored.AccountID,
		LoginEmail: stored.LoginEmail,
		URLMasked:  MaskOpenAIOAuthReauthURL(plain),
		Configured: true,
		UpdatedAt:  stored.UpdatedAt,
	}, nil
}

func (s *OpenAIOAuthReauthService) GetStatus(ctx context.Context, accountID int64) (*OpenAIOAuthReauthStatus, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if _, err := s.accountFor(ctx, accountID); err != nil {
		return nil, err
	}
	status := &OpenAIOAuthReauthStatus{}
	stored, err := s.repo.GetConfig(ctx, accountID)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_READ_FAILED", "failed to read OTP mailbox configuration")
	}
	if stored != nil {
		status.Config, err = s.configView(ctx, stored)
		if err != nil {
			return nil, err
		}
	}
	record, err := s.repo.GetLatestTask(ctx, accountID)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_TASK_READ_FAILED", "failed to read re-login task")
	}
	status.Task = safeReauthTask(record)
	return status, nil
}

func (s *OpenAIOAuthReauthService) CreateTask(ctx context.Context, accountID int64) (*OpenAIOAuthReauthTask, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if err := s.ensureDurableEncryption(); err != nil {
		return nil, err
	}
	account, err := s.accountFor(ctx, accountID)
	if err != nil {
		return nil, err
	}
	stored, err := s.repo.GetConfig(ctx, accountID)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_READ_FAILED", "failed to read OTP mailbox configuration")
	}
	if stored == nil {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_CONFIG_REQUIRED", "save an OTP mailbox configuration first")
	}
	if _, err := s.encryptor.Decrypt(stored.OTPURLCiphertext); err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CONFIG_DECRYPT_FAILED", "saved OTP mailbox configuration cannot be decrypted")
	}
	expectedCredentialsHash, err := hashReauthCredentials(account.Credentials)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_CREDENTIAL_SNAPSHOT_FAILED", "failed to snapshot account credentials")
	}
	record, err := s.repo.CreateTask(ctx, accountID, expectedCredentialsHash)
	if err != nil {
		if infraerrors.Code(err) == http.StatusConflict {
			return nil, err
		}
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_TASK_CREATE_FAILED", "failed to create re-login task")
	}
	return safeReauthTask(record), nil
}

// OpenAIOAuthReauthClaim is returned only to an authenticated worker. The OTP
// URL is plaintext in this in-memory response and must never be logged.
type OpenAIOAuthReauthClaim struct {
	TaskID     int64  `json:"task_id"`
	AccountID  int64  `json:"account_id"`
	LoginEmail string `json:"login_email"`
	OTPURL     string `json:"otp_url"`
	AuthURL    string `json:"auth_url"`
	ProxyURL   string `json:"proxy_url,omitempty"`
}

func (s *OpenAIOAuthReauthService) ClaimTask(ctx context.Context, workerID string) (*OpenAIOAuthReauthClaim, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	workerID = strings.TrimSpace(workerID)
	if workerID == "" || len(workerID) > 128 {
		return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_WORKER_ID_INVALID", "worker id is required")
	}
	record, err := s.repo.ClaimNextTask(ctx, workerID, openAIOAuthReauthStaleAfter)
	if err != nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_TASK_CLAIM_FAILED", "failed to claim re-login task")
	}
	if record == nil {
		return nil, nil
	}
	account, err := s.accountFor(ctx, record.AccountID)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "account is no longer eligible for OpenAI OAuth re-login")
		return nil, nil
	}
	stored, err := s.repo.GetConfig(ctx, record.AccountID)
	if err != nil || stored == nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "OTP mailbox configuration is missing")
		return nil, nil
	}
	otpURL, err := s.encryptor.Decrypt(stored.OTPURLCiphertext)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "OTP mailbox configuration cannot be decrypted")
		return nil, nil
	}
	if _, err := ValidateOpenAIOAuthReauthURL(otpURL); err != nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "OTP mailbox URL is invalid")
		return nil, nil
	}
	proxyURL := ""
	if account.ProxyID != nil {
		proxy, proxyErr := s.accounts.GetProxy(ctx, *account.ProxyID)
		if proxyErr != nil || proxy == nil || strings.TrimSpace(proxy.URL()) == "" {
			_ = s.repo.MarkFailed(ctx, record.ID, workerID, "configured account proxy is unavailable")
			return nil, nil
		}
		proxyURL = proxy.URL()
	}
	auth, err := s.oauth.GenerateAuthURL(ctx, account.ProxyID, "", PlatformOpenAI)
	if err != nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "failed to create OpenAI authorization session")
		return nil, nil
	}
	if err := s.repo.SetSession(ctx, record.ID, workerID, auth.SessionID); err != nil {
		_ = s.repo.MarkFailed(ctx, record.ID, workerID, "failed to persist OpenAI authorization session")
		return nil, nil
	}
	return &OpenAIOAuthReauthClaim{
		TaskID: record.ID, AccountID: record.AccountID, LoginEmail: stored.LoginEmail,
		OTPURL: otpURL, AuthURL: auth.AuthURL, ProxyURL: proxyURL,
	}, nil
}

func (s *OpenAIOAuthReauthService) UpdateStage(ctx context.Context, taskID int64, workerID, stage string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if !validWorkerReauthStage(stage) {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_STAGE_INVALID", "invalid re-login task stage")
	}
	if err := s.repo.UpdateStage(ctx, taskID, strings.TrimSpace(workerID), stage); err != nil {
		return infraerrors.New(http.StatusConflict, "OPENAI_REAUTH_TASK_NOT_ACTIVE", "re-login task is no longer active")
	}
	return nil
}

func parseReauthCallback(callback string) (code, state string, err error) {
	if len(callback) == 0 || len(callback) > maxOpenAIOAuthReauthURL {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_CALLBACK_INVALID", "invalid OAuth callback")
	}
	u, parseErr := url.Parse(callback)
	if parseErr != nil || u == nil {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_CALLBACK_INVALID", "invalid OAuth callback")
	}
	if u.Scheme != "http" || !strings.EqualFold(u.Host, "localhost:1455") || u.User != nil ||
		u.Path != "/auth/callback" || u.RawPath != "" || u.Fragment != "" {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_CALLBACK_INVALID", "invalid OAuth callback")
	}
	q := u.Query()
	code = strings.TrimSpace(q.Get("code"))
	state = strings.TrimSpace(q.Get("state"))
	if code == "" || state == "" || len(code) > 2048 || len(state) > 512 {
		return "", "", infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_CALLBACK_INVALID", "OAuth callback is missing code or state")
	}
	return code, state, nil
}

func (s *OpenAIOAuthReauthService) SubmitCallback(ctx context.Context, taskID int64, workerID, callback string) (*OpenAIOAuthReauthTask, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	record, started, err := s.repo.BeginCallback(ctx, taskID, strings.TrimSpace(workerID))
	if err != nil {
		return nil, infraerrors.New(http.StatusConflict, "OPENAI_REAUTH_TASK_NOT_ACTIVE", "re-login task is no longer active")
	}
	if !started {
		return safeReauthTask(record), nil
	}
	account, err := s.accountFor(ctx, record.AccountID)
	if err != nil {
		return s.failCallback(ctx, taskID, "account is no longer eligible for OpenAI OAuth re-login", err)
	}
	currentCredentialsHash, err := hashReauthCredentials(account.Credentials)
	if err != nil || subtle.ConstantTimeCompare([]byte(currentCredentialsHash), []byte(record.ExpectedCredentialsHash)) != 1 {
		return s.failCallback(ctx, taskID, "account credentials changed while re-login was running", errors.New("credential snapshot mismatch"))
	}
	code, state, err := parseReauthCallback(callback)
	if err != nil {
		return s.failCallback(ctx, taskID, "invalid OAuth callback", err)
	}
	if err := s.repo.UpdateStage(ctx, taskID, record.WorkerID, OpenAIOAuthReauthStageExchangingToken); err != nil {
		return s.failCallback(ctx, taskID, "re-login task state changed unexpectedly", err)
	}
	tokenInfo, err := s.oauth.ExchangeCode(ctx, &OpenAIExchangeCodeInput{
		SessionID: record.AuthSessionID, Code: code, State: state, ProxyID: account.ProxyID,
	})
	if err != nil {
		return s.failCallback(ctx, taskID, "OpenAI authorization code exchange failed", err)
	}
	if err := validateReauthToken(account, tokenInfo); err != nil {
		return s.failCallback(ctx, taskID, "new OAuth identity does not match the account", err)
	}
	if strings.TrimSpace(tokenInfo.RefreshToken) == "" || strings.TrimSpace(tokenInfo.AccessToken) == "" {
		return s.failCallback(ctx, taskID, "OpenAI did not return a usable OAuth token", errors.New("missing token"))
	}
	newCredentials := s.oauth.BuildAccountCredentials(tokenInfo)
	for key, value := range account.Credentials {
		if _, exists := newCredentials[key]; !exists {
			newCredentials[key] = value
		}
	}
	newCredentials["_token_version"] = time.Now().UnixMilli()
	newCredentials = SanitizeStoredCredentials(account.Platform, newCredentials)
	extra := map[string]any{}
	if tokenInfo.Email != "" {
		extra["email"] = tokenInfo.Email
	}
	if tokenInfo.PrivacyMode != "" {
		extra["privacy_mode"] = tokenInfo.PrivacyMode
	}
	if err := s.repo.UpdateStage(ctx, taskID, record.WorkerID, OpenAIOAuthReauthStageApplyingCredentials); err != nil {
		return s.failCallback(ctx, taskID, "re-login task state changed unexpectedly", err)
	}
	taskFinalized, err := s.applyCredentials(ctx, taskID, record.WorkerID, account, account.Credentials, newCredentials, extra)
	if err != nil {
		return s.failCallback(ctx, taskID, "failed to apply new OAuth credentials", err)
	}
	if s.tokenCacheInvalidator != nil {
		updatedAccount := *account
		updatedAccount.Credentials = newCredentials
		if err := s.tokenCacheInvalidator.InvalidateToken(ctx, &updatedAccount); err != nil {
			// Credentials are already durable; cache invalidation is retried by the
			// normal token path and must not make a successful login look failed.
		}
	}
	if s.runtimeBlocker != nil {
		s.runtimeBlocker.ClearAccountSchedulingBlock(account.ID)
	}
	if !taskFinalized {
		if err := s.repo.MarkSucceeded(ctx, taskID, record.WorkerID); err != nil {
			return nil, infraerrors.New(http.StatusInternalServerError, "OPENAI_REAUTH_TASK_FINALIZE_FAILED", "credentials applied but task finalization failed")
		}
	}
	final, _ := s.repo.GetTask(ctx, taskID)
	return safeReauthTask(final), nil
}

func (s *OpenAIOAuthReauthService) failCallback(ctx context.Context, taskID int64, public string, cause error) (*OpenAIOAuthReauthTask, error) {
	record, _ := s.repo.GetTask(ctx, taskID)
	workerID := ""
	if record != nil {
		workerID = record.WorkerID
	}
	_ = s.repo.MarkFailed(ctx, taskID, workerID, public)
	if appErr, ok := cause.(*infraerrors.ApplicationError); ok {
		return nil, infraerrors.New(int(appErr.Code), "OPENAI_REAUTH_FAILED", public)
	}
	return nil, infraerrors.New(http.StatusBadRequest, "OPENAI_REAUTH_FAILED", public)
}

func (s *OpenAIOAuthReauthService) FailTask(ctx context.Context, taskID int64, workerID, reason string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	reason = sanitizeReauthError(reason)
	if reason == "" {
		reason = "worker reported a re-login failure"
	}
	if err := s.repo.MarkFailed(ctx, taskID, strings.TrimSpace(workerID), reason); err != nil {
		return infraerrors.New(http.StatusConflict, "OPENAI_REAUTH_TASK_NOT_ACTIVE", "re-login task is no longer active")
	}
	return nil
}

func (s *OpenAIOAuthReauthService) applyCredentials(ctx context.Context, taskID int64, workerID string, account *Account, expectedCredentials, credentials, extra map[string]any) (bool, error) {
	changed, err := s.credentialUpdater.ApplyOpenAIOAuthReauth(ctx, taskID, workerID, account.ID, expectedCredentials, credentials, extra)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, infraerrors.Conflict("OPENAI_REAUTH_ACCOUNT_CHANGED", "account credentials changed while re-login was running")
	}
	return true, nil
}

func validateReauthToken(account *Account, tokenInfo *OpenAITokenInfo) error {
	if account == nil || tokenInfo == nil {
		return infraerrors.BadRequest("OPENAI_REAUTH_IDENTITY_MISMATCH", "OpenAI token identity is missing")
	}
	hasStoredIdentity := false
	oldAccountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id"))
	newAccountID := strings.TrimSpace(tokenInfo.ChatGPTAccountID)
	if oldAccountID != "" {
		hasStoredIdentity = true
		if newAccountID == "" || subtle.ConstantTimeCompare([]byte(oldAccountID), []byte(newAccountID)) != 1 {
			return infraerrors.Conflict("OPENAI_REAUTH_IDENTITY_MISMATCH", "new OAuth login belongs to a different ChatGPT account")
		}
	}
	oldUserID := strings.TrimSpace(account.GetCredential("chatgpt_user_id"))
	newUserID := strings.TrimSpace(tokenInfo.ChatGPTUserID)
	if oldUserID != "" {
		hasStoredIdentity = true
		if newUserID == "" || subtle.ConstantTimeCompare([]byte(oldUserID), []byte(newUserID)) != 1 {
			return infraerrors.Conflict("OPENAI_REAUTH_IDENTITY_MISMATCH", "new OAuth login belongs to a different ChatGPT user")
		}
	}
	oldEmail := strings.ToLower(strings.TrimSpace(account.GetCredential("email")))
	newEmail := strings.ToLower(strings.TrimSpace(tokenInfo.Email))
	if oldEmail != "" {
		hasStoredIdentity = true
		if newEmail == "" || oldEmail != newEmail {
			return infraerrors.Conflict("OPENAI_REAUTH_EMAIL_MISMATCH", "new OAuth login email does not match the account")
		}
	}
	if !hasStoredIdentity {
		return infraerrors.BadRequest("OPENAI_REAUTH_IDENTITY_MISSING", "account has no stored OpenAI identity for verification")
	}
	return nil
}

func hashReauthCredentials(credentials map[string]any) (string, error) {
	payload, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func validWorkerReauthStage(stage string) bool {
	switch stage {
	case OpenAIOAuthReauthStageStarting, OpenAIOAuthReauthStageProtocolConnecting,
		OpenAIOAuthReauthStageEmailSubmitted, OpenAIOAuthReauthStageWaitingOTP,
		OpenAIOAuthReauthStageOTPSubmitted, OpenAIOAuthReauthStageWaitingCallback:
		return true
	default:
		return false
	}
}

func safeReauthTask(record *OpenAIOAuthReauthTaskRecord) *OpenAIOAuthReauthTask {
	if record == nil {
		return nil
	}
	return &OpenAIOAuthReauthTask{
		ID: record.ID, AccountID: record.AccountID, Status: record.Status,
		Stage: record.Stage, Error: sanitizeReauthError(record.Error), Attempt: record.Attempt,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt, FinishedAt: record.FinishedAt,
	}
}

func sanitizeReauthError(raw string) string {
	msg := strings.TrimSpace(raw)
	if msg == "" {
		return ""
	}
	// Do not let callback URLs, bearer-like strings, or long provider payloads
	// become durable task logs.
	if len(msg) > maxOpenAIOAuthReauthError {
		msg = msg[:maxOpenAIOAuthReauthError]
	}
	for _, marker := range []string{"access_token", "refresh_token", "code=", "state=", "?key=", "&key="} {
		if strings.Contains(strings.ToLower(msg), marker) {
			return "re-login failed; sensitive provider details were omitted"
		}
	}
	return msg
}
