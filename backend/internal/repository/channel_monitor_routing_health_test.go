package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestChannelMonitorUnavailableGroupIDsUsesFreshConsecutiveV1Signals(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo := &channelMonitorRepository{db: db}

	mock.ExpectQuery(`(?s)m\.check_mode IN \('probe', 'quota_probe'\).*h\.checked_at >=.*rn <= 2 AND failure_kind IN \('transport', 'upstream_5xx', 'response_invalid'\).*HAVING NOT BOOL_OR\(latest_success\) AND BOOL_OR\(consecutive_failure\)`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"group_id"}).AddRow(int64(2)))

	unavailable, err := repo.UnavailableGroupIDs(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, map[int64]bool{2: true}, unavailable)
	require.NoError(t, mock.ExpectationsWereMet())
}
