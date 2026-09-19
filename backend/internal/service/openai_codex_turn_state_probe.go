package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Account settings enable probing; the acquisition proxy is a global setting.
// The raw state is kept in
// a separate managed cache key so the admin DTO can expose the configuration
// without ever returning the opaque upstream value.
const (
	CodexTurnStateProbeEnabledExtraKey = "codex_turn_state_probe_enabled"
	CodexTurnStateProbeProxyIDExtraKey = "codex_turn_state_probe_proxy_id"
	CodexTurnStateProbeFailureExtraKey = "codex_turn_state_probe_failures"
	CodexTurnStateProbeRetryExtraKey   = "codex_turn_state_probe_retry"
	// Kept as a migration key for older configurations. Model selection now
	// comes from credentials.model_mapping and this key is removed on update.
	CodexTurnStateProbeModelsExtraKey = "codex_turn_state_probe_models"
	CodexTurnStateProbeCacheExtraKey  = "codex_turn_state_probe_cache"

	codexTurnStateLength           = 292
	codexTurnStateTTL              = time.Hour
	codexTurnStateProbeMaxAttempts = 25
	codexTurnStateProbeTimeout     = 10 * time.Second
)

// CodexTurnStateProbeConfig is the validated account-level setting. The model
// set is derived from the account's model restriction mapping.
type CodexTurnStateProbeConfig struct {
	Enabled bool
	ProxyID int64 // Filled from global settings when a scan starts, never from account extra.
}

type codexTurnStateProbeFailure struct {
	Attempts int
	FailedAt time.Time
	Reason   string
}

type codexTurnStateCacheEntry struct {
	State          string
	ObtainedAt     time.Time
	ExpiresAt      time.Time
	ProxyID        int64
	CredentialHash string
	Source         string
}

// IsCodexTurnStateProbeAccount reports whether this feature can use the
// account's own ChatGPT/Codex bearer token. CPR, API-key, and credential-shadow
// accounts are not eligible because sub2api does not hold their independent
// upstream ChatGPT token.
func IsCodexTurnStateProbeAccount(account *Account) bool {
	return account != nil && !account.IsCredentialShadow() && account.Platform == PlatformOpenAI && account.IsOpenAIOAuthLike()
}

// CodexTurnStateProbeConfigFromExtra parses and validates the user-controlled
// part of an account extra document. It intentionally ignores the managed
// cache key.
func CodexTurnStateProbeConfigFromExtra(extra map[string]any) (CodexTurnStateProbeConfig, error) {
	mode, err := codexTurnStateModeFromExtra(extra)
	if err != nil {
		return CodexTurnStateProbeConfig{}, err
	}
	return CodexTurnStateProbeConfig{Enabled: mode == CodexTurnStateModeStatePool}, nil
}

// NormalizeCodexTurnStateProbeExtra removes malformed/inapplicable settings
// from ordinary account updates. Model selection is derived from the account's
// model restriction mapping, while cache/failure state is managed separately.
func NormalizeCodexTurnStateProbeExtra(platform, accountType string, shadow bool, extra map[string]any) (map[string]any, error) {
	if extra == nil {
		return nil, nil
	}
	if shadow || platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) {
		delete(extra, CodexTurnStateModeExtraKey)
		delete(extra, CodexTurnStateProbeEnabledExtraKey)
		delete(extra, CodexTurnStateProbeProxyIDExtraKey)
		delete(extra, CodexTurnStateProbeModelsExtraKey)
		delete(extra, CodexTurnStateProbeRetryExtraKey)
		return extra, nil
	}

	_, err := codexTurnStateModeFromExtra(extra)
	if err != nil {
		return nil, err
	}
	// Mode is the sole owner switch. Retired probe settings must not silently
	// reactivate the pool when an account is switched to another engine.
	delete(extra, CodexTurnStateProbeEnabledExtraKey)
	delete(extra, CodexTurnStateProbeProxyIDExtraKey)
	delete(extra, CodexTurnStateProbeModelsExtraKey)
	delete(extra, CodexTurnStateProbeRetryExtraKey)
	return extra, nil
}

