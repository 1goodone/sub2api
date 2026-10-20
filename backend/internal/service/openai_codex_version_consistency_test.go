//go:build unit

package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexVersionConstants_Consistency(t *testing.T) {
	require.True(t, strings.Contains(codexCLIUserAgent, codexOutboundOriginator+"/"+codexCLIVersion),
		"codexCLIUserAgent must embed codexCLIVersion")

	require.True(t, strings.Contains(DefaultOpenAICodexUserAgent, codexCLIVersion),
		"DefaultOpenAICodexUserAgent must embed codexCLIVersion")
}
