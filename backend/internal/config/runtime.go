package config

import "fmt"

const (
	RuntimeRoleFull    = "full"
	RuntimeRoleGateway = "gateway"
)

// RuntimeConfig separates global background jobs from request replicas.
type RuntimeConfig struct {
	NodeID                  string                   `mapstructure:"node_id"`
	ImageRelayOwnerOrigin   string                   `mapstructure:"image_relay_owner_origin"`
	WSNodeOwner             WSNodeOwnerConfig        `mapstructure:"ws_node_owner"`
	SchedulerReadiness      SchedulerReadinessConfig `mapstructure:"scheduler_readiness"`
	SchedulerSnapshotMode   string                   `mapstructure:"scheduler_snapshot_mode"`
	RegionalFundsFile       string                   `mapstructure:"regional_funds_file"`
	RegionalConcurrencyFile string                   `mapstructure:"regional_concurrency_file"`
	RegionalAuthFile        string                   `mapstructure:"regional_auth_file"`
	RegionalSchedulerFile   string                   `mapstructure:"regional_scheduler_file"`
	Role                    string                   `mapstructure:"role"`
}

func (c *Config) RunsBackgroundJobs() bool {
	return c == nil || c.Runtime.Role != RuntimeRoleGateway
}

func (c *Config) validateRuntime() error {
	if c.Runtime.NodeID != "" {
		if err := (WSNodeOwnerConfig{Enabled: true, NodeID: c.Runtime.NodeID}).Validate(); err != nil {
			return fmt.Errorf("runtime.node_id requires 1 to 64 lowercase identifier characters")
		}
		if c.Runtime.WSNodeOwner.Enabled && c.Runtime.WSNodeOwner.NodeID != c.Runtime.NodeID {
			return fmt.Errorf("runtime.node_id must match the enabled WS node owner identity")
		}
	}
	if err := ValidateImageRelayOwnerOrigin(c.Runtime.ImageRelayOwnerOrigin); err != nil {
		return err
	}
	if err := c.Runtime.WSNodeOwner.Validate(); err != nil {
		return err
	}
	if err := c.validateSchedulerReadiness(); err != nil {
		return err
	}
	if err := c.validateSchedulerSnapshotMode(); err != nil {
		return err
	}
	switch c.Runtime.Role {
	case "", RuntimeRoleFull, RuntimeRoleGateway:
		return nil
	default:
		return fmt.Errorf("runtime.role must be full or gateway")
	}
}
