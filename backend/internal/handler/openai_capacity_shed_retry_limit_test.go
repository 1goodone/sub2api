package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEffectiveSameAccountRetryLimitHonorsCapacityShedOverride(t *testing.T) {
	account := &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth}
	err := &service.UpstreamFailoverError{
		CapacityShedRetryOverride: true,
		SameAccountRetryMax:       5,
		RetryableOnSameAccount:    true,
	}

	require.Equal(t, 5, effectiveSameAccountRetryLimit(err, account))
	require.True(t, sameAccountRetryAllowed(err, 4, 5))
	require.False(t, sameAccountRetryAllowed(err, 5, 5))

	err.SameAccountRetryMax = 0
	require.Equal(t, 0, effectiveSameAccountRetryLimit(err, account))
	require.False(t, sameAccountRetryAllowed(err, 0, 0))
}
