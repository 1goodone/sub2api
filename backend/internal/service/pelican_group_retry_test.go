package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Local policy retries every failed buffered group-test result, including model-answer
// failures. These tests cover that policy plus the upstream attempt cap.

var groupTestNow = time.Date(2026, 9, 28, 4, 0, 0, 0, time.UTC)

type groupTestRetryRepo struct {
	results []*PelicanGroupTestResult
	nextID  int64
}

func (r *groupTestRetryRepo) ListPlans(context.Context) ([]*PelicanGroupTestPlan, error) {
	return nil, nil
}
func (r *groupTestRetryRepo) GetPlan(context.Context, int64) (*PelicanGroupTestPlan, error) {
	return nil, nil
}
func (r *groupTestRetryRepo) CreatePlan(context.Context, *PelicanGroupTestPlan) (*PelicanGroupTestPlan, error) {
	return nil, nil
}
func (r *groupTestRetryRepo) UpdatePlan(context.Context, *PelicanGroupTestPlan) (*PelicanGroupTestPlan, error) {
	return nil, nil
}
func (r *groupTestRetryRepo) DeletePlan(context.Context, int64) (bool, error) { return false, nil }
func (r *groupTestRetryRepo) ListDue(context.Context, time.Time) ([]*PelicanGroupTestPlan, error) {
	return nil, nil
}
func (r *groupTestRetryRepo) Claim(context.Context, *PelicanGroupTestPlan, time.Time, time.Time, *time.Time) (bool, error) {
	return false, nil
}
func (r *groupTestRetryRepo) Finish(context.Context, int64, time.Time, time.Time) error { return nil }
func (r *groupTestRetryRepo) CreateResult(_ context.Context, result *PelicanGroupTestResult) (*PelicanGroupTestResult, error) {
	saved := *result
	r.nextID++
	saved.ID = r.nextID
	r.results = append(r.results, &saved)
	return &saved, nil
}
func (r *groupTestRetryRepo) PruneResults(context.Context, int64, int) error { return nil }
func (r *groupTestRetryRepo) PruneExpiredResults(context.Context, time.Time) error {
	return nil
}
func (r *groupTestRetryRepo) ListResults(context.Context, int64, int, int) ([]*PelicanGroupTestResult, int64, error) {
	return nil, 0, nil
}
func (r *groupTestRetryRepo) GetResult(context.Context, int64) (*PelicanGroupTestResult, error) {
	return nil, nil
}

func groupTestPlan(parallel int) *PelicanGroupTestPlan {
	return &PelicanGroupTestPlan{ID: 7, GroupID: 4, ModelID: "gpt-6-astra", CronExpression: "*/30 * * * *", Enabled: true,
		PelicanConfig: &PelicanTestConfig{QuestionKind: "pelican", Prompt: "draw", ReasoningEffort: "high", ParallelCount: parallel, ModelID: "gpt-6-astra"}}
}

func newGroupTestService(repo *groupTestRetryRepo, router pelicanGroupRouter, run func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error)) *PelicanGroupTestService {
	return &PelicanGroupTestService{
		repo:       repo,
		groups:     groupTestGroupsFake{4: {ID: 4, Name: "GPT PRO", Platform: PlatformOpenAI, Status: StatusActive}},
		router:     router,
		runAccount: run,
		now:        func() time.Time { return groupTestNow },
	}
}

func answers(byAccount map[int64]ScheduledTestResult) func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
	return func(_ context.Context, accountID int64, _ string, _ *PelicanTestConfig) (*ScheduledTestResult, error) {
		result := byAccount[accountID]
		return &result, nil
	}
}

