package service

import (
	"context"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ContentModerationEngineOpenAI   = "openai"
	ContentModerationEngineTypeSafe = "typesafe"
)

type ContentModerationEngineConfig struct {
	BaseURL    string             `json:"base_url"`
	BaseURLs   []string           `json:"base_urls,omitempty"`
	Model      string             `json:"model"`
	ProxyID    *int64             `json:"proxy_id,omitempty"`
	APIKeys    []string           `json:"api_keys,omitempty"`
	TimeoutMS  int                `json:"timeout_ms"`
	RetryCount int                `json:"retry_count"`
	Thresholds map[string]float64 `json:"thresholds"`
}

type UpdateContentModerationEngineInput struct {
	BaseURL            *string                        `json:"base_url"`
	Model              *string                        `json:"model"`
	ProxyID            *int64                         `json:"proxy_id"`
	APIKey             *string                        `json:"api_key"`
	APIKeys            *[]string                      `json:"api_keys"`
	APIKeySlots        *[]ContentModerationAPIKeySlot `json:"api_key_slots"`
	APIKeysMode        string                         `json:"api_keys_mode"`
	DeleteAPIKeyHashes *[]string                      `json:"delete_api_key_hashes"`
	ClearAPIKey        bool                           `json:"clear_api_key"`
	TimeoutMS          *int                           `json:"timeout_ms"`
	RetryCount         *int                           `json:"retry_count"`
	Thresholds         *map[string]float64            `json:"thresholds"`
}

type ContentModerationAPIKeySlot struct {
	Key      string `json:"key"`
	KeepHash string `json:"keep_hash"`
}

func applyModerationAPIKeySlots(existing []string, slots []ContentModerationAPIKeySlot, engine string) ([]string, error) {
	byHash := make(map[string]string, len(existing))
	for _, key := range existing {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		byHash[scopedModerationKeyHash(key, engine)] = key
	}
	filled := 0
	for _, slot := range slots {
		if strings.TrimSpace(slot.Key) != "" || strings.TrimSpace(slot.KeepHash) != "" {
			filled++
		}
	}
	if filled == 0 {
		return []string{}, nil
	}
	out := make([]string, 0, len(slots))
	for i, slot := range slots {
		key := strings.TrimSpace(slot.Key)
		if key != "" {
			out = append(out, key)
			continue
		}
		hash := strings.TrimSpace(slot.KeepHash)
		if hash == "" {
			return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BASE_URL", fmt.Sprintf("第 %d 行缺少 API Key", i+1))
		}
		kept, ok := byHash[hash]
		if !ok {
			return nil, infraerrors.BadRequest("INVALID_CONTENT_MODERATION_BASE_URL", fmt.Sprintf("第 %d 行保存的 API Key 已变化，请重新填写", i+1))
		}
		out = append(out, kept)
	}
	return normalizeModerationAPIKeys(out), nil
}

type ContentModerationEngineMeta struct {
	Engine        string `json:"engine"`
	Model         string `json:"model"`
	RulesVersion  string `json:"rules_version"`
	SkippedImages int    `json:"skipped_images"`
}

func moderationEngine(engine string) string {
	if engine == "" {
		return ContentModerationEngineOpenAI
	}
	return engine
}

func validModerationEngine(engine string) bool {
	return engine == ContentModerationEngineOpenAI || engine == ContentModerationEngineTypeSafe
}

func moderationEngineDefaults(engine string) *ContentModerationEngineConfig {
	p := &ContentModerationEngineConfig{BaseURL: defaultContentModerationBaseURL, Model: defaultContentModerationModel, TimeoutMS: 3000, RetryCount: 2, Thresholds: ContentModerationDefaultThresholds()}
	if engine == ContentModerationEngineTypeSafe {
		p.BaseURL, p.Model = "https://api.typesafe.ai", "jev-latest"
	}
	return p
}

func (cfg *ContentModerationConfig) engineProfile(engine string) *ContentModerationEngineConfig {
	var p ContentModerationEngineConfig
	if engine == ContentModerationEngineTypeSafe {
		if cfg.TypeSafe == nil {
			return moderationEngineDefaults(engine)
		}
		p = *cfg.TypeSafe
	} else {
		p = ContentModerationEngineConfig{BaseURL: cfg.BaseURL, BaseURLs: append([]string(nil), cfg.BaseURLs...), Model: cfg.Model, ProxyID: cfg.ProxyID, APIKeys: cfg.apiKeys(), TimeoutMS: cfg.TimeoutMS, RetryCount: cfg.RetryCount, Thresholds: cfg.Thresholds}
	}
	p.ProxyID = cloneInt64Ptr(p.ProxyID)
	p.APIKeys = append([]string(nil), p.APIKeys...)
	p.BaseURLs = append([]string(nil), p.BaseURLs...)
	p.Thresholds = cloneFloatMap(p.Thresholds)
	p.normalize(engine)
	return &p
}

func (p *ContentModerationEngineConfig) normalize(engine string) {
	if p == nil {
		return
	}
	defaults := moderationEngineDefaults(engine)
	p.BaseURLs = collectModerationBaseURLs(append([]string{p.BaseURL}, p.BaseURLs...))
	if len(p.BaseURLs) == 0 {
		p.BaseURLs = []string{defaults.BaseURL}
	}
	p.BaseURL = p.BaseURLs[0]
	p.Model = strings.TrimSpace(p.Model)
	if p.Model == "" {
		p.Model = defaults.Model
	}
	if p.TimeoutMS <= 0 {
		p.TimeoutMS = defaults.TimeoutMS
	}
	if p.TimeoutMS > maxContentModerationTimeoutMS {
		p.TimeoutMS = maxContentModerationTimeoutMS
	}
	if p.RetryCount < 0 {
		p.RetryCount = defaults.RetryCount
	}
	if len(p.Thresholds) == 0 {
		p.Thresholds = cloneFloatMap(defaults.Thresholds)
	}
}

