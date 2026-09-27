package service

import (
	"encoding/json"
	"os"
	"testing"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCPACompatibleTransportManifestSupportsSub2API021(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/cpa-compatible-transport-plugin/manifest.source.json")
	require.NoError(t, err)

	var manifest PluginManifest
	require.NoError(t, json.Unmarshal(raw, &manifest))

	result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "0.2.1", BuildType: "release"})
	require.True(t, result.Compatible)
	assert.True(t, result.Tested)
	assert.Equal(t, "compatible", result.Status)
	assert.Equal(t, "0.1.2", manifest.Version)

	futureZeroMajor := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "0.9.9", BuildType: "release"})
	require.True(t, futureZeroMajor.Compatible)
	assert.False(t, futureZeroMajor.Tested)
}

func TestCodexRSTransportManifestSupportsSub2API2819(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/codex-rs-transport-plugin/manifest.source.json")
	require.NoError(t, err)

	var manifest PluginManifest
	require.NoError(t, json.Unmarshal(raw, &manifest))

	result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "2.8.19", BuildType: "release"})
	require.True(t, result.Compatible)
	assert.True(t, result.Tested)
	assert.Equal(t, "compatible", result.Status)
	assert.Equal(t, "0.1.2", manifest.Version)
}

func TestEvaluatePluginCompatibility(t *testing.T) {
	manifest := testPluginManifest(nil)
	host := PluginHostInfo{Version: "0.1.179", BuildType: "release"}

	result := EvaluatePluginCompatibility(manifest, host)
	require.True(t, result.Compatible)
	assert.True(t, result.Tested)
	assert.Equal(t, "compatible", result.Status)

	manifest.Requires.TestedSub2APIVersions = []string{"0.1.178"}
	result = EvaluatePluginCompatibility(manifest, host)
	require.True(t, result.Compatible)
	assert.False(t, result.Tested)
	assert.Equal(t, "untested", result.Status)

	manifest.Requires.Sub2API = ">=0.2.0 <0.3.0"
	result = EvaluatePluginCompatibility(manifest, host)
	assert.False(t, result.Compatible)
	assert.Equal(t, "incompatible", result.Status)
}

func TestEvaluatePluginCompatibilityRejectsProtocolMismatch(t *testing.T) {
	manifest := testPluginManifest(nil)
	manifest.Requires.PluginProtocol = pluginv1.ProtocolVersion + 1

	result := EvaluatePluginCompatibility(manifest, PluginHostInfo{Version: "0.1.179"})

	assert.False(t, result.Compatible)
	assert.Equal(t, "incompatible", result.Status)
}

func TestMatchesSemverRange(t *testing.T) {
	assert.True(t, matchesSemverRange("0.1.179", ">=0.1.170, <0.2.0"))
	assert.True(t, matchesSemverRange("v1.2.3", "=1.2.3"))
	assert.False(t, matchesSemverRange("0.1.169", ">=0.1.170 <0.2.0"))
	assert.False(t, matchesSemverRange("dev", ">=0.1.0"))
	assert.False(t, matchesSemverRange("0.1.179", "^0.1.0"))
}
