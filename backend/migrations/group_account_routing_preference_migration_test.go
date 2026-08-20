package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupAccountRoutingPreferenceMigration(t *testing.T) {
	content, err := FS.ReadFile("229_group_account_routing_preference.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS focus_account_id BIGINT NULL REFERENCES accounts(id) ON DELETE SET NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS account_type_preference VARCHAR NOT NULL DEFAULT 'default'")
	require.Contains(t, sql, "CHECK (account_type_preference IN ('default', 'auth_first', 'apikey_first'))")
}
