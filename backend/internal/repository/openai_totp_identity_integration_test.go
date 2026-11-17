//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTOTPRotationConcurrentIdentityHasSingleWinner(t *testing.T) {
	for _, automaticFirst := range []bool{false, true} {
		for _, automaticSecond := range []bool{false, true} {
			t.Run(fmt.Sprintf("automatic=%t,%t", automaticFirst, automaticSecond), func(t *testing.T) {
				base, repo, first := totpRotationFixture(t)
				_, _, second := totpRotationFixture(t)
				second.LoginEmail = " OWNER@EXAMPLE.TEST "
				require.NoError(t, repo.UpsertConfig(base, second))
				second, err := repo.GetConfig(base, second.AccountID)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(base, 20*time.Second)
				defer cancel()
				gate, err := integrationDB.Conn(ctx)
				require.NoError(t, err)
				defer gate.Close()
				_, err = gate.ExecContext(ctx, "SELECT pg_advisory_lock(892895)")
				require.NoError(t, err)
				defer gate.ExecContext(base, "SELECT pg_advisory_unlock(892895)")
				// Pause insertion after the identity check. Before the fix both
				// transactions reach this gate; with the fix the second waits on
				// the identity lock and must recheck after the first commits.
				_, err = integrationDB.ExecContext(ctx, `CREATE FUNCTION totp_identity_test_gate() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_advisory_xact_lock_shared(892895); RETURN NEW; END $$;
CREATE TRIGGER totp_identity_test_gate BEFORE INSERT ON openai_oauth_reauth_tasks FOR EACH ROW EXECUTE FUNCTION totp_identity_test_gate()`)
				require.NoError(t, err)
				defer integrationDB.ExecContext(base, "DROP TRIGGER totp_identity_test_gate ON openai_oauth_reauth_tasks; DROP FUNCTION totp_identity_test_gate()")
				type outcome struct {
					task *service.OpenAITOTPRotation
					err  error
				}
				results := make(chan outcome, 2)
				for i, cfg := range []*service.OpenAIOAuthReauthStoredConfig{first, second} {
					automatic := []bool{automaticFirst, automaticSecond}[i]
					go func() {
						task, err := repo.createTOTPRotation(ctx, cfg.AccountID, "fixture-snapshot", cfg.UpdatedAt, automatic)
						results <- outcome{task, err}
					}()
				}
				assert.Eventually(t, func() bool {
					var count int
					err := integrationDB.QueryRowContext(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE datname=current_database() AND wait_event_type='Lock'
AND (query LIKE '%pg_advisory_xact_lock(hashtextextended%' OR query LIKE 'INSERT INTO openai_oauth_reauth_tasks%')`).Scan(&count)
					return err == nil && count == 2
				}, 5*time.Second, 10*time.Millisecond)
				_, err = gate.ExecContext(ctx, "SELECT pg_advisory_unlock(892895)")
				require.NoError(t, err)
				created, conflicts := 0, 0
				for range 2 {
					r := <-results
					if r.err == nil && r.task != nil {
						created++
					} else if infraerrors.Reason(r.err) == "TOTP_RECOVERY_UNRESOLVED" {
						conflicts++
					} else {
						t.Errorf("unexpected rotation result: task=%v err=%v", r.task, r.err)
					}
				}
				require.Equal(t, 1, created)
				require.Equal(t, 1, conflicts)
			})
		}
	}
}

func TestTOTPRotationReimportCannotBypassUnresolvedIdentity(t *testing.T) {
	ctx, repo, original := totpRotationFixture(t)
	_, err := repo.CreateTOTPRotation(ctx, original.AccountID, "snapshot", original.UpdatedAt)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, original.AccountID)
	require.NoError(t, err)
	_, _, reimported := totpRotationFixture(t)
	reimported.LoginEmail = " OWNER@EXAMPLE.TEST "
	require.NoError(t, repo.UpsertConfig(ctx, reimported))
	reimported, err = repo.GetConfig(ctx, reimported.AccountID)
	require.NoError(t, err)
	for _, automatic := range []bool{false, true} {
		task, err := repo.createTOTPRotation(ctx, reimported.AccountID, "snapshot", reimported.UpdatedAt, automatic)
		require.Equal(t, "TOTP_RECOVERY_UNRESOLVED", infraerrors.Reason(err))
		require.Nil(t, task)
	}
	// The identity guard must not block an unrelated login on that account.
	reimported.LoginEmail = "another@example.test"
	require.NoError(t, repo.UpsertConfig(ctx, reimported))
	reimported, err = repo.GetConfig(ctx, reimported.AccountID)
	require.NoError(t, err)
	task, err := repo.CreateTOTPRotation(ctx, reimported.AccountID, "snapshot", reimported.UpdatedAt)
	require.NoError(t, err)
	require.NotNil(t, task)
}
