package repository

import (
	"context"
	"encoding/json"
	"errors"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var _ service.CodexTurnStateProbeRepository = (*accountRepository)(nil)

func (r *accountRepository) CompareAndSwapCodexTurnStateProbe(ctx context.Context, expected *service.Account, updates map[string]any) (bool, error) {
	if expected == nil {
		return false, service.ErrAccountNilInput
	}
	if len(updates) < 2 || len(updates) > 4 {
		return false, errors.New("codex turn-state runtime update requires both cache and failures")
	}
	hasCache, hasFailures := false, false
	for key := range updates {
		switch key {
		case service.CodexTurnStateProbeCacheExtraKey:
			hasCache = true
		case service.CodexTurnStateProbeFailureExtraKey:
			hasFailures = true
		case service.CodexTurnStateOutboundIdentityExtraKey, service.CodexTurnStateRoutingCookieExtraKey:
		default:
			return false, errors.New("invalid Codex turn-state runtime field")
		}
	}
	if !hasCache || !hasFailures {
		return false, errors.New("codex turn-state runtime update requires both cache and failures")
	}
	payload, err := json.Marshal(updates)
	if err != nil {
		return false, err
	}
	credentials, err := json.Marshal(normalizeJSONMap(expected.Credentials))
	if err != nil {
		return false, err
	}
	snapshot := make(map[string]any)
	for _, key := range []string{service.CodexTurnStateModeExtraKey, service.CodexTurnStateOutboundIdentityExtraKey, service.CodexTurnStateProbeCacheExtraKey, service.CodexTurnStateProbeFailureExtraKey, service.CodexTurnStateRoutingCookieExtraKey} {
		snapshot[key] = expected.Extra[key]
	}
	before, err := json.Marshal(snapshot)
	if err != nil {
		return false, err
	}
	// Cache and failures are always replaced. The outbound identity is stripped
	// only when this write includes it, so a ticket-only update cannot drop it.
	// The snapshot still fences the identity: a concurrent bundle swap loses.
	strip := " - 'codex_turn_state_probe_cache' - 'codex_turn_state_probe_failures'"
	if _, ok := updates[service.CodexTurnStateOutboundIdentityExtraKey]; ok {
		strip += " - 'codex_turn_state_outbound_identity'"
	}
	if _, ok := updates[service.CodexTurnStateRoutingCookieExtraKey]; ok {
		strip += " - 'codex_turn_state_routing_cookies'"
	}
	// Only the managed keys are stripped when cleared; unrelated JSON nulls
	// and concurrently updated account fields are preserved.
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, `UPDATE accounts SET extra =
  (COALESCE(extra, '{}'::jsonb)`+strip+`) || jsonb_strip_nulls($1::jsonb), updated_at = NOW()
  WHERE id = $2 AND deleted_at IS NULL AND platform = $3 AND type = $4 AND credentials = $5::jsonb
  AND jsonb_build_object(
   'codex_turn_state_mode', extra -> 'codex_turn_state_mode',
   'codex_turn_state_outbound_identity', extra -> 'codex_turn_state_outbound_identity',
   'codex_turn_state_probe_cache', extra -> 'codex_turn_state_probe_cache',
   'codex_turn_state_probe_failures', extra -> 'codex_turn_state_probe_failures',
   'codex_turn_state_routing_cookies', extra -> 'codex_turn_state_routing_cookies'
  ) = $6::jsonb`, string(payload), expected.ID, expected.Platform, expected.Type, string(credentials), string(before))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	if dbent.TxFromContext(ctx) == nil {
		r.syncSchedulerAccountSnapshot(ctx, expected.ID)
	}
	return true, nil
}
