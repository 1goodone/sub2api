package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type accountTokenGuardV2TestRepo struct {
	record     *AccountTokenGuardV2Record
	claimBusy  bool
	completion *AccountTokenGuardV2ProbeCompletion
}

func (r *accountTokenGuardV2TestRepo) UpsertAccount(_ context.Context, accountID int64, enabled, autoRelogin bool) error {
	if r.record == nil {
		r.record = &AccountTokenGuardV2Record{AccountID: accountID, NextProbeAt: time.Now()}
	}
	r.record.Enabled = enabled
	r.record.AutoReloginEnabled = autoRelogin
	return nil
}
func (r *accountTokenGuardV2TestRepo) DeleteAccount(context.Context, int64) error {
	r.record = nil
	return nil
}
func (r *accountTokenGuardV2TestRepo) GetAccount(context.Context, int64) (*AccountTokenGuardV2Record, error) {
	if r.record == nil {
		return nil, nil
	}
	copy := *r.record
	return &copy, nil
}
func (r *accountTokenGuardV2TestRepo) ListAccounts(context.Context) ([]AccountTokenGuardV2Record, error) {
	if r.record == nil {
		return nil, nil
	}
	return []AccountTokenGuardV2Record{*r.record}, nil
}
func (r *accountTokenGuardV2TestRepo) ClaimDue(context.Context, string, time.Duration, int) ([]AccountTokenGuardV2Record, error) {
	if r.record == nil || r.claimBusy {
		return nil, nil
	}
	return []AccountTokenGuardV2Record{*r.record}, nil
}
func (r *accountTokenGuardV2TestRepo) ClaimAccount(context.Context, int64, string, time.Duration) (*AccountTokenGuardV2Record, error) {
	if r.record == nil || r.claimBusy {
		return nil, nil
	}
	copy := *r.record
	return &copy, nil
}
func (r *accountTokenGuardV2TestRepo) CompleteProbe(_ context.Context, _ int64, _ string, result AccountTokenGuardV2ProbeCompletion) error {
	r.completion = &result
	if r.record != nil {
		r.record.ProbeState = result.ProbeState
		r.record.ProbeDetail = result.ProbeDetail
		r.record.FailStreak = result.FailStreak
		r.record.LastProbeAt = &result.LastProbeAt
		r.record.LastReauthAt = result.LastReauthAt
		r.record.NextProbeAt = result.NextProbeAt
		r.record.CooldownUntil = result.CooldownUntil
		r.record.BlockedReason = result.BlockedReason
	}
	return nil
}
func (r *accountTokenGuardV2TestRepo) MarkReauthRequested(context.Context, int64, time.Time, time.Time) error {
	return nil
}

type accountTokenGuardV2TestProber struct{ err error }

func (p accountTokenGuardV2TestProber) FetchOpenAIModelsList(context.Context, *Account) (*OpenAIModelsResponse, error) {
	return nil, p.err
}

func TestAccountTokenGuardV2SecondAuthFailureQueuesRelogin(t *testing.T) {
	reauth, reader, reauthRepo, _, _, _ := newReauthTestService("acct-1")
	require.NoError(t, reauthRepo.UpsertConfig(context.Background(), reauthEmailConfig(42, "user@example.com", "encrypted:https://mail.example.com/code")))
	guardRepo := &accountTokenGuardV2TestRepo{record: &AccountTokenGuardV2Record{
		AccountID: 42, Enabled: true, AutoReloginEnabled: true, FailStreak: 1, NextProbeAt: time.Now(),
	}}
	svc := NewAccountTokenGuardV2Service(guardRepo, reader, accountTokenGuardV2TestProber{err: &codexModelsManifestUpstreamError{
		err: errors.New("unauthorized"), statusCode: http.StatusUnauthorized,
	}}, reauth)
	record, err := guardRepo.GetAccount(context.Background(), 42)
	require.NoError(t, err)

	svc.runProbe(context.Background(), *record)

	require.NotNil(t, guardRepo.completion)
	require.Equal(t, AccountTokenGuardV2ProbeAuth, guardRepo.completion.ProbeState)
	require.Equal(t, 2, guardRepo.completion.FailStreak)
	require.NotNil(t, guardRepo.completion.LastReauthAt)
	require.NotNil(t, guardRepo.completion.CooldownUntil)
	require.Contains(t, guardRepo.completion.ProbeDetail, "automatic re-login queued")
	require.Equal(t, 1, reauthRepo.createCalls)
	require.NotNil(t, reauthRepo.task)
}

func TestAccountTokenGuardV2ActiveReloginTaskIsNotDuplicated(t *testing.T) {
	reauth, reader, reauthRepo, _, _, _ := newReauthTestService("acct-1")
	require.NoError(t, reauthRepo.UpsertConfig(context.Background(), reauthEmailConfig(42, "user@example.com", "encrypted:https://mail.example.com/code")))
	existing := &OpenAIOAuthReauthTaskRecord{ID: 99, AccountID: 42, Status: OpenAIOAuthReauthStatusRunning, Stage: OpenAIOAuthReauthStageStarting}
	reauthRepo.task = existing
	reauthRepo.createErr = infraerrors.Conflict("OPENAI_REAUTH_TASK_ACTIVE", "task already active")
	guardRepo := &accountTokenGuardV2TestRepo{record: &AccountTokenGuardV2Record{
		AccountID: 42, Enabled: true, AutoReloginEnabled: true, FailStreak: 1, NextProbeAt: time.Now(),
	}}
	svc := NewAccountTokenGuardV2Service(guardRepo, reader, accountTokenGuardV2TestProber{err: errors.New("invalid token")}, reauth)
	record, err := guardRepo.GetAccount(context.Background(), 42)
	require.NoError(t, err)

	svc.runProbe(context.Background(), *record)

	require.Same(t, existing, reauthRepo.task)
	require.Equal(t, 1, reauthRepo.createCalls)
	require.Contains(t, guardRepo.completion.ProbeDetail, "re-login task already active")
	require.Nil(t, guardRepo.completion.LastReauthAt)
}

func TestAccountTokenGuardV2ProbeNowDistinguishesMissingAndBusy(t *testing.T) {
	reauth, reader, _, _, _, _ := newReauthTestService("acct-1")
	prober := accountTokenGuardV2TestProber{}

	missing := NewAccountTokenGuardV2Service(&accountTokenGuardV2TestRepo{}, reader, prober, reauth)
	_, err := missing.ProbeNow(context.Background(), 42)
	require.Equal(t, http.StatusNotFound, infraerrors.Code(err))

	busyRepo := &accountTokenGuardV2TestRepo{
		record:    &AccountTokenGuardV2Record{AccountID: 42, NextProbeAt: time.Now()},
		claimBusy: true,
	}
	busy := NewAccountTokenGuardV2Service(busyRepo, reader, prober, reauth)
	_, err = busy.ProbeNow(context.Background(), 42)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
}
