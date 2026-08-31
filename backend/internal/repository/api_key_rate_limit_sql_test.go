package repository

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type captureRateLimitQueryMatcher struct{ actual *string }

func (m captureRateLimitQueryMatcher) Match(_, actual string) error {
	*m.actual = actual
	return nil
}

func assertRollingPackageWindows(t *testing.T, query string) {
	t.Helper()
	for _, window := range []string{"window_5h_start", "window_7d_start"} {
		var line string
		for _, candidate := range strings.Split(query, "\n") {
			if strings.Contains(candidate, window+" = CASE") {
				line = strings.TrimSpace(candidate)
				break
			}
		}
		require.NotEmpty(t, line, "%s assignment missing", window)
		require.Contains(t, line, "THEN NOW()", "%s must start from actual usage time", window)
		require.NotContains(t, line, "date_trunc", "%s must remain a rolling window", window)
	}
}

func TestAPIKeyRepositoryRateLimitSQLUsesRollingPackageWindows(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(context.Context, *apiKeyRepository) error
		args []driver.Value
	}{
		{
			name: "increment",
			run: func(ctx context.Context, repo *apiKeyRepository) error {
				return repo.IncrementRateLimitUsage(ctx, 7, 1.25)
			},
			args: []driver.Value{1.25, int64(7)},
		},
		{
			name: "reset",
			run:  func(ctx context.Context, repo *apiKeyRepository) error { return repo.ResetRateLimitWindows(ctx, 7) },
			args: []driver.Value{int64(7)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var query string
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureRateLimitQueryMatcher{actual: &query}))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			mock.ExpectExec("capture").WithArgs(test.args...).WillReturnResult(sqlmock.NewResult(0, 1))

			require.NoError(t, test.run(t.Context(), &apiKeyRepository{sql: db}))
			require.NoError(t, mock.ExpectationsWereMet())
			assertRollingPackageWindows(t, query)
		})
	}
}

func TestUsageBillingRateLimitSQLUsesRollingPackageWindows(t *testing.T) {
	var query string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureRateLimitQueryMatcher{actual: &query}))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	mock.ExpectBegin()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	mock.ExpectExec("capture").WithArgs(1.25, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	require.NoError(t, incrementUsageBillingAPIKeyRateLimit(t.Context(), tx, 7, 1.25))
	require.NoError(t, tx.Rollback())
	require.NoError(t, mock.ExpectationsWereMet())
	assertRollingPackageWindows(t, query)
}