func (p *ContentModerationEngineConfig) baseURLs() []string {
	if p == nil {
		return nil
	}
	if len(p.BaseURLs) > 0 {
		return p.BaseURLs
	}
	if strings.TrimSpace(p.BaseURL) != "" {
		return []string{strings.TrimRight(strings.TrimSpace(p.BaseURL), "/")}
	}
	return nil
}

func (cfg *ContentModerationConfig) applyEngineProfile(p *ContentModerationEngineConfig) {
	cfg.BaseURL, cfg.BaseURLs, cfg.Model, cfg.ProxyID = p.BaseURL, append([]string(nil), p.BaseURLs...), p.Model, cloneInt64Ptr(p.ProxyID)
	cfg.APIKey, cfg.APIKeys = "", append([]string(nil), p.APIKeys...)
	cfg.TimeoutMS, cfg.RetryCount = p.TimeoutMS, p.RetryCount
	cfg.Thresholds = cloneFloatMap(p.Thresholds)
}

func (cfg *ContentModerationConfig) effectiveEngine(engine string) *ContentModerationConfig {
	out := cloneContentModerationConfig(cfg)
	out.Engine = moderationEngine(engine)
	out.applyEngineProfile(cfg.engineProfile(out.Engine))
	return out
}

func (s *ContentModerationService) updateEngineProfile(ctx context.Context, cfg *ContentModerationConfig, engine string, input UpdateContentModerationEngineInput) error {
	if !validModerationEngine(engine) {
		return infraerrors.BadRequest("INVALID_CONTENT_MODERATION_ENGINE", "内容审计引擎无效")
	}
	p := cfg.engineProfile(engine)
	defaults := moderationEngineDefaults(engine)
	if input.BaseURL != nil {
		urls := collectModerationBaseURLs([]string{*input.BaseURL})
		if len(urls) == 0 {
			urls = []string{defaults.BaseURL}
		}
		p.BaseURLs = urls
		p.BaseURL = urls[0]
	}
	if input.Model != nil {
		p.Model = strings.TrimSpace(*input.Model)
		if p.Model == "" {
			p.Model = defaults.Model
		}
	}
	if input.ProxyID != nil {
		p.ProxyID = cloneInt64Ptr(input.ProxyID)
		if *input.ProxyID <= 0 {
			p.ProxyID = nil
		}
	}
	if input.TimeoutMS != nil {
		p.TimeoutMS = *input.TimeoutMS
	}
	if input.RetryCount != nil {
		p.RetryCount = *input.RetryCount
	}
	if input.Thresholds != nil {
		p.Thresholds = mergeContentModerationThresholds(defaults.Thresholds, *input.Thresholds)
	}
	if input.ClearAPIKey {
		p.APIKeys = []string{}
	} else if input.APIKeySlots != nil {
		keys, err := applyModerationAPIKeySlots(p.APIKeys, *input.APIKeySlots, engine)
		if err != nil {
			return err
		}
		p.APIKeys = keys
	} else {
		mode := normalizeContentModerationAPIKeysMode(input.APIKeysMode)
		if input.DeleteAPIKeyHashes != nil && mode != contentModerationAPIKeysModeReplace {
			p.APIKeys = deleteModerationAPIKeysByHash(p.APIKeys, *input.DeleteAPIKeyHashes, engine)
		}
		if input.APIKeys != nil {
			if mode == contentModerationAPIKeysModeReplace {
				p.APIKeys = normalizeModerationAPIKeys(*input.APIKeys)
			} else {
				p.APIKeys = normalizeModerationAPIKeys(append(p.APIKeys, *input.APIKeys...))
			}
		}
		if input.APIKey != nil {
			p.APIKeys = normalizeModerationAPIKeys(append(p.APIKeys, *input.APIKey))
		}
	}
	check := cloneContentModerationConfig(cfg)
	check.Engine = engine
	check.applyEngineProfile(p)
	if err := s.validateConfig(ctx, check); err != nil {
		return err
	}
	p = &ContentModerationEngineConfig{BaseURL: check.BaseURL, BaseURLs: append([]string(nil), check.BaseURLs...), Model: check.Model, ProxyID: check.ProxyID, APIKeys: check.APIKeys, TimeoutMS: check.TimeoutMS, RetryCount: check.RetryCount, Thresholds: check.Thresholds}
	if engine == ContentModerationEngineTypeSafe {
		cfg.TypeSafe = p
	} else {
		cfg.applyEngineProfile(p)
	}
	return nil
}

func (s *ContentModerationService) engineConfigView(cfg *ContentModerationConfig) *ContentModerationConfigView {
	view := s.configView(cfg.effectiveEngine(cfg.Engine))
	view.EngineConfigs = map[string]*ContentModerationConfigView{}
	for _, engine := range []string{ContentModerationEngineOpenAI, ContentModerationEngineTypeSafe} {
		view.EngineConfigs[engine] = s.configView(cfg.effectiveEngine(engine))
	}
	return view
}

func scopedModerationKeyHash(key string, engine ...string) string {
	if len(engine) > 0 && engine[0] == ContentModerationEngineTypeSafe {
		return moderationAPIKeyHash("typesafe\x00" + key)
	}
	return moderationAPIKeyHash(key)
}
