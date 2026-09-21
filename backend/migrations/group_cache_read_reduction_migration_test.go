package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupCacheReadReductionMigration(t *testing.T) {
	content, err := FS.ReadFile("241_group_cache_read_reduction_percent.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS cache_read_reduction_percent DECIMAL(10,4) NOT NULL DEFAULT 0")
	require.Contains(t, sql, "CHECK (cache_read_reduction_percent BETWEEN 0 AND 100)")
	require.Contains(t, sql, "OLD.cache_read_reduction_percent IS NOT DISTINCT FROM NEW.cache_read_reduction_percent")
	require.Contains(t, sql, "k.mixed_group_ids @> to_jsonb(ARRAY[target_group_id])")
}
