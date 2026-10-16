package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var candyNow = time.Date(2026, 9, 29, 3, 0, 0, 0, time.UTC)

type candyRepoFake struct {
	channelMonitorV2RepoStub
	mu         sync.Mutex
	claims     map[string]int64
	results    []ChannelMonitorV2CandyResult
	history    []ChannelMonitorV2CandyResult
	historyIDs []int64
	prunes     int
}

func (r *candyRepoFake) ClaimCandyProbe(_ context.Context, p ChannelMonitorV2CandyProbe, key string, slot time.Time, version int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	claimKey := fmt.Sprintf("%d\x00%s\x00%d", p.GroupID, key, slot.Unix())
	if r.claims == nil {
		r.claims = map[string]int64{}
	}
	if _, ok := r.claims[claimKey]; ok || version != r.config.Version {
		return 0, nil
	}
	id := int64(len(r.claims) + 1)
	r.claims[claimKey] = id
	return id, nil
}
func (r *candyRepoFake) FinishCandyProbe(_ context.Context, result ChannelMonitorV2CandyResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
	return nil
}
func (r *candyRepoFake) CandyHistory(_ context.Context, ids []int64, _ time.Time) ([]ChannelMonitorV2CandyResult, error) {
	r.historyIDs = ids
	return r.history, nil
}
func (r *candyRepoFake) PruneCandyHistory(context.Context, time.Time) error { r.prunes++; return nil }

type candyRunnerFake struct {
	groups map[int64]*Group
	run    func(context.Context, *Group, ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error)
}

func (r *candyRunnerFake) GetGroup(_ context.Context, id int64) (*Group, error) {
	return r.groups[id], nil
}
func (r *candyRunnerFake) Run(ctx context.Context, group *Group, probe ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
	return r.run(ctx, group, probe)
}

func candyProbeFixture() ChannelMonitorV2CandyProbe {
	return ChannelMonitorV2CandyProbe{GroupID: 4, Enabled: true, Model: "gpt-test", ReasoningEffort: "medium", IntervalMinutes: 1}
}
func candyConfigFixture() ChannelMonitorV2Config {
	return ChannelMonitorV2Config{Version: 1, Enabled: true, Platforms: []ChannelMonitorV2PlatformConfig{{Platform: PlatformOpenAI, Enabled: true}}, CandyProbes: []ChannelMonitorV2CandyProbe{candyProbeFixture()}}
}
func candyServiceFixture(repo *candyRepoFake, runner channelMonitorV2CandyRunner) *ChannelMonitorV2CandyService {
	s := newChannelMonitorV2CandyService(repo, runner, channelMonitorV2RuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV2}})
	s.now = func() time.Time { return candyNow }
	return s
}

func TestChannelMonitorV2CandyVerdictsAndNoCherryPicking(t *testing.T) {
	for _, tc := range []struct{ name, output, status, message, want string }{
		{"unit answer", "21个", "success", "", "correct"},
		{"wrong", "22", "failed", "answer_mismatch: expected 21", "incorrect"},
		{"substring", "121", "success", "", "incorrect"},
		{"partial failed stream", "21", "failed", "network error", "error"},
		{"empty", "", "success", "", "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &candyRepoFake{channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: candyConfigFixture()}}
			calls := 0
			runner := &candyRunnerFake{groups: map[int64]*Group{4: {ID: 4, Platform: PlatformOpenAI, Status: StatusActive}}, run: func(_ context.Context, group *Group, probe ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
				calls++
				require.Equal(t, int64(4), group.ID)
				require.Equal(t, "gpt-test", probe.Model)
				return &ScheduledTestResult{ResponseText: tc.output, Status: tc.status, ErrorMessage: tc.message}, nil
			}}
			s := candyServiceFixture(repo, runner)
			s.RunDue(context.Background(), candyNow)
			require.Len(t, repo.results, 1)
			require.Equal(t, tc.want, repo.results[0].Verdict)
			require.Equal(t, 1, calls, "a wrong answer is the sampled result, not a reason to try another account")
			s.RunDue(context.Background(), candyNow)
			require.Len(t, repo.results, 1, "same slot must not run twice")
		})
	}
}

