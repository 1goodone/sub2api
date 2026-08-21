package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupFocusPoolMigration(t *testing.T) {
	content, err := FS.ReadFile("230_group_focus_pool.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS is_focus BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "g.focus_account_id = ag.account_id")
	require.Contains(t, sql, "SET focus_account_id = NULL")
	require.Contains(t, sql, "WHERE is_focus = TRUE")
}
