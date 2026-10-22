package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractImageGenQuota(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"limits_progress": [
			{"feature_name": "image_gen", "remaining": 12, "limit": 40, "reset_after": "2026-10-23T00:00:00Z"},
			{"feature_name": "other", "remaining": 1, "limit": 2}
		]
	}`)
	quota, ok := extractImageGenQuota(body)
	if !ok {
		t.Fatal("expected image_gen quota")
	}
	if quota.Remaining != 12 || quota.Total == nil || *quota.Total != 40 || quota.ResetAfter != "2026-10-23T00:00:00Z" {
		t.Fatalf("quota = %+v", quota)
	}

	remainingOnly, ok := extractImageGenQuota([]byte(`{"limits_progress":[{"feature_name":"image_gen","remaining":"7","reset_after":""}]}`))
	if !ok || remainingOnly.Remaining != 7 || remainingOnly.Total != nil {
		t.Fatalf("remaining-only quota = %+v ok=%v", remainingOnly, ok)
	}

	if _, ok := extractImageGenQuota([]byte(`{"limits_progress":[{"feature_name":"file_upload","remaining":3,"limit":9}]}`)); ok {
		t.Fatal("non image_gen item must not count")
	}
}

func TestImageGenQuotaExtraRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	total := 40
	updates := imageGenQuotaExtraUpdates(imageGenQuota{Remaining: 12, Total: &total, ResetAfter: "soon"}, now)
	usage := &UsageInfo{}
	applyImageGenQuotaToUsage(usage, updates)
	if usage.ImageGenRemaining == nil || *usage.ImageGenRemaining != 12 || usage.ImageGenTotal == nil || *usage.ImageGenTotal != 40 {
		t.Fatalf("usage = %+v", usage)
	}

	unknownTotal := imageGenQuotaExtraUpdates(imageGenQuota{Remaining: 3}, now)
	if _, exists := unknownTotal[extraOpenAIImageGenTotal]; !exists || unknownTotal[extraOpenAIImageGenTotal] != nil {
		t.Fatalf("missing total must be stored as null, got %#v", unknownTotal[extraOpenAIImageGenTotal])
	}
	usage = &UsageInfo{}
	applyImageGenQuotaToUsage(usage, unknownTotal)
	if usage.ImageGenRemaining == nil || *usage.ImageGenRemaining != 3 || usage.ImageGenTotal != nil {
		t.Fatalf("unknown total usage = %+v", usage)
	}
	if !openAIImageGenSnapshotStale(map[string]any{extraOpenAIImageGenRemaining: 3}, now) {
		t.Fatal("missing updated_at must be stale")
	}
	if openAIImageGenSnapshotStale(updates, now.Add(time.Minute)) {
		t.Fatal("fresh snapshot must not be stale")
	}
}

func TestFetchOpenAIImageGenQuotaRequest(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != chatGPTConversationInitPath {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-OpenAI-Target-Path") != chatGPTConversationInitPath || r.Header.Get("chatgpt-account-id") != "acc-1" {
			t.Errorf("headers = %#v", r.Header)
		}
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"limits_progress":[{"feature_name":"image_gen","remaining":9,"max":30,"reset_after":"later"}]}`))
	}))
	defer server.Close()

	originalURL := chatGPTConversationInitURL
	chatGPTConversationInitURL = server.URL + chatGPTConversationInitPath
	t.Cleanup(func() { chatGPTConversationInitURL = originalURL })

	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "token",
			"chatgpt_account_id": "acc-1",
		},
	}
	quota, err := fetchOpenAIImageGenQuota(context.Background(), account)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if quota.Remaining != 9 || quota.Total == nil || *quota.Total != 30 {
		t.Fatalf("quota = %+v", quota)
	}
	if gotBody != chatGPTConversationInitBody {
		t.Fatalf("body = %s", gotBody)
	}
}
