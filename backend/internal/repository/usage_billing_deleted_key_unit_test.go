//go:build unit

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestUsageBillingAPIKeySettlementUpdatesRetainedRows(t *testing.T) {
	t.Run("quota", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		mock.ExpectQuery(`(?s)UPDATE api_keys.*WHEN deleted_at IS NULL AND quota > 0.*WHERE id = \$2\s+RETURNING deleted_at IS NULL`).
			WithArgs(1.25, int64(7), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
			WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))
		mock.ExpectRollback()

		exhausted, err := incrementUsageBillingAPIKeyQuota(t.Context(), tx, 7, 1.25)
		require.NoError(t, err)
		require.False(t, exhausted)
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("window", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer func() { _ = db.Close() }()

		mock.ExpectBegin()
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		mock.ExpectExec(`(?s)UPDATE api_keys SET.*WHERE id = \$2\s*$`).
			WithArgs(1.25, int64(7)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectRollback()

		require.NoError(t, incrementUsageBillingAPIKeyRateLimit(t.Context(), tx, 7, 1.25))
		require.NoError(t, tx.Rollback())
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// Including soft-deleted rows must not turn a genuinely missing row or a
// database failure into a successful, partially applied billing transaction.
func TestUsageBillingRepositoryApply_KeyUpdateFailureStillRollsBack(t *testing.T) {
	dbFailure := errors.New("database unavailable")
	for _, field := range []string{"quota", "window"} {
		for _, missing := range []bool{false, true} {
			name := field + "/database_error"
			if missing {
				name = field + "/missing_row"
			}
			t.Run(name, func(t *testing.T) {
				db, mock, err := sqlmock.New()
				require.NoError(t, err)
				defer func() { _ = db.Close() }()
				cmd := &service.UsageBillingCommand{
					RequestID: "failed-key-settlement", APIKeyID: 7, UserID: 42, BalanceCost: 1.25,
				}
				mock.ExpectBegin()
				mock.ExpectQuery("INSERT INTO usage_billing_dedup").
					WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				mock.ExpectQuery("SELECT request_fingerprint.*FROM usage_billing_dedup_archive").
					WillReturnRows(sqlmock.NewRows([]string{"request_fingerprint"}))
				mock.ExpectQuery(conditionalBalanceDeductSQL).
					WithArgs(1.25, int64(42)).
					WillReturnRows(sqlmock.NewRows([]string{"balance"}).AddRow(98.75))
				if field == "quota" {
					cmd.APIKeyQuotaCost = 1.25
					q := mock.ExpectQuery("UPDATE api_keys").
						WithArgs(1.25, int64(7), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted)
					if missing {
						q.WillReturnError(sql.ErrNoRows)
					} else {
						q.WillReturnError(dbFailure)
					}
				} else {
					cmd.APIKeyRateLimitCost = 1.25
					q := mock.ExpectExec("UPDATE api_keys").WithArgs(1.25, int64(7))
					if missing {
						q.WillReturnResult(sqlmock.NewResult(0, 0))
					} else {
						q.WillReturnError(dbFailure)
					}
				}
				mock.ExpectRollback()
				result, err := NewUsageBillingRepository(nil, db).Apply(context.Background(), cmd)
				require.Nil(t, result)
				wantErr := dbFailure
				if missing {
					wantErr = service.ErrAPIKeyNotFound
				}
				require.ErrorIs(t, err, wantErr)
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}
