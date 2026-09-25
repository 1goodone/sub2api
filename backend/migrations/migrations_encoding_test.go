package migrations

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLMigrationsDoNotStartWithUTF8BOM(t *testing.T) {
	files, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)

	for _, name := range files {
		content, err := FS.ReadFile(name)
		require.NoError(t, err)
		require.False(t, bytes.HasPrefix(content, []byte{0xEF, 0xBB, 0xBF}), "%s starts with a UTF-8 BOM", name)
	}
}