func TestPelicanGroupRetriesPartialTransientFailures(t *testing.T) {
	for _, message := range []string{
		"The service is temporarily unavailable. Please retry later.",
		"Service temporarily unavailable",
		"Stream read error: unexpected EOF",
		"Stream ended before response.completed",
		"Generation stream ended before completion",
		"Chat Completions stream from /v1/chat/completions ended before [DONE]",
		"Chat Completions stream read error from /v1/chat/completions: connection reset by peer",
	} {
		t.Run(message, func(t *testing.T) {
			repo := &groupTestRetryRepo{}
			router := &groupTestRouterFake{steps: []routeStep{{account: account(11, "a")}, {account: account(12, "b")}}}
			svc := newGroupTestService(repo, router, answers(map[int64]ScheduledTestResult{
				11: {Status: "failed", ResponseText: "<html>incomplete", ErrorMessage: message},
				12: {Status: "success", ResponseText: "<svg>complete</svg>"},
			}))
			svc.execute(groupTestPlan(1), groupTestNow)
			require.Len(t, repo.results, 1)
			result := repo.results[0]
			require.Equal(t, "success", result.Status)
			require.EqualValues(t, 12, result.AccountID)
			require.Equal(t, "<svg>complete</svg>", result.ResponseText)
			require.Equal(t, []PelicanGroupTestAttempt{{AccountID: 11, AccountName: "a", Error: message}}, result.Attempts)
			require.ElementsMatch(t, []int64{11, 12}, router.released)
			require.ElementsMatch(t, []int64{11}, router.excluded[1])
		})
	}
}

func TestPelicanGroupPartialRetryExhaustionKeepsFailure(t *testing.T) {
	for _, available := range []int{1, 3, 4} {
		t.Run(strconv.Itoa(available)+" accounts available", func(t *testing.T) {
			repo := &groupTestRetryRepo{}
			router := &groupTestRouterFake{}
			for id := 1; id <= available; id++ {
				router.steps = append(router.steps, routeStep{account: account(int64(id), "pool")})
			}
			clock := groupTestNow
			calls := 0
			svc := newGroupTestService(repo, router, func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
				calls++
				clock = clock.Add(20 * time.Second)
				return &ScheduledTestResult{Status: "failed", ResponseText: "<html>partial", ErrorMessage: "Stream ended before response.completed", LatencyMs: 20000}, nil
			})
			svc.now = func() time.Time { return clock }
			svc.execute(groupTestPlan(1), groupTestNow)
			require.Equal(t, min(available, 3), calls)
			require.Len(t, repo.results, 1)
			result := repo.results[0]
			require.Equal(t, "failed", result.Status)
			require.EqualValues(t, calls, result.AccountID)
			require.Equal(t, "<html>partial", result.ResponseText)
			require.Equal(t, "Stream ended before response.completed", result.ErrorMessage)
			require.Len(t, result.Attempts, calls-1)
			require.EqualValues(t, calls*20000, result.LatencyMs)
			require.Len(t, router.released, calls)
		})
	}
}

func TestPelicanGroupRetriesModelFailuresAfterPartialOutput(t *testing.T) {
	message := "Model did not return HTML or SVG"
	router := &groupTestRouterFake{steps: []routeStep{{account: account(11, "a")}, {account: account(12, "b")}}}
	svc := newGroupTestService(&groupTestRetryRepo{}, router, answers(map[int64]ScheduledTestResult{
		11: {Status: "failed", ResponseText: "partial answer", ErrorMessage: message},
		12: {Status: "success", ResponseText: "<svg>complete</svg>"},
	}))
	result := svc.runSample(context.Background(), groupTestPlan(1), &Group{ID: 4})
	require.Equal(t, "success", result.Status)
	require.EqualValues(t, 12, result.AccountID)
	require.Equal(t, "<svg>complete</svg>", result.ResponseText)
	require.Equal(t, []PelicanGroupTestAttempt{{AccountID: 11, AccountName: "a", Error: message}}, result.Attempts)
	require.ElementsMatch(t, []int64{11}, router.excluded[1])
}

func TestPelicanGroupCancellationStopsPartialRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router := &groupTestRouterFake{steps: []routeStep{{account: account(11, "a")}, {account: account(12, "b")}}}
	svc := newGroupTestService(&groupTestRetryRepo{}, router, func(context.Context, int64, string, *PelicanTestConfig) (*ScheduledTestResult, error) {
		cancel()
		return &ScheduledTestResult{Status: "failed", ResponseText: "partial", ErrorMessage: "Stream read error: context canceled"}, nil
	})
	result := svc.runSample(ctx, groupTestPlan(1), &Group{ID: 4})
	require.Equal(t, "failed", result.Status)
	require.Equal(t, "partial", result.ResponseText)
	require.Empty(t, result.Attempts)
	require.Len(t, router.excluded, 1)
	require.Equal(t, []int64{11}, router.released)
}
