package service

import (
	"context"
	"log/slog"
	"time"
)

type CodexStateEvent struct {
	ID             int64      `json:"id"`
	AccountID      int64      `json:"account_id"`
	Model          string     `json:"model"`
	Kind           string     `json:"kind"`
	Source         string     `json:"source"`
	Reason         string     `json:"reason"`
	Attempt        int        `json:"attempt"`
	ProxyID        int64      `json:"proxy_id"`
	ProxyName      string     `json:"proxy_name"`
	ProxyAddress   string     `json:"proxy_address"`
	ReferenceIP    string     `json:"reference_ip"`
	IPStatus       string     `json:"ip_status"`
	HTTPStatus     int        `json:"http_status"`
	SentLength     int        `json:"sent_length"`
	ReturnedLength int        `json:"returned_length"`
	DurationMS     int64      `json:"duration_ms"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

type CodexStateEventRepository interface {
	AppendCodexStateEvent(context.Context, CodexStateEvent) error
	ListCodexStateEvents(context.Context, int64, string, int64, int) ([]CodexStateEvent, error)
	CleanupCodexStateEvents(context.Context, time.Time) error
}

func recordCodexStateEvent(repo AccountRepository, event CodexStateEvent) {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	store, ok := repo.(CodexStateEventRepository)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := store.AppendCodexStateEvent(ctx, event); err != nil {
		slog.Warn("codex_state_event_write_failed", "account_id", event.AccountID)
	}
}

func (s *CodexTurnStateProbeService) Events(ctx context.Context, id int64, model string, before int64) ([]CodexStateEvent, error) {
	store, ok := s.accountRepo.(CodexStateEventRepository)
	if !ok {
		return []CodexStateEvent{}, nil
	}
	return store.ListCodexStateEvents(ctx, id, model, before, 50)
}

func (s *CodexTurnStateProbeService) referenceIP(ctx context.Context, proxy *Proxy, event *CodexStateEvent) {
	event.IPStatus = "unavailable"
	prober, ok := s.exitProber.(interface {
		ProbeProxyOnce(context.Context, string) (*ProxyExitInfo, int64, error)
	})
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, _, err := prober.ProbeProxyOnce(ctx, proxy.URL())
	if err != nil || info == nil {
		event.IPStatus = "failed"
		return
	}
	event.ReferenceIP = info.IP
	event.IPStatus = "queried_after_request"
}
