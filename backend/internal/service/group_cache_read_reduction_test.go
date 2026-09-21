package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type cacheReductionGroupRepoStub struct {
	GroupRepository
	group   *Group
	created *Group
	updated *Group
}

func (s *cacheReductionGroupRepoStub) Create(_ context.Context, group *Group) error {
	s.created = group
	return nil
}

func (s *cacheReductionGroupRepoStub) GetByID(_ context.Context, _ int64) (*Group, error) {
	return s.group, nil
}

func (s *cacheReductionGroupRepoStub) GetByIDLite(_ context.Context, _ int64) (*Group, error) {
	return s.group, nil
}

func (s *cacheReductionGroupRepoStub) Update(_ context.Context, group *Group) error {
	s.updated = group
	return nil
}

func TestReduceCacheReadTokens(t *testing.T) {
	tests := []struct {
		name      string
		cacheRead int
		percent   float64
		remaining int
		moved     int
	}{
		{name: "no cache", cacheRead: 0, percent: 90, remaining: 0, moved: 0},
		{name: "disabled", cacheRead: 100, percent: 0, remaining: 100, moved: 0},
		{name: "partial", cacheRead: 100, percent: 30, remaining: 70, moved: 30},
		{name: "rounds to nearest token", cacheRead: 3, percent: 50, remaining: 1, moved: 2},
		{name: "all", cacheRead: 100, percent: 100, remaining: 0, moved: 100},
		{name: "runtime clamps oversized percent", cacheRead: 100, percent: 150, remaining: 0, moved: 100},
		{name: "runtime ignores negative percent", cacheRead: 100, percent: -10, remaining: 100, moved: 0},
		{name: "negative cache cannot become a deduction", cacheRead: -5, percent: 90, remaining: 0, moved: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, moved := reduceCacheReadTokens(tt.cacheRead, tt.percent)
			require.Equal(t, tt.remaining, remaining)
			require.Equal(t, tt.moved, moved)
			require.GreaterOrEqual(t, remaining, 0)
			require.GreaterOrEqual(t, moved, 0)
		})
	}
}

func TestApplyClaudeCacheReadReductionPreservesTotalInput(t *testing.T) {
	usage := ClaudeUsage{InputTokens: 10, CacheReadInputTokens: 90}
	before := usage.InputTokens + usage.CacheReadInputTokens

	applyClaudeCacheReadReduction(&usage, 50)

	require.Equal(t, 55, usage.InputTokens)
	require.Equal(t, 45, usage.CacheReadInputTokens)
	require.Equal(t, before, usage.InputTokens+usage.CacheReadInputTokens)

	noCache := ClaudeUsage{InputTokens: 12}
	applyClaudeCacheReadReduction(&noCache, 95)
	require.Equal(t, ClaudeUsage{InputTokens: 12}, noCache)

	malformed := ClaudeUsage{InputTokens: 12, CacheReadInputTokens: -4}
	applyClaudeCacheReadReduction(&malformed, 95)
	require.Equal(t, 12, malformed.InputTokens)
	require.Zero(t, malformed.CacheReadInputTokens)
}

func TestApplyOpenAICacheReadReductionPreservesTotalsAndImageBounds(t *testing.T) {
	usage := OpenAIUsage{
		InputTokens:          100,
		ImageInputTokens:     70,
		CacheReadInputTokens: 80,
		ImageCacheReadTokens: 60,
	}
	beforeInput := usage.InputTokens
	beforeRegularInput := usage.InputTokens - usage.CacheReadInputTokens
	beforeRegularImageInput := usage.ImageInputTokens - usage.ImageCacheReadTokens

	applyOpenAICacheReadReduction(&usage, 25)

	require.Equal(t, beforeInput, usage.InputTokens, "OpenAI input_tokens already includes cache reads")
	require.Equal(t, 60, usage.CacheReadInputTokens)
	require.Equal(t, 45, usage.ImageCacheReadTokens)
	require.Equal(t, 40, usage.InputTokens-usage.CacheReadInputTokens)
	require.Equal(t, 25, usage.ImageInputTokens-usage.ImageCacheReadTokens)
	require.Equal(t, beforeInput, beforeRegularInput+20+usage.CacheReadInputTokens)
	require.Equal(t, usage.ImageInputTokens, beforeRegularImageInput+15+usage.ImageCacheReadTokens)
	require.LessOrEqual(t, usage.ImageCacheReadTokens, usage.CacheReadInputTokens)

	noCache := OpenAIUsage{InputTokens: 12, ImageCacheReadTokens: 5}
	applyOpenAICacheReadReduction(&noCache, 95)
	require.Equal(t, 12, noCache.InputTokens)
	require.Zero(t, noCache.CacheReadInputTokens)
	require.Zero(t, noCache.ImageCacheReadTokens)

	malformed := OpenAIUsage{InputTokens: 12, CacheReadInputTokens: 2, ImageCacheReadTokens: 10}
	applyOpenAICacheReadReduction(&malformed, 50)
	require.Equal(t, 1, malformed.CacheReadInputTokens)
	require.Equal(t, 1, malformed.ImageCacheReadTokens)
}

