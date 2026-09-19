package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const codexTurnStateGlobalProxySetting = "codex_turn_state_probe_proxy_id"

type CodexTurnStateGlobalConfig struct {
	ProxyID   int64  `json:"proxy_id"`
	ProxyName string `json:"proxy_name"`
	Available bool   `json:"available"`
	Issue     string `json:"issue"`
}

type codexTurnStatePoolKey struct {
	accountID int64
	model     string
}
type codexTurnStateProbeRuntime struct {
	Status    string
	Attempts  int
	UpdatedAt time.Time
	Reason    string
}

type CodexTurnStatePoolRow struct {
	AccountID      int64      `json:"account_id"`
	AccountName    string     `json:"account_name"`
	AccountStatus  string     `json:"account_status"`
	Plan           string     `json:"plan"`
	Model          string     `json:"model"`
	Enabled        bool       `json:"enabled"`
	ExpectedLength int        `json:"expected_length"`
	StateLength    int        `json:"state_length"`
	StateStatus    string     `json:"state_status"`
	ObtainedAt     *time.Time `json:"obtained_at,omitempty"`
	IssuedAt       *time.Time `json:"issued_at,omitempty"`
	Mode           string     `json:"mode"`
	NextProbeAt    *time.Time `json:"next_probe_at,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ProbeStatus    string     `json:"probe_status"`
	Attempts       int        `json:"attempts"`
	LastAttemptAt  *time.Time `json:"last_attempt_at,omitempty"`
	Reason         string     `json:"reason"`
	CanRefresh     bool       `json:"can_refresh"`
}

type CodexTurnStatePool struct {
	ServerTime time.Time                  `json:"server_time"`
	Config     CodexTurnStateGlobalConfig `json:"config"`
	Models     []string                   `json:"models"`
	Items      []CodexTurnStatePoolRow    `json:"items"`
}

func (s *CodexTurnStateProbeService) GetGlobalConfig(ctx context.Context) (CodexTurnStateGlobalConfig, error) {
	config := CodexTurnStateGlobalConfig{Issue: "proxy_not_configured"}
	if s.settingRepo == nil {
		return config, nil
	}
	value, err := s.settingRepo.GetValue(ctx, codexTurnStateGlobalProxySetting)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return config, err
	}
	config.ProxyID, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if config.ProxyID <= 0 {
		config.ProxyID = 0
		return config, nil
	}
	config.Issue = "proxy_unavailable"
	if s.proxyRepo == nil {
		return config, nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, config.ProxyID)
	if err != nil || proxy == nil {
		return config, nil
	}
	config.ProxyName = proxy.Name
	if proxy.IsActive() && !proxy.IsExpired(time.Now()) {
		config.Available = true
		config.Issue = ""
	}
	return config, nil
}

func (s *CodexTurnStateProbeService) SetGlobalProxy(ctx context.Context, id int64) (CodexTurnStateGlobalConfig, error) {
	if s.settingRepo == nil {
		return CodexTurnStateGlobalConfig{}, errors.New("global settings repository is unavailable")
	}
	if id < 0 {
		return CodexTurnStateGlobalConfig{}, infraerrors.BadRequest("INVALID_PROBE_PROXY", "proxy_id must not be negative")
	}
	if id > 0 {
		if s.proxyRepo == nil {
			return CodexTurnStateGlobalConfig{}, errors.New("proxy repository is unavailable")
		}
		proxy, err := s.proxyRepo.GetByID(ctx, id)
		if err != nil || proxy == nil || !proxy.IsActive() || proxy.IsExpired(time.Now()) {
			return CodexTurnStateGlobalConfig{}, infraerrors.BadRequest("INVALID_PROBE_PROXY", "probe proxy must be active and not expired")
		}
	}
	if err := s.settingRepo.Set(ctx, codexTurnStateGlobalProxySetting, strconv.FormatInt(id, 10)); err != nil {
		return CodexTurnStateGlobalConfig{}, err
	}
	s.Wake()
	return s.GetGlobalConfig(ctx)
}

func (s *CodexTurnStateProbeService) setProbeRuntime(id int64, model, status string, attempts int, reason string) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtime == nil {
		s.runtime = make(map[codexTurnStatePoolKey]codexTurnStateProbeRuntime)
	}
	s.runtime[codexTurnStatePoolKey{id, model}] = codexTurnStateProbeRuntime{Status: status, Attempts: attempts, Reason: reason, UpdatedAt: time.Now().UTC()}
}

func (s *CodexTurnStateProbeService) takeManualRefresh(id int64, model string) bool {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	key := codexTurnStatePoolKey{id, model}
	state := s.runtime[key]
	if state.Status != "queued" {
		return false
	}
	state.Status = "running"
	s.runtime[key] = state
	return true
}

func (s *CodexTurnStateProbeService) runtimeSnapshot() map[codexTurnStatePoolKey]codexTurnStateProbeRuntime {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	copyMap := make(map[codexTurnStatePoolKey]codexTurnStateProbeRuntime, len(s.runtime))
	for key, value := range s.runtime {
		copyMap[key] = value
	}
	return copyMap
}

func (s *CodexTurnStateProbeService) QueueRefresh(ctx context.Context, id int64, model string) error {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	model = codexTurnStateModelKey(account, model)
	if !codexTurnStateProbeEnabled(account) || account.Status != StatusActive || CodexTurnStateHealthyLength(account) == 0 || !containsString(CodexTurnStateProbeModels(account), model) {
		return infraerrors.BadRequest("PROBE_NOT_ELIGIBLE", "enable probing on an active account with a known plan and an exact model restriction")
	}
	config, err := s.GetGlobalConfig(ctx)
	if err != nil {
		return err
	}
	if !config.Available {
		return infraerrors.BadRequest("PROBE_PROXY_UNAVAILABLE", "configure an active global probe proxy first")
	}
	s.runtimeMu.Lock()
	runtimeKey := codexTurnStatePoolKey{id, model}
	runtimeState := s.runtime[runtimeKey]
	if runtimeState.Status != "running" && runtimeState.Status != "queued" {
		if s.runtime == nil {
			s.runtime = make(map[codexTurnStatePoolKey]codexTurnStateProbeRuntime)
		}
		s.runtime[runtimeKey] = codexTurnStateProbeRuntime{Status: "queued", UpdatedAt: time.Now().UTC()}
	}
	s.runtimeMu.Unlock()
	s.scheduleMu.Lock()
	if s.schedules == nil {
		s.schedules = make(map[codexTurnStatePoolKey]*codexStateSchedule)
	}
	scheduleKey := codexTurnStatePoolKey{id, model}
	scheduleState := s.schedules[scheduleKey]
	if scheduleState == nil {
		scheduleState = &codexStateSchedule{Next: time.Now().UTC()}
		s.schedules[scheduleKey] = scheduleState
	}
	scheduleState.ManualPending = true
	scheduleState.Next = time.Now().UTC()
	s.scheduleMu.Unlock()
	s.Wake()
	return nil
}

func (s *CodexTurnStateProbeService) Pool(ctx context.Context) (*CodexTurnStatePool, error) {
	config, err := s.GetGlobalConfig(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := &CodexTurnStatePool{ServerTime: now, Config: config, Items: make([]CodexTurnStatePoolRow, 0), Models: make([]string, 0)}
	models := map[string]bool{}
	runtime := s.runtimeSnapshot()
	for i := range accounts {
		account := &accounts[i]
		if !IsCodexTurnStateProbeAccount(account) {
			continue
		}
		cache := codexTurnStateCacheFromExtra(account.Extra)
		failures := codexTurnStateProbeFailuresFromExtra(account.Extra)
		for _, model := range CodexTurnStateProbeModels(account) {
			models[model] = true
			row := CodexTurnStatePoolRow{AccountID: account.ID, AccountName: account.Name, AccountStatus: account.Status, Plan: account.GetCredential("plan_type"), Model: model, Enabled: codexTurnStateProbeEnabled(account), ExpectedLength: CodexTurnStateHealthyLength(account), StateStatus: "missing", ProbeStatus: "idle"}
			if entry, ok := cache[model]; ok {
				row.StateLength = len(entry.State)
				row.ObtainedAt = &entry.ObtainedAt
				row.LastAttemptAt = &entry.ObtainedAt
				issued, expires, _ := codexStateTimes(entry.State, entry.ObtainedAt, now)
				row.IssuedAt = &issued
				row.ExpiresAt = &expires
				switch {
				case validCodexTurnStateCacheEntry(entry, row.ExpectedLength, account.GetOpenAIAccessToken(), now):
					row.StateStatus = "valid"
				case !now.Before(entry.ExpiresAt):
					row.StateStatus = "expired"
				default:
					row.StateStatus = "invalid"
				}
			}
			if row.ExpectedLength == 0 {
				row.StateStatus = "unknown_plan"
			}
			if !row.Enabled {
				row.StateStatus = "disabled"
			}
			if failure, ok := failures[model]; ok {
				row.Attempts = failure.Attempts
				row.LastAttemptAt = &failure.FailedAt
				row.Reason = failure.Reason
				if row.StateStatus == "missing" && failure.Reason != "" && failure.Reason != "probe_failed" {
					row.StateStatus = "invalid"
				}

			}
			if live, ok := runtime[codexTurnStatePoolKey{account.ID, model}]; ok {
				if live.Status == "queued" || live.Status == "running" {
					row.ProbeStatus = live.Status
					row.Attempts = live.Attempts
				}
				if row.LastAttemptAt == nil || live.UpdatedAt.After(*row.LastAttemptAt) {
					row.LastAttemptAt = &live.UpdatedAt
					row.Attempts = live.Attempts
					row.Reason = live.Reason
				}
			}
			entry := cache[model]
			row.Mode = codexStateMode(entry, row.ExpectedLength, account.GetOpenAIAccessToken(), now)
			s.scheduleMu.Lock()
			if scheduled := s.schedules[codexTurnStatePoolKey{account.ID, model}]; scheduled != nil {
				row.Mode = scheduled.Mode
				next := scheduled.Next
				row.NextProbeAt = &next
			}
			busy := s.active[account.ID]
			s.scheduleMu.Unlock()
			if row.Mode == "continuous" {
				row.ProbeStatus = "queued"
			}
			if busy {
				row.ProbeStatus = "running"
			}
			row.CanRefresh = row.Enabled && row.ExpectedLength > 0 && account.Status == StatusActive && config.Available
			if !config.Available || account.Status != StatusActive || !row.Enabled || row.ExpectedLength == 0 {
				row.ProbeStatus = "blocked"
				row.Mode = "paused"
			}
			result.Items = append(result.Items, row)
		}
	}
	for model := range models {
		result.Models = append(result.Models, model)
	}
	sort.Strings(result.Models)
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].AccountID != result.Items[j].AccountID {
			return result.Items[i].AccountID < result.Items[j].AccountID
		}
		return result.Items[i].Model < result.Items[j].Model
	})
	return result, nil
}

// RawState is deliberately separate from the list/DTO/export paths.
func (s *CodexTurnStateProbeService) RawState(ctx context.Context, id int64, model string) (string, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	value := ""
	if codexTurnStateProbeEnabled(account) {
		value = configuredCodexTurnState(account, model, account.GetOpenAIAccessToken(), time.Now())
	}
	if value == "" {
		return "", infraerrors.BadRequest("STATE_UNAVAILABLE", "no valid candidate for this account and model")
	}
	return value, nil
}
