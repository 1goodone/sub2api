package service

import (
	"context"
	"net/http"
)

const (
	codexTurnStateOwnerLegacyTicket = "legacy_ticket"
	codexTurnStateOwnerNative       = "native"
)

func resolveCodexTicketOwner(account *Account, legacyTicketEnabled bool) string {
	if account == nil || !account.IsOpenAIOAuthLike() || account.IsShadow() {
		return codexTurnStateOwnerNative
	}
	if legacyTicketEnabled {
		return codexTurnStateOwnerLegacyTicket
	}
	return codexTurnStateOwnerNative
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
	return resolveCodexTicketOwner(account, s.openAICodexTicketEnabledContext(ctx))
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

// prepareCodexTurnStateHTTPRequest pins the global ticket policy for retries.
func (s *OpenAIGatewayService) prepareCodexTurnStateHTTPRequest(ctx context.Context, account *Account, model string, req *http.Request) error {
	requestCtx := withCodexTurnStateOwner(req.Context(), s.codexTurnStateOwner(ctx, account))
	*req = *req.WithContext(requestCtx)
	return s.applyOpenAICodexTicket(requestCtx, account, model, req.Header)
}