func TestChannelMonitorV2CandyGatesAndValidation(t *testing.T) {
	newRunner := func() *candyRunnerFake {
		return &candyRunnerFake{groups: map[int64]*Group{4: {ID: 4, Platform: PlatformOpenAI, Status: StatusActive}}, run: func(context.Context, *Group, ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
			return &ScheduledTestResult{Status: "success", ResponseText: "21"}, nil
		}}
	}
	for _, mutation := range []func(*ChannelMonitorV2CandyService, *candyRepoFake){
		func(s *ChannelMonitorV2CandyService, _ *candyRepoFake) { s.settings = nil },
		func(s *ChannelMonitorV2CandyService, _ *candyRepoFake) {
			s.settings = channelMonitorV2RuntimeStub{rt: ChannelMonitorRuntime{Enabled: true, Mode: ChannelMonitorModeV1}}
		},
		func(_ *ChannelMonitorV2CandyService, r *candyRepoFake) { r.config.Enabled = false },
		func(_ *ChannelMonitorV2CandyService, r *candyRepoFake) { r.config.CandyProbes[0].Enabled = false },
		func(_ *ChannelMonitorV2CandyService, r *candyRepoFake) { r.config.GroupIDs = []int64{99} },
		func(_ *ChannelMonitorV2CandyService, r *candyRepoFake) { r.config.Platforms[0].Enabled = false },
	} {
		repo := &candyRepoFake{channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: candyConfigFixture()}}
		s := candyServiceFixture(repo, newRunner())
		mutation(s, repo)
		s.RunDue(context.Background(), candyNow)
		require.Empty(t, repo.claims)
		require.Equal(t, 1, repo.prunes)
	}
	for _, mutation := range []func(*ChannelMonitorV2CandyProbe){
		func(p *ChannelMonitorV2CandyProbe) { p.GroupID = 0 },
		func(p *ChannelMonitorV2CandyProbe) { p.Model = " " },
		func(p *ChannelMonitorV2CandyProbe) { p.IntervalMinutes = -1 },
		func(p *ChannelMonitorV2CandyProbe) { p.IntervalMinutes = 1441 },
		func(p *ChannelMonitorV2CandyProbe) { p.ReasoningEffort = "invalid" },
	} {
		p := candyProbeFixture()
		mutation(&p)
		require.ErrorIs(t, normalizeChannelMonitorV2CandyProbes([]ChannelMonitorV2CandyProbe{p}), ErrChannelMonitorV2InvalidConfig)
	}
	require.Error(t, normalizeChannelMonitorV2CandyProbes([]ChannelMonitorV2CandyProbe{candyProbeFixture(), candyProbeFixture()}))
	require.Error(t, normalizeChannelMonitorV2CandyProbes(make([]ChannelMonitorV2CandyProbe, 65)))
	second := candyProbeFixture()
	second.Model = "gpt-other"
	require.NoError(t, normalizeChannelMonitorV2CandyProbes([]ChannelMonitorV2CandyProbe{candyProbeFixture(), second}))
	otherEffort := second
	otherEffort.ReasoningEffort = "high"
	require.NoError(t, normalizeChannelMonitorV2CandyProbes([]ChannelMonitorV2CandyProbe{second, otherEffort}))
	sameEffort := second
	sameEffort.IntervalMinutes = 5
	require.Error(t, normalizeChannelMonitorV2CandyProbes([]ChannelMonitorV2CandyProbe{second, sameEffort}))
	for _, limit := range []int{14, 101} {
		cfg := ChannelMonitorV2Config{RefreshIntervalSeconds: 60, CandyHistoryLimit: limit}
		require.ErrorIs(t, normalizeChannelMonitorV2Config(&cfg), ErrChannelMonitorV2InvalidConfig)
	}
	cfg := ChannelMonitorV2Config{RefreshIntervalSeconds: 60, CandyHistoryLimit: 15}
	require.NoError(t, normalizeChannelMonitorV2Config(&cfg))
	require.Equal(t, 15, cfg.CandyHistoryLimit)
	cfg.CandyHistoryLimit = 30
	require.NoError(t, normalizeChannelMonitorV2Config(&cfg))
	require.Equal(t, 30, cfg.CandyHistoryLimit)
	cfg.CandyHistoryLimit = ChannelMonitorV2CandyHistoryMax
	require.NoError(t, normalizeChannelMonitorV2Config(&cfg))
	require.Equal(t, ChannelMonitorV2CandyHistoryMax, cfg.CandyHistoryLimit)
	cfg.CandyHistoryLimit = 0
	require.NoError(t, normalizeChannelMonitorV2Config(&cfg))
	require.Equal(t, ChannelMonitorV2CandyHistoryDefault, cfg.CandyHistoryLimit)
}

