package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupDisabledModelsMigration(t *testing.T) {
	content, err := FS.ReadFile("227_group_disabled_models.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS disabled_models JSONB NOT NULL DEFAULT '[]'::jsonb")
}
