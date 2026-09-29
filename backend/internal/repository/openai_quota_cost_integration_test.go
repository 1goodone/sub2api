//go:build integration

package repository

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *AccountRepoSuite) TestUpdateExtra_OpenAIQuotaCostSamplesSurviveReset() {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	rate := 2.0
	account := mustCreateAccount(s.T(), s.client, &service.Account{
		Name: "quota-samples", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "Plus"}, RateMultiplier: &rate,
		Extra: map[string]any{"unrelated": "kept", "codex_usage_updated_at": now.Add(-time.Hour).Format(time.RFC3339)},
	})
	s.Require().NoError(s.client.Account.UpdateOneID(account.ID).SetRateMultiplier(rate).Exec(s.ctx))
	updates := map[string]any{
		"codex_5h_used_percent":  50.0,
		"codex_5h_reset_at":      now.Add(time.Hour).Format(time.RFC3339),
		"codex_usage_updated_at": now.Format(time.RFC3339),
	}
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))
	got, err := s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	sample := got.Extra["codex_5h_cost_sample"].(map[string]any)
	s.Require().Equal("plus", sample["plan_type"])
	s.Require().Equal(rate, sample["rate_multiplier"])
	s.Require().Equal(float64(now.Unix()), sample["sampled_at"])
	s.Require().Equal(now.Add(-time.Hour).Format(time.RFC3339), got.Extra["codex_7d_observed_at"])
	s.Require().NotContains(updates, "codex_5h_cost_sample", "do not mutate the caller's snapshot")
	for _, used := range []float64{0, 1} {
		updates["codex_5h_used_percent"] = used
		updates["codex_usage_updated_at"] = now.Add(time.Second).Format(time.RFC3339)
		s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))
		got, err = s.repo.GetByID(s.ctx, account.ID)
		s.Require().NoError(err)
		s.Require().Equal(sample, got.Extra["codex_5h_cost_sample"])
		s.Require().Equal("kept", got.Extra["unrelated"])
	}
	updates["codex_5h_used_percent"] = 80.0
	updates["codex_usage_updated_at"] = now.Add(-time.Second).Format(time.RFC3339)
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))
	got, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(sample, got.Extra["codex_5h_cost_sample"], "late snapshots cannot replace a newer sample")
	updates["codex_usage_updated_at"] = now.Add(2 * time.Second).Format(time.RFC3339)
	s.Require().NoError(s.repo.UpdateExtra(s.ctx, account.ID, updates))
	got, err = s.repo.GetByID(s.ctx, account.ID)
	s.Require().NoError(err)
	s.Require().Equal(80.0, got.Extra["codex_5h_cost_sample"].(map[string]any)["used_percent"])
}

func (s *UsageLogRepoSuite) TestOpenAIQuotaWindowStatsBatchBoundsAndBillingBasis() {
	now := time.Now().UTC().Truncate(time.Second)
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "quota-cost@test.com"})
	key := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-quota-cost", Name: "k"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "quota-cost"})
	other := mustCreateAccount(s.T(), s.client, &service.Account{Name: "other-quota-cost"})
	start, end := now.Add(-3*time.Hour), now.Add(-time.Hour)
	s.createUsageLog(user, key, account, 1, 1, 100, start.Add(-time.Second))
	s.createUsageLog(user, key, account, 1, 1, 2, start)
	s.createUsageLog(user, key, account, 1, 1, 3, end)
	s.createUsageLog(user, key, account, 1, 1, 200, end.Add(time.Second))
	s.createUsageLog(user, key, other, 1, 1, 500, end)
	bps := "/basispoints/api/responses"
	_, err := s.repo.Create(s.ctx, &service.UsageLog{UserID: user.ID, APIKeyID: key.ID,
		AccountID: account.ID, RequestID: "bps-quota-cost", Model: "gpt-5",
		TotalCost: 4, ActualCost: 4, UpstreamEndpoint: &bps, CreatedAt: end.Add(-time.Second)})
	s.Require().NoError(err)
	samples := map[int64]service.OpenAIQuotaCostSample{account.ID: {StartAt: start.Unix(), SampledAt: end.Unix(), RateMultiplier: 1}}
	stats, err := s.repo.GetOpenAIQuotaWindowStatsBatch(s.ctx, samples)
	s.Require().NoError(err)
	s.Require().Len(stats, 1)
	s.Require().Equal(int64(3), stats[account.ID].Requests)
	s.Require().InDelta(9, stats[account.ID].Cost, 1e-9, "include shared BPS quota, exclude later and other-account costs")
	sample := samples[account.ID]
	sample.RateMultiplier = 2
	samples[account.ID] = sample
	stats, err = s.repo.GetOpenAIQuotaWindowStatsBatch(s.ctx, samples)
	s.Require().NoError(err)
	s.Require().Zero(stats[account.ID].Cost, "do not estimate from a different billing basis")
}
