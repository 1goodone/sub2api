package service

import (
	"context"
	"reflect"
	"testing"
)

func TestAPIKeyService_RejectsV10AuthSnapshotWithoutModelAllowlist(t *testing.T) {
	groupID := int64(9)
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-models-list", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{
			Version:  10,
			APIKeyID: 1,
			UserID:   2,
			GroupID:  &groupID,
			Status:   StatusActive,
			User: APIKeyAuthUserSnapshot{
				ID:          2,
				Status:      StatusActive,
				Role:        RoleUser,
				Balance:     10,
				Concurrency: 3,
			},
			Group: &APIKeyAuthGroupSnapshot{
				ID:               groupID,
				Name:             "openai",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
		},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatalf("expected v10 auth snapshot to be rejected after model_allowlist was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_RejectsV15AuthSnapshotWithoutReasoningEffortPolicy(t *testing.T) {
	svc := &APIKeyService{}

	apiKey, ok, err := svc.applyAuthCacheEntry("k-legacy-reasoning-mappings", &APIKeyAuthCacheEntry{
		Snapshot: &APIKeyAuthSnapshot{Version: 15},
	})

	if err != nil {
		t.Fatalf("expected stale snapshot to be ignored without error, got %v", err)
	}
	if ok {
		t.Fatal("expected v15 auth snapshot to be rejected after reasoning effort policy was added")
	}
	if apiKey != nil {
		t.Fatalf("expected no API key from stale snapshot, got %#v", apiKey)
	}
}

func TestAPIKeyService_CurrentAuthSnapshotRoundTripPreservesAutomaticRouting(t *testing.T) {
	groupID := int64(9)
	mixedGroupIDs := []int64{9, 11}
	apiKey := &APIKey{
		ID:            1,
		UserID:        2,
		GroupID:       &groupID,
		MixedMode:     true,
		MixedGroupIDs: mixedGroupIDs,
		Key:           "k-mixed-roundtrip",
		Status:        StatusActive,
		User:          &User{ID: 2, Status: StatusActive, Role: RoleUser},
		Group: &Group{
			ID:               9,
			Name:             "cheap",
			Platform:         PlatformOpenAI,
			Status:           StatusActive,
			SubscriptionType: SubscriptionTypeStandard,
			RateMultiplier:   1,
		},
		MixedGroups: []*Group{
			{
				ID:               9,
				Name:             "cheap",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   1,
			},
			{
				ID:               11,
				Name:             "fallback",
				Platform:         PlatformOpenAI,
				Status:           StatusActive,
				SubscriptionType: SubscriptionTypeStandard,
				RateMultiplier:   2,
				AllowLive:        true,
				ModelAllowlist: GroupModelAllowlist{
					Enabled: true,
					Models:  []string{"gpt-allowed"},
				},
			},
		},
	}
	svc := &APIKeyService{}

	snapshot := svc.snapshotFromAPIKey(context.Background(), apiKey)
	if snapshot.Version != apiKeyAuthSnapshotVersion {
		t.Fatalf("unexpected auth snapshot version: snapshot=%d current=%d", snapshot.Version, apiKeyAuthSnapshotVersion)
	}
	roundTrip := svc.snapshotToAPIKey(apiKey.Key, snapshot)
	if !roundTrip.MixedMode || !reflect.DeepEqual(mixedGroupIDs, roundTrip.MixedGroupIDs) {
		t.Fatalf("automatic routing flags were not preserved: %#v", roundTrip)
	}
	if len(roundTrip.MixedGroups) != 2 || roundTrip.MixedGroups[1].ID != 11 ||
		!roundTrip.MixedGroups[1].AllowLive || !roundTrip.MixedGroups[1].ModelAllowlist.Allows("GPT-ALLOWED") ||
		roundTrip.MixedGroups[1].ModelAllowlist.Allows("GPT-DISABLED") {
		t.Fatalf("automatic routing group snapshots were not preserved: %#v", roundTrip.MixedGroups)
	}
}
