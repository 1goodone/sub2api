package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountOpsErrorCloseIgnoresQuotaAndUserErrors(t *testing.T) {
	require.Equal(t, "ignore", accountOpsErrorCloseDecision(PlatformOpenAI, 429, nil, []byte(`{"error":{"type":"insufficient_quota","message":"quota"}}`), nil))
	require.Equal(t, "ignore", accountOpsErrorCloseDecision(PlatformOpenAI, 400, nil, []byte(`{"error":{"type":"invalid_request_error","message":"bad request"}}`), nil))
	require.Equal(t, "ignore", accountOpsErrorCloseDecision(PlatformOpenAI, 502, nil, []byte(`{"error":{"type":"upstream_error","message":"Your input exceeds the context window"}}`), nil))
	require.Equal(t, "count", accountOpsErrorCloseDecision(PlatformOpenAI, 429, nil, []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`), nil))
	require.Equal(t, "count", accountOpsErrorCloseDecision(PlatformOpenAI, 0, nil, []byte(`{"error":{"type":"stream_timeout"}}`), nil))
	require.Equal(t, "count", accountOpsErrorCloseDecision(PlatformAnthropic, 409, nil, []byte(`{"error":{"type":"custom_break"}}`), []string{"custom_break"}))
	require.Equal(t, "reset", accountOpsErrorCloseDecision(PlatformOpenAI, 200, nil, nil, nil))
}

func TestAccountOpsErrorCloseConsecutiveSuccessResets(t *testing.T) {
	svc := NewAccountOpsService(nil, nil, nil)
	var closed string
	svc.errorClose = func(_ context.Context, id int64, message string) error {
		require.Equal(t, int64(7), id)
		closed = message
		return nil
	}
	svc.config.Store(AccountOpsConfig{ErrorCloseEnabled: true, ErrorCloseConsecutive: 2})
	account := &Account{ID: 7, Platform: PlatformOpenAI, Name: "acct"}
	body := []byte(`{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	svc.Observe(account, 400, nil, []byte(`{"error":{"type":"invalid_request_error","message":"bad request"}}`))
	svc.Observe(account, 429, nil, []byte(`{"error":{"type":"insufficient_quota","message":"quota"}}`))
	svc.Observe(account, 429, nil, body)
	svc.NoteSuccess(account.ID)
	svc.Observe(account, 429, nil, body)
	require.Empty(t, closed)
	svc.NoteStreamBreak(account)
	require.Contains(t, closed, "连续 2 次")
	require.NotContains(t, closed, "slow down")
}

func TestAccountOpsErrorCloseRetriesAfterCloseFailure(t *testing.T) {
	svc := NewAccountOpsService(nil, nil, nil)
	calls := 0
	svc.errorClose = func(context.Context, int64, string) error {
		calls++
		if calls == 1 {
			return errors.New("db down")
		}
		return nil
	}
	svc.config.Store(AccountOpsConfig{ErrorCloseEnabled: true, ErrorCloseConsecutive: 2})
	account := &Account{ID: 9, Platform: PlatformOpenAI}
	svc.NoteTransportFailure(account)
	svc.NoteTransportFailure(account)
	require.Equal(t, 1, calls)
	require.Equal(t, 2, svc.errorCloseStreak[9])
	svc.NoteSuccess(account.ID)
	svc.NoteTransportFailure(account)
	require.Equal(t, 1, calls)
	svc.NoteTransportFailure(account)
	require.Equal(t, 2, calls)
	require.NotContains(t, svc.errorCloseStreak, int64(9))
}

func TestAccountOpsErrorCloseSkipsAccountAlreadyClosed(t *testing.T) {
	svc := NewAccountOpsService(nil, nil, nil)
	called := false
	svc.errorClose = func(context.Context, int64, string) error {
		called = true
		return nil
	}
	svc.config.Store(AccountOpsConfig{ErrorCloseEnabled: true, ErrorCloseConsecutive: 2})
	account := &Account{ID: 8, Platform: PlatformOpenAI, Status: StatusError}
	svc.NoteStreamBreak(account)
	svc.NoteStreamBreak(account)
	require.False(t, called)
}
