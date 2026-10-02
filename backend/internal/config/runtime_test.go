package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeRolesPreserveDefaultsAndRejectTypos(t *testing.T) {
	require.True(t, (*Config)(nil).RunsBackgroundJobs())
	for _, role := range []string{"", RuntimeRoleFull, RuntimeRoleGateway} {
		cfg := &Config{Runtime: RuntimeConfig{Role: role}}
		require.NoError(t, cfg.validateRuntime())
		require.Equal(t, role != RuntimeRoleGateway, cfg.RunsBackgroundJobs())
	}
	require.ErrorContains(t, (&Config{Runtime: RuntimeConfig{Role: "gateawy"}}).validateRuntime(), "runtime.role")
}

func TestRuntimeRoleEnvironmentOverride(t *testing.T) {
	resetViperWithJWTSecret(t)
	t.Setenv("RUNTIME_ROLE", RuntimeRoleGateway)
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, RuntimeRoleGateway, cfg.Runtime.Role)
	require.False(t, cfg.RunsBackgroundJobs())
}
