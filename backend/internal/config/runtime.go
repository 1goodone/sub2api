package config

import "fmt"

const (
	RuntimeRoleFull    = "full"
	RuntimeRoleGateway = "gateway"
)

// RuntimeConfig separates global background jobs from request replicas.
type RuntimeConfig struct {
	Role                    string `mapstructure:"role"`
	RegionalFundsFile       string `mapstructure:"regional_funds_file"`
	RegionalConcurrencyFile string `mapstructure:"regional_concurrency_file"`
	RegionalAuthFile        string `mapstructure:"regional_auth_file"`
	RegionalSchedulerFile   string `mapstructure:"regional_scheduler_file"`
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
