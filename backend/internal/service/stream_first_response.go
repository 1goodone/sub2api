package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func (s *SettingService) IsStreamFirstResponseEnabled(ctx context.Context) bool {
	if s != nil {
		return s.getGatewayForwardingSettingsCached(ctx).streamFirstResponseEnabled
	}
	if cached, ok := gatewayForwardingCache.Load().(*cachedGatewayForwardingSettings); ok && cached != nil && time.Now().UnixNano() < cached.expiresAt {
		return cached.streamFirstResponseEnabled
	}
	return false
}

// Heartbeats and SSE sentinels are transport traffic, not upstream responses.
func isStreamFirstResponse(data, eventType string) bool {
	data = strings.TrimSpace(data)
	if !gjson.Valid(data) || !gjson.Parse(data).IsObject() {
		return false
	}
	if kind := gjson.Get(data, "type").String(); kind != "" {
		eventType = kind
	}
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "ping", "pong", "keepalive", "heartbeat":
		return false
	}
	return true
}

func observeStreamFirstResponse(firstTokenMs **int, enabled bool, startTime time.Time, data, eventType string) {
	if !enabled || *firstTokenMs != nil || !isStreamFirstResponse(data, eventType) {
		return
	}
	ms := max(0, int(time.Since(startTime).Milliseconds()))
	*firstTokenMs = &ms
}

// A native OAuth rate-limit rejection is not output. Only structured evidence
// qualifies; a message containing "rate_limit" is not sufficient. The caller
// still checks whether an earlier event was committed and applies retry policy.
func isNativeOAuthFirstResponseRateLimit(account *Account, payload []byte) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth ||
		account.AuthConnectionID != "" || account.HasManagedOpenAIConnections() {
		return false
	}
	for _, prefix := range []string{"response.error", "error"} {
		if gjson.GetBytes(payload, prefix+".status_code").Int() == http.StatusTooManyRequests {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, prefix+".type").String()), "rate_limit_error") {
			return true
		}
		switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, prefix+".code").String())) {
		case "rate_limit_exceeded", "rate_limit_error":
			return true
		}
	}
	return false
}

func isOpenAIRelayConcurrencyRejection(payload []byte) bool {
	return openAIStreamFailedEventErrorCode(payload) == "gateway_concurrency_limit"
}

func isOpenAIRelayFirstResponseConcurrency(account *Account, payload []byte) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeAPIKey &&
		isOpenAIRelayConcurrencyRejection(payload)
}

func upstreamResponseElapsedMs(start time.Time) int {
	ms := int(time.Since(start).Milliseconds())
	if ms < 0 {
		return 0
	}
	return ms
}

// storedStreamFirstTokenMs is the value written as first_token_ms.
// When the switch is on, that value is the upstream response arrival, not the
// first stream event. eventMs stays unchanged for forwarding and failover.
func storedStreamFirstTokenMs(enabled bool, responseMs int, eventMs *int) *int {
	if !enabled {
		return eventMs
	}
	ms := responseMs
	if ms < 0 {
		ms = 0
	}
	return &ms
}

func reportedOpenAIFirstTokenMs(enabled bool, mode string, responseMs int, eventMs *int) *int {
	if enabled || mode == OpenAITTFTModeSemantic {
		return storedStreamFirstTokenMs(true, responseMs, nil)
	}
	return eventMs
}
