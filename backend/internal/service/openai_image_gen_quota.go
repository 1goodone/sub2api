package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	chatGPTConversationInitPath = "/backend-api/conversation/init"
	// Matches chatgpt2api: timezone_offset_min -480 (UTC+8) and a null conversation.
	chatGPTConversationInitBody = `{"gizmo_id":null,"requested_default_model":null,"conversation_id":null,"timezone_offset_min":-480}`

	extraOpenAIImageGenRemaining  = "openai_image_gen_remaining"
	extraOpenAIImageGenTotal      = "openai_image_gen_total"
	extraOpenAIImageGenResetAfter = "openai_image_gen_reset_after"
	extraOpenAIImageGenUpdatedAt  = "openai_image_gen_updated_at"
)

var chatGPTConversationInitURL = "https://chatgpt.com" + chatGPTConversationInitPath

// imageGenQuota is the image_gen item from conversation/init limits_progress.
// Total is nil when the payload has no limit/max/total key; do not invent one.
type imageGenQuota struct {
	Remaining  int
	Total      *int
	ResetAfter string
}

func (s *AccountUsageService) refreshOpenAIImageGenQuota(ctx context.Context, account *Account, usage *UsageInfo, now time.Time, force bool) {
	if s == nil || account == nil || usage == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return
	}
	if strings.TrimSpace(account.GetOpenAIAccessToken()) == "" {
		return
	}
	if !force && !openAIImageGenSnapshotStale(account.Extra, now) {
		return
	}
	if !s.shouldFetchOpenAIImageGen(account.ID, now, force) {
		return
	}
	var factory PrivacyClientFactory
	if s.openAIQuotaService != nil {
		factory = s.openAIQuotaService.privacyClientFactory
	}
	quota, err := fetchOpenAIImageGenQuota(ctx, account, factory)
	if err != nil {
		slog.Warn("openai_image_gen_quota_failed", "account_id", account.ID, "error", err)
		return
	}
	updates := imageGenQuotaExtraUpdates(quota, now)
	mergeAccountExtra(account, updates)
	applyImageGenQuotaToUsage(usage, account.Extra)
	if usage.UpdatedAt == nil {
		usage.UpdatedAt = &now
	}
	s.persistOpenAIImageGenQuota(account.ID, updates)
}

func (s *AccountUsageService) shouldFetchOpenAIImageGen(accountID int64, now time.Time, force bool) bool {
	if s == nil || s.cache == nil {
		return force
	}
	if !force {
		if cached, ok := s.cache.openAIImageGenCache.Load(accountID); ok {
			if ts, ok := cached.(time.Time); ok && now.Sub(ts) < openAIProbeCacheTTL {
				return false
			}
		}
	}
	s.cache.openAIImageGenCache.Store(accountID, now)
	return true
}

func (s *AccountUsageService) persistOpenAIImageGenQuota(accountID int64, updates map[string]any) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || len(updates) == 0 {
		return
	}
	go func() {
		updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.accountRepo.UpdateExtra(updateCtx, accountID, updates); err != nil {
			slog.Warn("openai_image_gen_quota_persist_failed", "account_id", accountID, "error", err)
		}
	}()
}

func openAIImageGenSnapshotStale(extra map[string]any, now time.Time) bool {
	if _, ok := imageGenNumber(extraValue(extra, extraOpenAIImageGenRemaining)); !ok {
		return true
	}
	raw, _ := extraValue(extra, extraOpenAIImageGenUpdatedAt).(string)
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return true
	}
	return now.Sub(ts) >= openAIProbeCacheTTL
}

func extraValue(extra map[string]any, key string) any {
	if extra == nil {
		return nil
	}
	return extra[key]
}

func imageGenQuotaExtraUpdates(quota imageGenQuota, now time.Time) map[string]any {
	updates := map[string]any{
		extraOpenAIImageGenRemaining:  quota.Remaining,
		extraOpenAIImageGenResetAfter: quota.ResetAfter,
		extraOpenAIImageGenUpdatedAt:  now.UTC().Format(time.RFC3339),
	}
	if quota.Total != nil {
		updates[extraOpenAIImageGenTotal] = *quota.Total
	} else {
		updates[extraOpenAIImageGenTotal] = nil
	}
	return updates
}

