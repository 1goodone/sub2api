package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type startupCleanupCache struct {
	ConcurrencyCache
	prefixes []string
}

func (c *startupCleanupCache) CleanupStaleProcessSlots(_ context.Context, prefix string) error {
	c.prefixes = append(c.prefixes, prefix)
	return nil
}

func TestConcurrencyStartupPreservesOtherProcessSlotsDuringRollingDeployment(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		cache := &startupCleanupCache{}
		cfg := &config.Config{}
		cfg.Gateway.Scheduling.PreserveProcessSlotsOnStartup = preserve
		ProvideConcurrencyService(cache, nil, cfg)
		if preserve {
			require.Empty(t, cache.prefixes, "a new instance must not remove live slots owned by the draining instance")
		} else {
			require.Equal(t, []string{RequestIDPrefix()}, cache.prefixes)
		}
	}
}
