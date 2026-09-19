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
	if len(updates) != 2 {
		return false, errors.New("codex turn-state runtime update requires both cache and failures")
	}
	for key := range updates {
		if key != service.CodexTurnStateProbeCacheExtraKey && key != service.CodexTurnStateProbeFailureExtraKey {
			return false, errors.New("invalid Codex turn-state runtime field")
		}
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
	for _, key := range []string{service.CodexTurnStateModeExtraKey, service.CodexTurnStateProbeCacheExtraKey, service.CodexTurnStateProbeFailureExtraKey} {
		snapshot[key] = expected.Extra[key]
	}
	before, err := json.Marshal(snapshot)
	if err != nil {
		return false, err
	}
	// Only the two managed keys are stripped when cleared; unrelated JSON nulls
	// and concurrently updated account fields are preserved.
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, `UPDATE accounts SET extra =
  (COALESCE(extra, '{}'::jsonb) - 'codex_turn_state_probe_cache' - 'codex_turn_state_probe_failures') || jsonb_strip_nulls($1::jsonb), updated_at = NOW()
  WHERE id = $2 AND deleted_at IS NULL AND platform = $3 AND type = $4 AND credentials = $5::jsonb
  AND jsonb_build_object(
   'codex_turn_state_mode', extra -> 'codex_turn_state_mode',
   'codex_turn_state_probe_cache', extra -> 'codex_turn_state_probe_cache',
   'codex_turn_state_probe_failures', extra -> 'codex_turn_state_probe_failures'
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