func applyImageGenQuotaToUsage(usage *UsageInfo, extra map[string]any) {
	if usage == nil {
		return
	}
	remaining, ok := imageGenNumber(extraValue(extra, extraOpenAIImageGenRemaining))
	if !ok {
		return
	}
	usage.ImageGenRemaining = &remaining
	usage.ImageGenTotal = nil
	if total, ok := imageGenNumber(extraValue(extra, extraOpenAIImageGenTotal)); ok {
		usage.ImageGenTotal = &total
	}
	usage.ImageGenResetAfter = ""
	if reset, ok := extraValue(extra, extraOpenAIImageGenResetAfter).(string); ok {
		usage.ImageGenResetAfter = strings.TrimSpace(reset)
	}
}

var fetchOpenAIImageGenQuota = func(ctx context.Context, account *Account, factory PrivacyClientFactory) (imageGenQuota, error) {
	if account == nil {
		return imageGenQuota{}, fmt.Errorf("missing openai account")
	}
	token := strings.TrimSpace(account.GetOpenAIAccessToken())
	if token == "" {
		return imageGenQuota{}, fmt.Errorf("no access token available")
	}
	if factory == nil {
		return imageGenQuota{}, fmt.Errorf("chatgpt conversation init client unavailable")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := factory(proxyURL)
	if err != nil {
		return imageGenQuota{}, fmt.Errorf("build chatgpt conversation init client: %w", err)
	}
	headers := buildCodexCommonHeaders(token, account.GetChatGPTAccountID(), account.IsChatGPTAccountFedRAMP())
	headers["content-type"] = "application/json"
	headers["origin"] = "https://chatgpt.com"
	headers["referer"] = "https://chatgpt.com/"
	resp, err := client.R().
		SetContext(reqCtx).
		SetHeaders(headers).
		SetBody([]byte(chatGPTConversationInitBody)).
		Post(chatGPTConversationInitURL)
	if err != nil {
		return imageGenQuota{}, fmt.Errorf("chatgpt conversation init: %w", err)
	}
	body := resp.Bytes()
	if len(body) > 1<<20 {
		body = body[:1<<20]
	}
	if !resp.IsSuccessState() {
		if mitigated := strings.TrimSpace(resp.Header.Get("cf-mitigated")); mitigated != "" {
			return imageGenQuota{}, fmt.Errorf("chatgpt conversation init status %d cf-mitigated=%s", resp.StatusCode, mitigated)
		}
		return imageGenQuota{}, fmt.Errorf("chatgpt conversation init status %d", resp.StatusCode)
	}
	quota, ok := extractImageGenQuota(body)
	if !ok {
		return imageGenQuota{}, fmt.Errorf("chatgpt conversation init has no image_gen remaining")
	}
	return quota, nil
}

func extractImageGenQuota(body []byte) (imageGenQuota, bool) {
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return imageGenQuota{}, false
	}
	item := findImageGenLimit(payload, 0)
	if item == nil {
		return imageGenQuota{}, false
	}
	remaining, ok := imageGenNumber(item["remaining"])
	if !ok {
		return imageGenQuota{}, false
	}
	quota := imageGenQuota{Remaining: remaining}
	if reset, ok := item["reset_after"].(string); ok {
		quota.ResetAfter = strings.TrimSpace(reset)
	}
	for _, key := range []string{"limit", "max", "total"} {
		if total, ok := imageGenNumber(item[key]); ok {
			quota.Total = &total
			break
		}
	}
	return quota, true
}

func findImageGenLimit(node any, depth int) map[string]any {
	if depth > 8 || node == nil {
		return nil
	}
	switch value := node.(type) {
	case map[string]any:
		if isImageGenFeature(value["feature_name"]) {
			if _, ok := imageGenNumber(value["remaining"]); ok {
				return value
			}
		}
		if nested, ok := value["image_gen"].(map[string]any); ok {
			if _, hasRemaining := imageGenNumber(nested["remaining"]); hasRemaining {
				return nested
			}
		}
		for _, child := range value {
			if found := findImageGenLimit(child, depth+1); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range value {
			if found := findImageGenLimit(child, depth+1); found != nil {
				return found
			}
		}
	}
	return nil
}

func isImageGenFeature(value any) bool {
	name, ok := value.(string)
	return ok && strings.EqualFold(strings.TrimSpace(name), "image_gen")
}

func imageGenNumber(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		return finiteImageCount(n)
	case float32:
		return finiteImageCount(float64(n))
	case int:
		return nonNegativeImageCount(n)
	case int64:
		return nonNegativeImageCount(int(n))
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return finiteImageCount(parsed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0, false
		}
		return finiteImageCount(parsed)
	default:
		return 0, false
	}
}

func finiteImageCount(n float64) (int, bool) {
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > math.MaxInt32 {
		return 0, false
	}
	return int(n), true
}

func nonNegativeImageCount(n int) (int, bool) {
	if n < 0 {
		return 0, false
	}
	return n, true
}
