package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var ErrCodexTurnStateUnavailable = errors.New("codex turn-state unavailable")

func (s *OpenAIGatewayService) applyCodexTurnStateForHTTPRequest(ctx context.Context, account *Account, model, token string, headers http.Header) error {
	if headers == nil {
		return nil
	}
	switch s.codexTurnStateOwner(ctx, account) {
	case CodexTurnStateModeLegacyTicket:
		return s.applyOpenAICodexTicket(ctx, account, model, headers)
	case CodexTurnStateModeStatePool:
		headers.Del(openAICodexTurnStateHeader)
		now := time.Now()
		applyCodexRoutingCookies(headers, account, now)
		// A ticket without a live routing cookie is not sent. The request
		// continues without it; a missing ticket while the cookie is live
		// still fails closed.
		if !codexRoutingCookiesLive(account, now) {
			return nil
		}
		state := configuredCodexTurnState(account, model, token, now)
		if state == "" {
			return ErrCodexTurnStateUnavailable
		}
		headers.Set(openAICodexTurnStateHeader, state)
	}
	return nil
}

func (s *OpenAIGatewayService) applyCodexTurnStateForWSHandshake(ctx context.Context, account *Account, model string, headers http.Header) error {
	if headers == nil {
		return nil
	}
	switch s.codexTurnStateOwner(ctx, account) {
	case CodexTurnStateModeLegacyTicket:
		return s.applyOpenAICodexTicket(ctx, account, model, headers, "websocket")
	case CodexTurnStateModeStatePool:
		headers.Del(openAICodexTurnStateHeader)
		applyCodexRoutingCookies(headers, account, time.Now())
	}
	return nil
}

func applyCodexTurnStateToWSFrame(account *Account, owner, token string, payload []byte) ([]byte, error) {
	if owner != CodexTurnStateModeStatePool {
		return payload, nil
	}
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType != "" && eventType != "response.create" {
		return payload, nil
	}
	metadata := gjson.GetBytes(payload, "client_metadata")
	if metadata.Exists() && !metadata.IsObject() {
		return nil, errors.New("client_metadata must be an object")
	}
	model := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	now := time.Now()
	if !codexRoutingCookiesLive(account, now) {
		return payload, nil
	}
	state := configuredCodexTurnState(account, model, token, now)
	if state == "" {
		return nil, ErrCodexTurnStateUnavailable
	}
	updated, err := sjson.SetBytes(payload, "client_metadata."+openAICodexTurnStateHeader, state)
	if err != nil {
		return nil, err
	}
	return updated, nil
}
