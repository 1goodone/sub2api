package service

import (
	"context"
	"errors"
	"strings"
)

const CodexTurnStateModeExtraKey = "codex_turn_state_mode"

const (
	CodexTurnStateModeInherit      = "inherit"
	CodexTurnStateModeLegacyTicket = "legacy_ticket"
	CodexTurnStateModeStatePool    = "state_pool"
	CodexTurnStateModeNative       = "native"
)

func codexTurnStateModeFromExtra(extra map[string]any) (string, error) {
	if extra == nil {
		return CodexTurnStateModeInherit, nil
	}
	raw, ok := extra[CodexTurnStateModeExtraKey]
	if !ok {
		return CodexTurnStateModeInherit, nil
	}
	mode, ok := raw.(string)
	if !ok {
		return "", errors.New(CodexTurnStateModeExtraKey + " must be a string")
	}
	mode = strings.TrimSpace(mode)
	switch mode {
	case CodexTurnStateModeInherit, CodexTurnStateModeLegacyTicket, CodexTurnStateModeStatePool, CodexTurnStateModeNative:
		return mode, nil
	default:
		return "", errors.New(CodexTurnStateModeExtraKey + " must be one of inherit, legacy_ticket, state_pool, native")
	}
}

func resolveCodexTurnStateMode(account *Account, legacyTicketEnabled bool) string {
	if account == nil || !account.IsOpenAIOAuthLike() || account.IsShadow() {
		return CodexTurnStateModeNative
	}
	mode, err := codexTurnStateModeFromExtra(account.Extra)
	if err != nil {
		return CodexTurnStateModeNative
	}
	if mode != CodexTurnStateModeInherit {
		return mode
	}
	if legacyTicketEnabled {
		return CodexTurnStateModeLegacyTicket
	}
	return CodexTurnStateModeNative
}

func codexTurnStateProbeEnabled(account *Account) bool {
	return IsCodexTurnStateProbeAccount(account) && resolveCodexTurnStateMode(account, false) == CodexTurnStateModeStatePool
}

func codexTurnStateUsesLegacyTicket(account *Account, legacyTicketEnabled bool) bool {
	return isOpenAICodexTicketAccount(account) && resolveCodexTurnStateMode(account, legacyTicketEnabled) == CodexTurnStateModeLegacyTicket
}

type codexTurnStateOwnerContextKey struct{}

func withCodexTurnStateOwner(ctx context.Context, owner string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, codexTurnStateOwnerContextKey{}, owner)
}

// PinCodexTurnStateOwner fixes the owner for the lifetime of one request.
// Later account retries are filtered against this value by the scheduler.
func (s *OpenAIGatewayService) PinCodexTurnStateOwner(ctx context.Context, account *Account) context.Context {
	if ctx != nil {
		if owner, ok := ctx.Value(codexTurnStateOwnerContextKey{}).(string); ok && owner != "" {
			return ctx
		}
	}
	return withCodexTurnStateOwner(ctx, s.resolveCodexTurnStateOwner(ctx, account))
}

func (s *OpenAIGatewayService) resolveCodexTurnStateOwner(ctx context.Context, account *Account) string {
	return resolveCodexTurnStateMode(account, s.openAICodexTicketEnabledContext(ctx))
}

func (s *OpenAIGatewayService) codexTurnStateOwnerCompatible(ctx context.Context, account *Account) bool {
	if ctx == nil {
		return true
	}
	owner, ok := ctx.Value(codexTurnStateOwnerContextKey{}).(string)
	return !ok || owner == "" || owner == s.resolveCodexTurnStateOwner(ctx, account)
}

func (s *OpenAIGatewayService) codexTurnStateOwner(ctx context.Context, account *Account) string {
	if ctx != nil {
		if owner, ok := ctx.Value(codexTurnStateOwnerContextKey{}).(string); ok && owner != "" {
			return owner
		}
	}
	return s.resolveCodexTurnStateOwner(ctx, account)
}
