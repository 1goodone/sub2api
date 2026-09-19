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
		state := configuredCodexTurnState(account, model, token, time.Now())
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
		return s.applyOpenAICodexTicket(ctx, account, model, headers)
	case CodexTurnStateModeStatePool:
		headers.Del(openAICodexTurnStateHeader)
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
	state := configuredCodexTurnState(account, model, token, time.Now())
	if state == "" {
		return nil, ErrCodexTurnStateUnavailable
	}
	updated, err := sjson.SetBytes(payload, "client_metadata."+openAICodexTurnStateHeader, state)
	if err != nil {
		return nil, err
	}
	return updated, nil
}
