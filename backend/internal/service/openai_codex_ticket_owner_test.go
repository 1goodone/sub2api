package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRetiredTurnStateModesFollowGlobalTicketPolicy(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
			for _, mode := range []any{"inherit", "legacy_ticket", "state_pool", "native", "invalid", true} {
				t.Run(fmt.Sprintf("%s/%v/global=%t", accountType, mode, enabled), func(t *testing.T) {
					svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: enabled, FailClosed: true}, nil)
					account := ticketTestAccount(41)
					account.Type = accountType
					account.Extra = map[string]any{
						"codex_turn_state_mode":            mode,
						"codex_turn_state_probe_cache":     map[string]any{"gpt-6-astra": map[string]any{"state": "retired-pool-state"}},
						"codex_turn_state_routing_cookies": map[string]any{"__cf_lb": "retired-cookie"},
					}
					require.Equal(t, enabled, svc.isOpenAIAccountRequestRuntimeBlocked(context.Background(), account, "gpt-6-astra", false))
					attachReadyCodexTicket(account, "gpt-6-astra")
					req, err := http.NewRequest(http.MethodPost, "https://example.test/responses", nil)
					require.NoError(t, err)
					req.Header.Set(openAICodexTurnStateHeader, "client-state")
					require.NoError(t, svc.prepareCodexTurnStateHTTPRequest(context.Background(), account, "gpt-6-astra", req))
					want := "client-state"
					owner := codexTurnStateOwnerNative
					if enabled {
						want = account.Extra[openAICodexTicketExtraKey("gpt-6-astra")].(*openAICodexTicket).State
						owner = codexTurnStateOwnerLegacyTicket
					}
					require.Equal(t, want, req.Header.Get(openAICodexTurnStateHeader))
					require.Empty(t, req.Header.Get("Cookie"))
					require.Equal(t, owner, svc.codexTurnStateOwner(req.Context(), account))
					svc.cfg.Gateway.OpenAICodexTicket.Enabled = !enabled
					require.Same(t, req.Context(), svc.PinCodexTurnStateOwner(req.Context(), account))
					require.Equal(t, owner, svc.codexTurnStateOwner(req.Context(), account))
				})
			}
		}
	}
}

func TestStripRetiredCodexTurnStateExtra(t *testing.T) {
	extra := map[string]any{
		"codex_turn_state_mode":              "state_pool",
		"codex_turn_state_probe_cache":       "private-state",
		"codex_turn_state_probe_failures":    "private-failure",
		"codex_turn_state_probe_enabled":     true,
		"codex_turn_state_probe_proxy_id":    7,
		"codex_turn_state_outbound_identity": "private-identity",
		"codex_turn_state_routing_cookies":   "private-cookie",
		"codex_turn_ticket:gpt-6-astra":      "existing-ticket",
		"codex_cli_only":                     true,
	}
	require.Equal(t, map[string]any{"codex_cli_only": true}, RedactOpenAICodexTurnStateExtra(extra))
	require.Contains(t, extra, "codex_turn_state_probe_cache", "redaction must not mutate the source")
	StripRetiredCodexTurnStateExtra(extra)
	require.Equal(t, map[string]any{"codex_turn_ticket:gpt-6-astra": "existing-ticket", "codex_cli_only": true}, extra)
	require.NotPanics(t, func() { StripRetiredCodexTurnStateExtra(nil) })
}
