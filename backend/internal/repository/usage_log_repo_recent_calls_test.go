package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type captureRecentCallsQueryMatcher struct{ actual *string }

func (m captureRecentCallsQueryMatcher) Match(_, actual string) error {
	*m.actual = actual
	return nil
}

func TestGetRecentAccountCallsBatch(t *testing.T) {
	var query string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureRecentCallsQueryMatcher{actual: &query}))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	older := time.Date(2026, time.September, 30, 9, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	mock.ExpectQuery("capture").
		WithArgs(sqlmock.AnyArg(), 10).
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "success", "created_at"}).
			AddRow(int64(3), false, older).
			AddRow(int64(3), true, newer))

	got, err := (&usageLogRepository{sql: db}).GetRecentAccountCallsBatch(t.Context(), []int64{3, 8}, 10)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, false, got[3][0].Success)
	require.Equal(t, newer, got[3][1].CreatedAt)
	require.Empty(t, got[8])

	normalized := strings.Join(strings.Fields(query), " ")
	require.Contains(t, normalized, "CROSS JOIN LATERAL")
	require.Contains(t, normalized, "DISTINCT ON (call_key)")
	require.Contains(t, normalized, "ORDER BY call_key, success ASC")
	require.Contains(t, normalized, "NOT COALESCE(e.is_count_tokens, FALSE)")
	require.Contains(t, normalized, "COALESCE(e.status_code, 0) >= 400 OR e.error_type = 'cyber_policy'")
	require.Contains(t, normalized, "ORDER BY account_id, created_at ASC")
}