func positiveInt64Extra(value any) (int64, bool) {
	var parsed int64
	switch value := value.(type) {
	case int:
		parsed = int64(value)
	case int64:
		parsed = value
	case int32:
		parsed = int64(value)
	case uint:
		parsed = int64(value)
	case uint64:
		if value > uint64(^uint64(0)>>1) {
			return 0, false
		}
		parsed = int64(value)
	case float64:
		if value != float64(int64(value)) {
			return 0, false
		}
		parsed = int64(value)
	case json.Number:
		valueInt, err := value.Int64()
		if err != nil {
			return 0, false
		}
		parsed = valueInt
	case string:
		valueInt, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return 0, false
		}
		parsed = valueInt
	default:
		return 0, false
	}
	return parsed, parsed > 0
}

func normalizeCodexTurnStateModels(value any) []string {
	var raw []string
	switch value := value.(type) {
	case []string:
		raw = value
	case []any:
		for _, item := range value {
			if model, ok := item.(string); ok {
				raw = append(raw, model)
			}
		}
	case string:
		raw = append(raw, strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })...)
	}

	seen := make(map[string]struct{}, len(raw))
	models := make([]string, 0, len(raw))
	for _, model := range raw {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 200 {
			continue
		}
		if _, exists := seen[model]; exists {
			continue
		}
		seen[model] = struct{}{}
		models = append(models, model)
	}
	return models
}

// CodexTurnStateProbeModels derives the concrete upstream model IDs from the
// same model restriction mapping used by account routing. An empty mapping
// means the account allows all models, so there is no finite probe set.
func CodexTurnStateProbeModels(account *Account) []string {
	if !IsCodexTurnStateProbeAccount(account) || account.IsOpenAIPassthroughEnabled() {
		return nil
	}
	mapping := account.GetModelMapping()
	if len(mapping) == 0 {
		return nil
	}
	models := make([]string, 0, len(mapping))
	for _, mapped := range mapping {
		mapped = strings.TrimSpace(mapped)
		if mapped == "" || strings.Contains(mapped, "*") {
			continue
		}
		if account.UsesOpenAICodexProtocol() {
			mapped = normalizeOpenAIModelForUpstream(account, mapped)
		}
		models = append(models, mapped)
	}
	models = normalizeCodexTurnStateModels(models)
	sort.Strings(models)
	return models
}

// Callers select the engine before looking up a candidate. Do not re-read the
// mode here: an in-flight request must retain its pinned owner after an edit.
func configuredCodexTurnState(account *Account, model, token string, now time.Time) string {
	if !IsCodexTurnStateProbeAccount(account) || strings.TrimSpace(token) == "" {
		return ""
	}
	configuredModels := CodexTurnStateProbeModels(account)
	modelKey := codexTurnStateModelKey(account, model)
	if modelKey == "" || !containsString(configuredModels, modelKey) {
		return ""
	}
	entry, ok := codexTurnStateCacheFromExtra(account.Extra)[modelKey]
	if !ok || !validCodexTurnStateCacheEntry(entry, CodexTurnStateHealthyLength(account), token, now) {
		return ""
	}
	return entry.State
}

func validCodexTurnStateCacheEntry(entry codexTurnStateCacheEntry, expectedLength int, token string, now time.Time) bool {
	if !validCodexTurnStateCacheMetadata(entry, expectedLength, now) || entry.CredentialHash == "" {
		return false
	}
	return entry.CredentialHash == codexTurnStateCredentialHash(token)
}

func validCodexTurnStateCacheMetadata(entry codexTurnStateCacheEntry, expectedLength int, now time.Time) bool {
	if expectedLength == 0 || len(entry.State) != expectedLength || codexStateEffectiveExpiry(entry, now).IsZero() {
		return false
	}
	if entry.ObtainedAt.IsZero() || entry.ExpiresAt.IsZero() || !now.Before(entry.ExpiresAt) {
		return false
	}
	if entry.ExpiresAt.Sub(entry.ObtainedAt) > codexTurnStateTTL || entry.ExpiresAt.Before(entry.ObtainedAt) {
		return false
	}
	return true
}

