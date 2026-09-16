package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsAsyncVideoTaskRead(t *testing.T) {
	for _, path := range []string{"/v1/videos/task", "/videos/task/content", "/v1/videos/task/download", "/v1/videos/operations/key"} {
		require.True(t, isAsyncVideoTaskRead(http.MethodGet, path))
		require.True(t, isAsyncVideoTaskRead(http.MethodHead, path))
		require.False(t, isAsyncVideoTaskRead(http.MethodPost, path))
	}
	for _, path := range []string{"/v1/videos", "/videos", "/v1/videos-other/task", "/v1/responses"} {
		require.False(t, isAsyncVideoTaskRead(http.MethodGet, path))
	}
}
