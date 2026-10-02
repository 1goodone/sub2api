package config

import "fmt"

const (
	RuntimeRoleFull    = "full"
	RuntimeRoleGateway = "gateway"
)

// RuntimeConfig separates global background jobs from request replicas.
type RuntimeConfig struct {
	Role string `mapstructure:"role"`
}

func (c *Config) RunsBackgroundJobs() bool {
	return c == nil || c.Runtime.Role != RuntimeRoleGateway
}

func (c *Config) validateRuntime() error {
	switch c.Runtime.Role {
	case "", RuntimeRoleFull, RuntimeRoleGateway:
		return nil
	default:
		return fmt.Errorf("runtime.role must be full or gateway")
	}
}
