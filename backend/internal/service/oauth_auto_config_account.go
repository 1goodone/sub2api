package service

import (
	"maps"
	"math"
	"slices"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// OAuthInitialAccountSettings holds optional account configuration applied only
// to new OAuth accounts. A nil field leaves the imported or default value alone.
type OAuthInitialAccountSettings struct {
	RateMultiplier           *float64 `json:"rate_multiplier,omitempty"`
	CostMultiplier           *float64 `json:"cost_multiplier,omitempty"`
	ProxyID                  *int64   `json:"proxy_id,omitempty"`
	AutoPauseOnExpired       *bool    `json:"auto_pause_on_expired,omitempty"`
	OpenAIPassthrough        *bool    `json:"openai_passthrough,omitempty"`
	OpenAIFlattenNamespaces  *bool    `json:"openai_flatten_namespaces,omitempty"`
	OpenAILongContextBilling *bool    `json:"openai_long_context_billing,omitempty"`
	CodexCLIOnly             *bool    `json:"codex_cli_only,omitempty"`
	CodexCLIOnlyAppServer    *bool    `json:"codex_cli_only_app_server,omitempty"`
	CodexFingerprintMode     *string  `json:"codex_fingerprint_mode,omitempty"`
	OpenAICompactMode        *string  `json:"openai_compact_mode,omitempty"`
	OpenAIRequestTimezone    *string  `json:"openai_request_timezone,omitempty"`
	SlotReservationEnabled   *bool    `json:"slot_reservation_enabled,omitempty"`
	SlotReservationSeconds   *int     `json:"slot_reservation_seconds,omitempty"`
	OpenAIWSMode             *string  `json:"openai_ws_mode,omitempty"`
	InterceptWarmup          *bool    `json:"intercept_warmup,omitempty"`
}

func validateInitialAccountSettings(c OAuthAutoConfig) error {
	bad := func(s string) error { return infraerrors.BadRequest("AUTO_CONFIG_INVALID", s) }
	s := c.AccountSettings
	if s.RateMultiplier != nil && (*s.RateMultiplier < 0 || math.IsNaN(*s.RateMultiplier) || math.IsInf(*s.RateMultiplier, 0)) {
		return bad("billing rate multiplier must be a finite number >= 0")
	}
	if s.CostMultiplier != nil {
		if err := ValidateAccountCostMultiplierExtra(map[string]any{AccountCostMultiplierExtraKey: *s.CostMultiplier}); err != nil {
			return bad("cost multiplier must be a finite number between 0 and 1000000")
		}
	}
	if s.ProxyID != nil && *s.ProxyID < 0 {
		return bad("proxy id must be >= 0")
	}
	if s.CodexCLIOnlyAppServer != nil && (s.CodexCLIOnly == nil || !*s.CodexCLIOnly) {
		return bad("app-server access requires Codex CLI only")
	}
	if s.CodexFingerprintMode != nil && !slices.Contains([]string{"off", "device", "session", "full"}, *s.CodexFingerprintMode) {
		return bad("invalid Codex fingerprint mode")
	}
	if s.OpenAICompactMode != nil && !slices.Contains([]string{OpenAICompactModeAuto, OpenAICompactModeForceOn, OpenAICompactModeForceOff}, *s.OpenAICompactMode) {
		return bad("invalid compact mode")
	}
	if s.OpenAIRequestTimezone != nil {
		if _, err := time.LoadLocation(*s.OpenAIRequestTimezone); err != nil {
			return bad("invalid request timezone")
		}
	}
	if s.SlotReservationEnabled != nil && *s.SlotReservationEnabled {
		if s.SlotReservationSeconds == nil || *s.SlotReservationSeconds < 1 || *s.SlotReservationSeconds > 3600 {
			return bad("slot reservation seconds must be 1–3600")
		}
	} else if s.SlotReservationSeconds != nil {
		return bad("slot reservation seconds require the feature to be enabled")
	}
	if s.OpenAIWSMode != nil && normalizeOpenAIWSIngressMode(*s.OpenAIWSMode) == "" {
		return bad("invalid WebSocket mode")
	}
	return nil
}

func applyInitialAccountSettings(input *CreateAccountInput, c OAuthAutoConfig) {
	s := c.AccountSettings
	if s.RateMultiplier != nil {
		v := *s.RateMultiplier
		input.RateMultiplier = &v
	}
	if s.ProxyID != nil {
		v := *s.ProxyID
		input.ProxyID = &v
	}
	if s.AutoPauseOnExpired != nil {
		v := *s.AutoPauseOnExpired
		input.AutoPauseOnExpired = &v
	}
	extra := map[string]any{}
	if s.CostMultiplier != nil {
		extra[AccountCostMultiplierExtraKey] = *s.CostMultiplier
	}
	if c.Platform == PlatformOpenAI {
		putBool(extra, "openai_passthrough", s.OpenAIPassthrough)
		if s.OpenAIPassthrough != nil && !*s.OpenAIPassthrough {
			extra["openai_oauth_passthrough"] = false
		}
		putBool(extra, "openai_responses_flatten_namespaces", s.OpenAIFlattenNamespaces)
		putBool(extra, openAILongContextBillingEnabledKey, s.OpenAILongContextBilling)
		putBool(extra, "codex_cli_only", s.CodexCLIOnly)
		putBool(extra, "codex_cli_only_allow_app_server", s.CodexCLIOnlyAppServer)
		putString(extra, codexFingerprintModeExtraKey, s.CodexFingerprintMode)
		putString(extra, "openai_compact_mode", s.OpenAICompactMode)
		putString(extra, "openai_request_timezone", s.OpenAIRequestTimezone)
		putBool(extra, SlotReservationEnabled, s.SlotReservationEnabled)
		if s.SlotReservationSeconds != nil {
			extra[SlotReservationSeconds] = *s.SlotReservationSeconds
		}
		if s.OpenAIWSMode != nil {
			mode := normalizeOpenAIWSIngressMode(*s.OpenAIWSMode)
			extra["openai_oauth_responses_websockets_v2_mode"] = mode
			extra["openai_oauth_responses_websockets_v2_enabled"] = mode != OpenAIWSIngressModeOff
		}
	}
	if len(extra) > 0 {
		if input.Extra == nil {
			input.Extra = map[string]any{}
		}
		for key, value := range extra {
			input.Extra[key] = value
		}
	}
	if s.InterceptWarmup != nil && (c.Platform == PlatformAnthropic || c.Platform == PlatformAntigravity) {
		input.Credentials = maps.Clone(input.Credentials)
		if input.Credentials == nil {
			input.Credentials = map[string]any{}
		}
		input.Credentials["intercept_warmup_requests"] = *s.InterceptWarmup
	}
}

func putBool(extra map[string]any, key string, value *bool) {
	if value != nil {
		extra[key] = *value
	}
}

func putString(extra map[string]any, key string, value *string) {
	if value != nil {
		extra[key] = *value
	}
}
