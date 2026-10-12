package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type accountTokenGuardV2TestRepo struct {
	record      *AccountTokenGuardV2Record
	claimBusy   bool
	completion  *AccountTokenGuardV2ProbeCompletion
	rescheduled bool
	pruneErr    error
	pruneOrphan bool
	claimed     bool
}

func (r *accountTokenGuardV2TestRepo) PruneDeletedAccounts(context.Context) (int64, error) {
	if r.pruneErr != nil {
		return 0, r.pruneErr
	}
	if r.pruneOrphan && r.record != nil {
		r.record = nil
		return 1, nil
	}
	return 0, nil
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
	r.claimed = true
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
func (r *accountTokenGuardV2TestRepo) RescheduleEnabled(context.Context) error {
	r.rescheduled = true
	return nil
}
func (r *accountTokenGuardV2TestRepo) RetireAccount(_ context.Context, accountID int64, detail string) error {
	if r.record == nil {
		r.record = &AccountTokenGuardV2Record{AccountID: accountID}
	}
	r.record.Enabled = false
	r.record.AutoReloginEnabled = false
	r.record.BlockedReason = "account_retired"
	r.record.ProbeDetail = detail
	return nil
}

type accountTokenGuardV2TestSettings struct{ raw string }

func (s *accountTokenGuardV2TestSettings) GetValue(_ context.Context, key string) (string, error) {
	if s.raw == "" || key == openAIOAuthReauthRuntimeSettingsKey {
		return "", ErrSettingNotFound
	}
	return s.raw, nil
}
func (s *accountTokenGuardV2TestSettings) Set(_ context.Context, _ string, value string) error {
	s.raw = value
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
	svc := NewAccountTokenGuardV2Service(guardRepo, nil, reader, accountTokenGuardV2TestProber{err: &codexModelsManifestUpstreamError{
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

func TestAccountTokenGuardV2RateLimitNeverQueuesRelogin(t *testing.T) {
	for _, message := range []string{"too many requests", "unauthorized: invalid_token", "token expired; requires re-login"} {
		t.Run(message, func(t *testing.T) {
			reauth, reader, reauthRepo, _, _, _ := newReauthTestService("acct-1")
			require.NoError(t, reauthRepo.UpsertConfig(context.Background(), reauthEmailConfig(42, "user@example.com", "encrypted:https://mail.example.com/code")))
			record := AccountTokenGuardV2Record{AccountID: 42, Enabled: true, AutoReloginEnabled: true, FailStreak: 1}
			repo := &accountTokenGuardV2TestRepo{record: &record}
			svc := NewAccountTokenGuardV2Service(repo, nil, reader, accountTokenGuardV2TestProber{err: &codexModelsManifestUpstreamError{
				err: errors.New(message), statusCode: http.StatusTooManyRequests,
			}}, reauth)

			svc.runProbe(context.Background(), record)

			require.Equal(t, AccountTokenGuardV2ProbeTransient, repo.completion.ProbeState)
			require.Equal(t, 1, repo.completion.FailStreak)
			require.Zero(t, reauthRepo.createCalls)
			require.Nil(t, repo.completion.LastReauthAt)
			require.Equal(t, repo.completion.LastProbeAt.Add(AccountTokenGuardV2ProbeInterval), repo.completion.NextProbeAt)
		})
	}
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
	svc := NewAccountTokenGuardV2Service(guardRepo, nil, reader, accountTokenGuardV2TestProber{err: errors.New("invalid token")}, reauth)
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

	missing := NewAccountTokenGuardV2Service(&accountTokenGuardV2TestRepo{}, nil, reader, prober, reauth)
	_, err := missing.ProbeNow(context.Background(), 42)
	require.Equal(t, http.StatusNotFound, infraerrors.Code(err))

	busyRepo := &accountTokenGuardV2TestRepo{
		record:    &AccountTokenGuardV2Record{AccountID: 42, NextProbeAt: time.Now()},
		claimBusy: true,
	}
	busy := NewAccountTokenGuardV2Service(busyRepo, nil, reader, prober, reauth)
	_, err = busy.ProbeNow(context.Background(), 42)
	require.Equal(t, http.StatusConflict, infraerrors.Code(err))
}

func TestAccountTokenGuardV2RulesDefaultAndPersistence(t *testing.T) {
	repo := &accountTokenGuardV2TestRepo{}
	settings := &accountTokenGuardV2TestSettings{}
	svc := NewAccountTokenGuardV2Service(repo, settings, nil, nil, nil)

	rules, err := svc.GetRules(context.Background())
	require.NoError(t, err)
	require.Equal(t, defaultAccountTokenGuardV2Rules(), rules)

	want := AccountTokenGuardV2Rules{
		ProbeIntervalSeconds: 900, RetryIntervalSeconds: 60,
		ReloginCooldownSeconds: 7200, FailStreakThreshold: 3,
	}
	saved, err := svc.SaveRules(context.Background(), want)
	require.NoError(t, err)
	require.Equal(t, want, saved)
	require.True(t, repo.rescheduled)

	var persisted AccountTokenGuardV2Rules
	require.NoError(t, json.Unmarshal([]byte(settings.raw), &persisted))
	require.Equal(t, want, persisted)
	reread, err := svc.GetRules(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, reread)
}

func TestAccountTokenGuardV2RulesRejectInvalidRange(t *testing.T) {
	repo := &accountTokenGuardV2TestRepo{}
	settings := &accountTokenGuardV2TestSettings{}
	svc := NewAccountTokenGuardV2Service(repo, settings, nil, nil, nil)

	_, err := svc.SaveRules(context.Background(), AccountTokenGuardV2Rules{
		ProbeIntervalSeconds: 59, RetryIntervalSeconds: 60,
		ReloginCooldownSeconds: 1800, FailStreakThreshold: 2,
	})
	require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	require.Empty(t, settings.raw)
	require.False(t, repo.rescheduled)
}

func TestAccountTokenGuardV2CustomRulesDriveProbe(t *testing.T) {
	reauth, reader, reauthRepo, _, _, _ := newReauthTestService("acct-1")
	require.NoError(t, reauthRepo.UpsertConfig(context.Background(), reauthEmailConfig(42, "user@example.com", "encrypted:https://mail.example.com/code")))
	guardRepo := &accountTokenGuardV2TestRepo{record: &AccountTokenGuardV2Record{
		AccountID: 42, Enabled: true, AutoReloginEnabled: true, NextProbeAt: time.Now(),
	}}
	settings := &accountTokenGuardV2TestSettings{}
	svc := NewAccountTokenGuardV2Service(guardRepo, settings, reader, accountTokenGuardV2TestProber{err: errors.New("invalid token")}, reauth)
	_, err := svc.SaveRules(context.Background(), AccountTokenGuardV2Rules{
		ProbeIntervalSeconds: 600, RetryIntervalSeconds: 45,
		ReloginCooldownSeconds: 7200, FailStreakThreshold: 1,
	})
	require.NoError(t, err)
	record, err := guardRepo.GetAccount(context.Background(), 42)
	require.NoError(t, err)

	svc.runProbe(context.Background(), *record)

	require.Equal(t, 1, reauthRepo.createCalls)
	require.WithinDuration(t, guardRepo.completion.LastProbeAt.Add(45*time.Second), guardRepo.completion.NextProbeAt, time.Second)
	require.NotNil(t, guardRepo.completion.CooldownUntil)
	require.WithinDuration(t, guardRepo.completion.LastProbeAt.Add(2*time.Hour), *guardRepo.completion.CooldownUntil, time.Second)
}

func TestAccountTokenGuardV2RunDuePrunesBeforeClaim(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			repo := &accountTokenGuardV2TestRepo{
				record:      &AccountTokenGuardV2Record{AccountID: 42, Enabled: enabled, NextProbeAt: time.Now().Add(time.Hour)},
				pruneOrphan: true,
			}
			// No account reader/prober: orphaned records must never reach probing.
			svc := NewAccountTokenGuardV2Service(repo, nil, nil, nil, nil)
			n, err := svc.RunDue(context.Background())
			require.NoError(t, err)
			require.Zero(t, n)
			require.Nil(t, repo.record)
			require.True(t, repo.claimed)
			n, err = svc.RunDue(context.Background())
			require.NoError(t, err)
			require.Zero(t, n, "cleanup must be idempotent")
		})
	}
}

func TestRetiredAccountReasonAndMarker(t *testing.T) {
	require.True(t, isRetiredAccountReason("WorkerError: account_retired: You do not have an account because it has been deleted or deactivated"))
	require.True(t, isRetiredAccountReason("[Codex] 账号已废（account_deactivated）"))
	require.False(t, isRetiredAccountReason("password/TOTP protocol failed (exit 1)"))
	repo := &accountTokenGuardV2TestRepo{record: &AccountTokenGuardV2Record{AccountID: 32, Enabled: true, AutoReloginEnabled: true}}
	svc := NewAccountTokenGuardV2Service(repo, nil, nil, nil, nil)
	require.NoError(t, svc.MarkAccountRetired(context.Background(), 32, retiredAccountDetail))
	require.False(t, repo.record.Enabled)
	require.False(t, repo.record.AutoReloginEnabled)
	require.Equal(t, "account_retired", repo.record.BlockedReason)
	require.Contains(t, repo.record.ProbeDetail, "deleted or deactivated")
}

func TestAccountTokenGuardV2RunDuePruneFailurePreservesRecords(t *testing.T) {
	repo := &accountTokenGuardV2TestRepo{
		record:   &AccountTokenGuardV2Record{AccountID: 42},
		pruneErr: errors.New("database unavailable"),
	}
	svc := NewAccountTokenGuardV2Service(repo, nil, nil, nil, nil)
	n, err := svc.RunDue(context.Background())
	require.ErrorIs(t, err, repo.pruneErr)
	require.Zero(t, n)
	require.NotNil(t, repo.record)
	require.False(t, repo.claimed)
}
