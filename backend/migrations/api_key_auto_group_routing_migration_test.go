package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyAutoGroupRoutingMigration(t *testing.T) {
	content, err := FS.ReadFile("231_api_key_auto_group_routing.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS mixed_mode_enabled BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS mixed_group_ids JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS group_id BIGINT")
	require.Contains(t, sql, "FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS status_code INTEGER")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS failure_kind VARCHAR(32) NOT NULL DEFAULT ''")
	require.Contains(t, sql, "OLD.mixed_mode_enabled IS DISTINCT FROM NEW.mixed_mode_enabled")
	require.Contains(t, sql, "OLD.mixed_group_ids IS DISTINCT FROM NEW.mixed_group_ids")
	require.Contains(t, sql, "k.mixed_group_ids @> to_jsonb(ARRAY[target_group_id])")
	require.Contains(t, sql, "CREATE OR REPLACE FUNCTION enqueue_allowed_group_auth_cache_invalidation()")
	require.Contains(t, sql, "k.mixed_group_ids @> to_jsonb(ARRAY[OLD.group_id])")
}