func codexTurnStateModelKey(account *Account, model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if account != nil {
		if account.UsesOpenAICodexProtocol() {
			model = normalizeOpenAIModelForUpstream(account, model)
		}
	}
	return strings.TrimSpace(model)
}

func codexTurnStateCredentialHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func codexTurnStateCacheFromExtra(extra map[string]any) map[string]codexTurnStateCacheEntry {
	result := make(map[string]codexTurnStateCacheEntry)
	if extra == nil {
		return result
	}
	rawCache, ok := extra[CodexTurnStateProbeCacheExtraKey]
	if !ok {
		return result
	}
	cache, ok := rawCache.(map[string]any)
	if !ok {
		if typed, typedOK := rawCache.(map[string]codexTurnStateCacheEntry); typedOK {
			for model, entry := range typed {
				result[model] = entry
			}
		}
		return result
	}
	for model, rawEntry := range cache {
		entry, ok := decodeCodexTurnStateCacheEntry(rawEntry)
		if ok {
			result[model] = entry
		}
	}
	return result
}

func decodeCodexTurnStateCacheEntry(raw any) (codexTurnStateCacheEntry, bool) {
	if typed, ok := raw.(codexTurnStateCacheEntry); ok {
		return typed, true
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return codexTurnStateCacheEntry{}, false
	}
	state, ok := values["state"].(string)
	if !ok || state == "" {
		return codexTurnStateCacheEntry{}, false
	}
	obtainedAt, ok := parseCodexTurnStateTime(values["obtained_at"])
	if !ok {
		return codexTurnStateCacheEntry{}, false
	}
	expiresAt, ok := parseCodexTurnStateTime(values["expires_at"])
	if !ok {
		return codexTurnStateCacheEntry{}, false
	}
	proxyID, ok := positiveInt64Extra(values["proxy_id"])
	if !ok {
		return codexTurnStateCacheEntry{}, false
	}
	credentialHash, ok := values["credential_hash"].(string)
	if !ok || strings.TrimSpace(credentialHash) == "" {
		return codexTurnStateCacheEntry{}, false
	}
	source, _ := values["source"].(string)
	return codexTurnStateCacheEntry{
		Source:         source,
		State:          state,
		ObtainedAt:     obtainedAt,
		ExpiresAt:      expiresAt,
		ProxyID:        proxyID,
		CredentialHash: credentialHash,
	}, true
}

func parseCodexTurnStateTime(value any) (time.Time, bool) {
	switch value := value.(type) {
	case time.Time:
		return value, !value.IsZero()
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
		return parsed, err == nil && !parsed.IsZero()
	default:
		return time.Time{}, false
	}
}

func codexTurnStateCacheToExtra(cache map[string]codexTurnStateCacheEntry) map[string]any {
	if len(cache) == 0 {
		return nil
	}
	out := make(map[string]any, len(cache))
	for model, entry := range cache {
		out[model] = map[string]any{
			"state":           entry.State,
			"source":          entry.Source,
			"obtained_at":     entry.ObtainedAt.UTC().Format(time.RFC3339Nano),
			"expires_at":      entry.ExpiresAt.UTC().Format(time.RFC3339Nano),
			"proxy_id":        entry.ProxyID,
			"credential_hash": entry.CredentialHash,
		}
	}
	return out
}