func TestChannelMonitorV2CandyHistoryScopeAndRedaction(t *testing.T) {
	p := candyProbeFixture()
	second := p
	second.Model = "gpt-other"
	other := p
	other.GroupID = 99
	repo := &candyRepoFake{history: []ChannelMonitorV2CandyResult{
		{GroupID: 4, ConfigKey: p.key(), Verdict: "correct", AnswerPreview: "private answer", Reason: "private reason"},
		{GroupID: 4, ConfigKey: second.key(), Verdict: "incorrect", AnswerPreview: "second private", Reason: "second reason"},
		{GroupID: 4, ConfigKey: "obsolete model", Verdict: "incorrect"},
		{GroupID: 99, ConfigKey: other.key(), Verdict: "incorrect", AnswerPreview: "other group"},
	}}
	runner := &candyRunnerFake{groups: map[int64]*Group{}, run: func(context.Context, *Group, ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
		return nil, nil
	}}
	s := candyServiceFixture(repo, runner)
	cfg := candyConfigFixture()
	cfg.CandyHistoryLimit = 40
	cfg.CandyProbes = []ChannelMonitorV2CandyProbe{p, second, other}
	id := int64(4)
	matrix := &ChannelMonitorV2Matrix{Items: []ChannelMonitorV2MatrixRow{{GroupID: &id}}}
	require.NoError(t, s.attachHistory(context.Background(), matrix, &cfg, false))
	require.Equal(t, []int64{4}, repo.historyIDs)
	require.Len(t, matrix.Items[0].Candy.Results, 1)
	require.Len(t, matrix.Items[0].CandyChecks, 2)
	require.Equal(t, "gpt-other", matrix.Items[0].CandyChecks[1].Model)
	require.Equal(t, 40, matrix.Items[0].CandyChecks[1].HistoryLimit)
	require.Empty(t, matrix.Items[0].CandyChecks[1].Results[0].AnswerPreview)
	encoded, err := json.Marshal(matrix)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private")
	require.NotContains(t, string(encoded), "other group")
	require.NoError(t, s.attachHistory(context.Background(), matrix, &cfg, true))
	require.Equal(t, "private answer", matrix.Items[0].Candy.Results[0].AnswerPreview)
	require.Equal(t, "second private", matrix.Items[0].CandyChecks[1].Results[0].AnswerPreview)
	redactChannelMonitorV2PublicConfig(&cfg)
	require.Nil(t, cfg.CandyProbes)
}

func TestChannelMonitorV2CandyBoundsGroupConcurrency(t *testing.T) {
	repo := &candyRepoFake{channelMonitorV2RepoStub: channelMonitorV2RepoStub{config: candyConfigFixture()}}
	repo.config.CandyProbes = nil
	groups := map[int64]*Group{}
	for i := int64(1); i <= 12; i++ {
		p := candyProbeFixture()
		p.GroupID = i
		repo.config.CandyProbes = append(repo.config.CandyProbes, p)
		groups[i] = &Group{ID: i, Platform: PlatformOpenAI, Status: StatusActive}
	}
	var active, maxActive atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	runner := &candyRunnerFake{groups: groups, run: func(ctx context.Context, _ *Group, _ ChannelMonitorV2CandyProbe) (*ScheduledTestResult, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maxActive.Load(); n > old; old = maxActive.Load() {
			if maxActive.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return &ScheduledTestResult{Status: "success", ResponseText: "21"}, nil
	}}
	s := candyServiceFixture(repo, runner)
	done := make(chan struct{})
	go func() { defer close(done); s.RunDue(context.Background(), candyNow) }()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("probe workers did not start")
		}
	}
	require.Equal(t, int32(4), maxActive.Load())
	close(release)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("probes did not finish")
	}
	require.Len(t, repo.results, 12)
	require.LessOrEqual(t, maxActive.Load(), int32(4))
}

func TestChannelMonitorV2CandyFailoverOnlyBeforeOutput(t *testing.T) {
	require.True(t, channelMonitorV2CandyFailedBeforeOutput(&ScheduledTestResult{Status: "failed", ErrorMessage: "upstream unavailable"}))
	require.False(t, channelMonitorV2CandyFailedBeforeOutput(&ScheduledTestResult{Status: "failed", ResponseText: "22", ErrorMessage: "answer_mismatch: expected 21"}))
	require.False(t, channelMonitorV2CandyFailedBeforeOutput(&ScheduledTestResult{Status: "failed", ErrorMessage: "Model returned empty output"}))
}
