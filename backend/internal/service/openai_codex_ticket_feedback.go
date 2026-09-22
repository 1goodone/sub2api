package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func openAICodexReturnedState(account *Account, returned string, now time.Time, cfg config.OpenAICodexTicketConfig) (openAICodexTicketShape, bool) {
	returned = strings.TrimSpace(returned)
	shape, err := parseOpenAICodexTicketShape(returned)
	if err != nil || len(returned) != openAICodexTicketTargetLength(account, cfg) || shape.Blocks != openAICodexTicketExpectedBlocks(account) {
		return shape, false
	}
	if shape.IssuedAt.After(now.Add(30*time.Second)) || !now.Before(shape.IssuedAt.Add(time.Hour-30*time.Second)) {
		return shape, false
	}
	return shape, true
}

// observeCodexTicketResponse records a valid turn-state rotation and marks a
// ticket revoked when the upstream returns a malformed state. It never retries
// the request: the response may already have been charged.
func (s *OpenAIGatewayService) observeCodexTicketResponse(req *http.Request, resp *http.Response, account *Account) {
	if s == nil || req == nil || resp == nil || account == nil || resp.StatusCode != http.StatusOK {
		return
	}
	sent := strings.TrimSpace(req.Header.Get(openAICodexTurnStateHeader))
	returned := strings.TrimSpace(extractOpenAICodexTurnState(resp.Header))
	if sent == "" || returned == "" || !s.openAICodexTicketEnabledContext(req.Context()) {
		return
	}
	cfg := s.openAICodexTicketConfig()
	now := time.Now()
	shape, acceptable := openAICodexReturnedState(account, returned, now, cfg)

	s.openaiCodexTicketStateMu.Lock()
	defer s.openaiCodexTicketStateMu.Unlock()
	for _, model := range cfg.Models {
		model = normalizeOpenAICodexTicketModel(model)
		current := s.lookupOpenAICodexTicketLocked(account, model)
		if current == nil || current.State != sent || current.Revoked {
			continue
		}
		next := *current
		if acceptable {
			next.State = returned
			next.Length = len(returned)
			next.Blocks = shape.Blocks
			next.IssuedAt = shape.IssuedAt
			next.CapturedAt = now
			next.ExpiresAt = now.Add(time.Duration(cfg.TTLSeconds) * time.Second)
			next.Revoked = false
		} else if current.Standby != nil && current.Standby.validForAccount(account, now, cfg) {
			next = *current.Standby
			next.Standby = nil
		} else {
			next.Revoked = true
		}
		key := openAICodexTicketKey(account.ID, model)
		s.openaiCodexTickets.Store(key, &next)
		if s.accountRepo != nil {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), time.Second)
			_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{openAICodexTicketExtraKey(model): &next})
			cancel()
		}
	}
}
