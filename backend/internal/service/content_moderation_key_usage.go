package service

import (
	"context"
	"log/slog"
	"time"
)

const (
	contentModerationUsageBucketDay   = "day"
	contentModerationUsageBucketTotal = "total"
)

type ContentModerationKeyUsageDelta struct {
	Engine       string
	KeyHash      string
	Day          time.Time
	Calls        int64
	Success      int64
	Errors       int64
	LatencyMS    int64
	InputTokens  int64
	OutputTokens int64
}

type ContentModerationKeyUsageSnapshot struct {
	TodayCalls        int64
	TodaySuccess      int64
	TodayErrors       int64
	TodayLatencyMS    int64
	TodayInputTokens  int64
	TodayOutputTokens int64
	TotalCalls        int64
	TotalSuccess      int64
	TotalErrors       int64
	TotalLatencyMS    int64
	TotalInputTokens  int64
	TotalOutputTokens int64
}

type ContentModerationKeyUsageStore interface {
	AddKeyUsage(ctx context.Context, delta ContentModerationKeyUsageDelta) error
	ListKeyUsage(ctx context.Context, engine string, day time.Time) (map[string]ContentModerationKeyUsageSnapshot, error)
}

func contentModerationUsageDay(now time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	y, m, d := now.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func (s *ContentModerationService) keyUsageStore() ContentModerationKeyUsageStore {
	if s == nil {
		return nil
	}
	if s.keyUsage != nil {
		return s.keyUsage
	}
	store, ok := s.repo.(ContentModerationKeyUsageStore)
	if !ok {
		return nil
	}
	return store
}

func (s *ContentModerationService) recordModerationKeyUsage(ctx context.Context, cfg *ContentModerationConfig, key string, latencyMS int, success bool, result *moderationAPIResult) {
	store := s.keyUsageStore()
	if store == nil || cfg == nil || key == "" {
		return
	}
	if latencyMS < 0 {
		latencyMS = 0
	}
	delta := ContentModerationKeyUsageDelta{
		Engine:    moderationEngine(cfg.Engine),
		KeyHash:   scopedModerationKeyHash(key, cfg.Engine),
		Day:       contentModerationUsageDay(time.Now()),
		Calls:     1,
		LatencyMS: int64(latencyMS),
	}
	if success {
		delta.Success = 1
		if result != nil {
			if result.InputTokens > 0 {
				delta.InputTokens = int64(result.InputTokens)
			}
			if result.OutputTokens > 0 {
				delta.OutputTokens = int64(result.OutputTokens)
			}
		}
	} else {
		delta.Errors = 1
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := store.AddKeyUsage(persistCtx, delta); err != nil {
		slog.Warn("content_moderation.key_usage_persist_failed", "error", err)
	}
}

func (s *ContentModerationService) applyPersistedKeyUsage(ctx context.Context, engine string, loads []ContentModerationAPIKeyLoad) []ContentModerationAPIKeyLoad {
	store := s.keyUsageStore()
	if store == nil || len(loads) == 0 {
		return loads
	}
	snaps, err := store.ListKeyUsage(ctx, moderationEngine(engine), contentModerationUsageDay(time.Now()))
	if err != nil {
		slog.Warn("content_moderation.key_usage_read_failed", "error", err)
		return loads
	}
	for i := range loads {
		snap, ok := snaps[loads[i].KeyHash]
		if !ok {
			loads[i].Total = 0
			loads[i].Success = 0
			loads[i].Errors = 0
			loads[i].AvgLatencyMS = 0
			continue
		}
		loads[i].Total = snap.TotalCalls
		loads[i].Success = snap.TotalSuccess
		loads[i].Errors = snap.TotalErrors
		if snap.TotalCalls > 0 {
			loads[i].AvgLatencyMS = snap.TotalLatencyMS / snap.TotalCalls
		}
		loads[i].TodayCalls = snap.TodayCalls
		loads[i].TodayInputTokens = snap.TodayInputTokens
		loads[i].TodayOutputTokens = snap.TodayOutputTokens
		loads[i].TotalInputTokens = snap.TotalInputTokens
		loads[i].TotalOutputTokens = snap.TotalOutputTokens
	}
	return loads
}