func TestOpenAIRecordUsageAppliesGroupCacheReadReduction(t *testing.T) {
	tests := []struct {
		name       string
		usage      OpenAIUsage
		percent    float64
		wantInput  int
		wantCached int
	}{
		{name: "partial cache", usage: OpenAIUsage{InputTokens: 100, CacheReadInputTokens: 80}, percent: 25, wantInput: 40, wantCached: 60},
		{name: "no cache", usage: OpenAIUsage{InputTokens: 12}, percent: 95, wantInput: 12, wantCached: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
			svc := newOpenAIRecordUsageServiceForTest(usageRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
			groupID := int64(17)

			err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
				Result: &OpenAIForwardResult{
					RequestID: "cache-reduction-" + tt.name,
					Usage:     tt.usage,
					Model:     "gpt-5.1",
					Duration:  time.Second,
				},
				APIKey: &APIKey{
					ID:      1,
					GroupID: &groupID,
					Group: &Group{
						ID:                        groupID,
						RateMultiplier:            1,
						CacheReadReductionPercent: tt.percent,
					},
				},
				User:    &User{ID: 2},
				Account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
			})

			require.NoError(t, err)
			require.NotNil(t, usageRepo.lastLog)
			require.Equal(t, tt.wantInput, usageRepo.lastLog.InputTokens)
			require.Equal(t, tt.wantCached, usageRepo.lastLog.CacheReadTokens)
			require.GreaterOrEqual(t, usageRepo.lastLog.CacheReadTokens, 0)
		})
	}
}

func TestValidateCacheReadReductionPercent(t *testing.T) {
	for _, percent := range []float64{0, 0.1, 50, 100} {
		require.NoError(t, ValidateCacheReadReductionPercent(percent))
	}
	for _, percent := range []float64{-0.1, 100.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		require.Error(t, ValidateCacheReadReductionPercent(percent))
	}
}

func TestAdminServicePersistsCacheReadReductionPercent(t *testing.T) {
	createRepo := &cacheReductionGroupRepoStub{}
	createService := &adminServiceImpl{groupRepo: createRepo}
	created, err := createService.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                      "cache-adjusted",
		RateMultiplier:            1,
		CacheReadReductionPercent: 37.5,
	})
	require.NoError(t, err)
	require.Equal(t, 37.5, created.CacheReadReductionPercent)
	require.Equal(t, 37.5, createRepo.created.CacheReadReductionPercent)

	invalidRepo := &cacheReductionGroupRepoStub{}
	invalidService := &adminServiceImpl{groupRepo: invalidRepo}
	_, err = invalidService.CreateGroup(context.Background(), &CreateGroupInput{
		Name:                      "invalid-cache-adjustment",
		RateMultiplier:            1,
		CacheReadReductionPercent: 100.1,
	})
	require.Error(t, err)
	require.Nil(t, invalidRepo.created)

	updateRepo := &cacheReductionGroupRepoStub{group: &Group{
		ID: 1, Name: "existing", Platform: PlatformAnthropic, Status: StatusActive, RateMultiplier: 1,
	}}
	updateService := &adminServiceImpl{groupRepo: updateRepo}
	percent := 90.0
	updated, err := updateService.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		CacheReadReductionPercent: &percent,
	})
	require.NoError(t, err)
	require.Equal(t, 90.0, updated.CacheReadReductionPercent)
	require.Equal(t, 90.0, updateRepo.updated.CacheReadReductionPercent)

	invalidPercent := -1.0
	_, err = updateService.UpdateGroup(context.Background(), 1, &UpdateGroupInput{
		CacheReadReductionPercent: &invalidPercent,
	})
	require.Error(t, err)
}

func TestCacheReadReductionPercentSnapshotRoundTrip(t *testing.T) {
	groupID := int64(7)
	apiKey := &APIKey{
		GroupID: &groupID,
		User:    &User{ID: 1, Status: StatusActive, Role: RoleUser},
		Group: &Group{
			ID: groupID, Platform: PlatformOpenAI, Status: StatusActive,
			CacheReadReductionPercent: 94.5,
		},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	require.NotNil(t, snapshot)
	require.NotNil(t, snapshot.Group)
	require.Equal(t, 94.5, snapshot.Group.CacheReadReductionPercent)

	restored := svc.snapshotToAPIKey("sk-test", snapshot)
	require.NotNil(t, restored.Group)
	require.Equal(t, 94.5, restored.Group.CacheReadReductionPercent)
}