func codexTurnStateProbeFailuresFromExtra(extra map[string]any) map[string]codexTurnStateProbeFailure {
	result := make(map[string]codexTurnStateProbeFailure)
	if extra == nil {
		return result
	}
	raw, ok := extra[CodexTurnStateProbeFailureExtraKey].(map[string]any)
	if !ok {
		return result
	}
	for model, rawFailure := range raw {
		values, ok := rawFailure.(map[string]any)
		if !ok {
			continue
		}
		attempts, err := strconv.ParseInt(fmt.Sprint(values["attempts"]), 10, 64)
		if err != nil || attempts < 0 {
			continue
		}
		failedAt, ok := parseCodexTurnStateTime(values["failed_at"])
		if !ok {
			continue
		}
		reason, _ := values["reason"].(string)
		result[model] = codexTurnStateProbeFailure{Attempts: int(min(attempts, int64(codexTurnStateProbeMaxAttempts))), FailedAt: failedAt, Reason: reason}
	}
	return result
}

func codexTurnStateProbeFailuresToExtra(failures map[string]codexTurnStateProbeFailure) map[string]any {
	if len(failures) == 0 {
		return nil
	}
	result := make(map[string]any, len(failures))
	for model, failure := range failures {
		result[model] = map[string]any{
			"attempts":  failure.Attempts,
			"failed_at": failure.FailedAt.UTC().Format(time.RFC3339Nano),
			"reason":    failure.Reason,
		}
	}
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// CodexTurnStateProbeService refreshes configured account/model candidates in
// the background using isolated HTTP SSE connections and account-scoped scheduling.
type CodexTurnStateProbeService struct {
	accountRepo   AccountRepository
	proxyRepo     ProxyRepository
	tokenProvider *OpenAITokenProvider
	httpUpstream  HTTPUpstream
	settingRepo   SettingRepository
	exitProber    ProxyExitInfoProber
	scheduleMu    sync.Mutex
	schedules     map[codexTurnStatePoolKey]*codexStateSchedule
	active        map[int64]bool
	lastCleanup   time.Time
	requestDo     func(*http.Request, string) (*http.Response, error)
	runtimeMu     sync.Mutex
	runtime       map[codexTurnStatePoolKey]codexTurnStateProbeRuntime
	importMu      sync.Mutex
	importJobs    map[string]*CodexStateImportJob
	importPending map[int64]bool

	startOnce sync.Once
	stopOnce  sync.Once
	stopCh    chan struct{}
	wakeCh    chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

func NewCodexTurnStateProbeService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	httpUpstream HTTPUpstream,
) *CodexTurnStateProbeService {
	ctx, cancel := context.WithCancel(context.Background())
	return &CodexTurnStateProbeService{
		accountRepo:   accountRepo,
		proxyRepo:     proxyRepo,
		tokenProvider: tokenProvider,
		httpUpstream:  httpUpstream,
		runtime:       make(map[codexTurnStatePoolKey]codexTurnStateProbeRuntime),
		importJobs:    make(map[string]*CodexStateImportJob),
		importPending: make(map[int64]bool),
		stopCh:        make(chan struct{}),
		wakeCh:        make(chan struct{}, 1),
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (s *CodexTurnStateProbeService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.run()
	})
}

func (s *CodexTurnStateProbeService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})
	s.importMu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.importMu.Unlock()
	s.wg.Wait()
}

func (s *CodexTurnStateProbeService) run() {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		s.dispatchStateProbes(s.ctx)
		select {
		case <-s.ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
		case <-s.wakeCh:
		}
	}
}

func (s *CodexTurnStateProbeService) probeToken(ctx context.Context, account *Account) (string, error) {
	if account.Type == AccountTypeOAuth {
		if s.tokenProvider == nil {
			return "", errors.New("OpenAI token provider is unavailable")
		}
		return s.tokenProvider.GetAccessToken(ctx, account)
	}
	token := strings.TrimSpace(account.GetOpenAIAccessToken())
	if token == "" {
		return "", errors.New("account has no access token")
	}
	return token, nil
}

// Wake schedules an early scan without interrupting the serial probe loop.
func (s *CodexTurnStateProbeService) Wake() {
	if s == nil {
		return
	}
	select {
	case s.wakeCh <- struct{}{}:
	default:
	}
}
