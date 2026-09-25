package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func (s *OpenAIGatewayService) boundCodexTicket(req *http.Request, account *Account) *openAICodexTicket {
	if req == nil {
		return nil
	}
	return s.boundCodexTicketFromHeader(req.Context(), req.Header, account)
}

func (s *OpenAIGatewayService) boundCodexTicketFromHeader(ctx context.Context, h http.Header, account *Account) *openAICodexTicket {
	if s == nil || h == nil || account == nil || !s.openAICodexTicketEnabledContext(ctx) {
		return nil
	}
	sent := h.Get(openAICodexTurnStateHeader)
	if sent == "" {
		return nil
	}
	for _, model := range s.openAICodexTicketConfig().Models {
		ticket := s.lookupOpenAICodexTicket(account, model)
		if ticket != nil && ticket.State == sent && !ticket.Revoked {
			return ticket
		}
	}
	return nil
}

func (s *OpenAIGatewayService) codexTicketRequestBound(req *http.Request, account *Account) bool {
	return s.boundCodexTicket(req, account) != nil
}

func openAICodexReturnedState(account *Account, returned string, now time.Time, cfg config.OpenAICodexTicketConfig) (openAICodexTicketShape, bool) {
	returned = strings.TrimSpace(returned)
	shape, err := parseOpenAICodexTicketShape(returned)
	if err != nil || len(returned) != openAICodexTicketTargetLength(account, cfg) || shape.Blocks != openAICodexTicketExpectedBlocks(account) {
		return shape, false
	}
	if shape.IssuedAt.After(now.Add(30*time.Second)) || !now.Before(shape.IssuedAt.Add(codexTicketLifetime(len(returned)))) {
		return shape, false
	}
	return shape, true
}

// observeCodexTicketResponse records a valid turn-state rotation and marks a
// ticket revoked when the upstream returns a malformed state. It never retries
// the request: the response may already have been charged.
func (s *OpenAIGatewayService) observeCodexTicketResponse(req *http.Request, resp *http.Response, account *Account) {
	if s == nil || req == nil || resp == nil || account == nil {
		return
	}
	sent := strings.TrimSpace(req.Header.Get(openAICodexTurnStateHeader))
	returned := strings.TrimSpace(extractOpenAICodexTurnState(resp.Header))
	cfg := s.openAICodexTicketConfig()
	now := time.Now()
	s.openaiCodexTicketStateMu.Lock()
	defer s.openaiCodexTicketStateMu.Unlock()
	for _, model := range cfg.Models {
		model = normalizeOpenAICodexTicketModel(model)
		current := s.lookupOpenAICodexTicketLocked(account, model)
		if current == nil || current.Revoked {
			continue
		}
		if current.Length == 780 && sent != "" && current.State == sent && codex780RouteDeleted(resp) {
			next := *current
			next.Revoked = true
			s.openaiCodexTickets.Store(openAICodexTicketKey(account.ID, model), &next)
			return
		}
		if resp.StatusCode != http.StatusOK || sent == "" || returned == "" || !s.openAICodexTicketEnabledContext(req.Context()) || current.State != sent {
			continue
		}
		shape, acceptable := openAICodexReturnedState(account, returned, now, cfg)
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
