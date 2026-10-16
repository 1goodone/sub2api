package service

import (
	"context"
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
