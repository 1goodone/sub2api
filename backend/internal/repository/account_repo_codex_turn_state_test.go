package repository

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestCodexTurnStateCASFencesCredentialsModeAndRuntime(t *testing.T) {
	for _, changed := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
		mock.ExpectBegin()
		tx, err := client.Tx(context.Background())
		require.NoError(t, err)
		ctx := dbent.NewTxContext(context.Background(), tx)
		repo := &accountRepository{client: client}
		expected := &service.Account{ID: 41, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "test-token"},
			Extra:       map[string]any{service.CodexTurnStateModeExtraKey: service.CodexTurnStateModeStatePool},
		}
		updates := map[string]any{service.CodexTurnStateProbeCacheExtraKey: nil, service.CodexTurnStateProbeFailureExtraKey: nil}
		affected := int64(1)
		if changed {
			affected = 0
		}
		mock.ExpectExec(`(?s)UPDATE accounts SET extra =.*WHERE id = \$2.*credentials = \$5::jsonb.*jsonb_build_object.*codex_turn_state_mode.*codex_turn_state_probe_cache.*codex_turn_state_probe_failures.*= \$6::jsonb`).
			WithArgs(`{"codex_turn_state_probe_cache":null,"codex_turn_state_probe_failures":null}`, int64(41), service.PlatformOpenAI, service.AccountTypeOAuth,
				`{"access_token":"test-token"}`, `{"codex_turn_state_mode":"state_pool","codex_turn_state_probe_cache":null,"codex_turn_state_probe_failures":null}`).
			WillReturnResult(sqlmock.NewResult(0, affected))
		saved, err := repo.CompareAndSwapCodexTurnStateProbe(ctx, expected, updates)
		require.NoError(t, err)
		require.Equal(t, !changed, saved)
		_, err = repo.CompareAndSwapCodexTurnStateProbe(ctx, expected, map[string]any{"unrelated": true, service.CodexTurnStateProbeCacheExtraKey: nil})
		require.Error(t, err)
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
		_ = client.Close()
		_ = db.Close()
	}
}
