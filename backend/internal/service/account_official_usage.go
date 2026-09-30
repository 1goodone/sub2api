package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	httppool "github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

const (
	openAIOfficialUsageCountsURL    = "https://chatgpt.com/backend-api/wham/analytics/daily-workspace-usage-counts"
	openAIOfficialUsageBreakdownURL = "https://chatgpt.com/backend-api/wham/usage/daily-token-usage-breakdown"
	openAIOfficialUsageCacheTTL     = 3 * time.Minute
	openAIOfficialUsageRetention    = 84
	openAIOfficialCreditsPerUSD     = 25.0
)

var officialUsageCountsURL = openAIOfficialUsageCountsURL
var officialUsageBreakdownURL = openAIOfficialUsageBreakdownURL

type OfficialUsageState string

const (
	OfficialUsageStateOK           OfficialUsageState = "ok"
	OfficialUsageStateUnsupported  OfficialUsageState = "unsupported"
	OfficialUsageStateUnauthorized OfficialUsageState = "unauthorized"
	OfficialUsageStateRateLimited  OfficialUsageState = "rate_limited"
	OfficialUsageStateTimeout      OfficialUsageState = "timeout"
	OfficialUsageStateUnavailable  OfficialUsageState = "unavailable"
	OfficialUsageStateStale        OfficialUsageState = "stale"
)

type AccountOfficialUsageResponse struct {
	State          OfficialUsageState    `json:"state"`
	Message        string                `json:"message,omitempty"`
	Range          string                `json:"range"`
	Days           int                   `json:"days"`
	Source         string                `json:"source"`
	FetchedAt      time.Time             `json:"fetched_at"`
	Stale          bool                  `json:"stale"`
	RefreshError   string                `json:"refresh_error,omitempty"`
	BreakdownState OfficialUsageState    `json:"breakdown_state,omitempty"`
	BreakdownError string                `json:"breakdown_error,omitempty"`
	RetentionDays  int                   `json:"retention_days"`
	CreditsPerUSD  float64               `json:"credits_per_usd"`
	Totals         OfficialUsageTotals   `json:"totals"`
	Cycle          OfficialUsageCycle    `json:"cycle"`
	Daily          []OfficialUsageDay    `json:"daily"`
	Clients        []OfficialUsageClient `json:"clients"`
	Models         []OfficialUsageModel  `json:"models"`
}

type OfficialUsageTotals struct {
	Credits     float64 `json:"credits"`
	USD         float64 `json:"usd"`
	Tokens      int64   `json:"tokens"`
	Turns       int64   `json:"turns"`
	Threads     int64   `json:"threads"`
	SettledDays int     `json:"settled_days"`
}

type OfficialUsageDay struct {
	Date                string               `json:"date"`
	Settled             bool                 `json:"settled"`
	Credits             float64              `json:"credits"`
	USD                 float64              `json:"usd"`
	Users               int                  `json:"users"`
	Threads             int                  `json:"threads"`
	Turns               int                  `json:"turns"`
	UncachedInputTokens int64                `json:"uncached_input_tokens"`
	CachedInputTokens   int64                `json:"cached_input_tokens"`
	OutputTokens        int64                `json:"output_tokens"`
	TotalTokens         int64                `json:"total_tokens"`
	Models              []OfficialUsageModel `json:"models,omitempty"`
}

type OfficialUsageClient struct {
	ClientID string  `json:"client_id"`
	Credits  float64 `json:"credits"`
	USD      float64 `json:"usd"`
	Tokens   int64   `json:"tokens"`
	Turns    int64   `json:"turns"`
}

type OfficialUsageModel struct {
	Model   string  `json:"model"`
	Speed   string  `json:"speed"`
	Share   float64 `json:"share,omitempty"`
	Credits float64 `json:"credits"`
	USD     float64 `json:"usd"`
}

type OfficialUsageCycle struct {
	Available        bool       `json:"available"`
	Reliable         bool       `json:"reliable"`
	Reason           string     `json:"reason,omitempty"`
	Utilization      float64    `json:"utilization,omitempty"`
	UsedUSD          float64    `json:"used_usd,omitempty"`
	EstimatedUSD     float64    `json:"estimated_usd,omitempty"`
	EstimatedLowUSD  float64    `json:"estimated_low_usd,omitempty"`
	EstimatedHighUSD float64    `json:"estimated_high_usd,omitempty"`
	StartsAt         *time.Time `json:"starts_at,omitempty"`
	ResetsAt         *time.Time `json:"resets_at,omitempty"`
	UsageUpdatedAt   *time.Time `json:"usage_updated_at,omitempty"`
}

type officialUsageCacheEntry struct {
	response  *AccountOfficialUsageResponse
	timestamp time.Time
}

type officialUsageFetchError struct {
	state   OfficialUsageState
	message string
	cause   error
}

func (e *officialUsageFetchError) Error() string { return e.message }
func (e *officialUsageFetchError) Unwrap() error { return e.cause }

type officialCountsResponse struct {
	Data []officialCountsDay `json:"data"`
}
type officialCountsDay struct {
	Date    string                 `json:"date"`
	Totals  officialCounts         `json:"totals"`
	Clients []officialCountsClient `json:"clients"`
}
type officialCounts struct {
	Users                   int     `json:"users"`
	Threads                 int     `json:"threads"`
	Turns                   int     `json:"turns"`
	Credits                 float64 `json:"credits"`
	UncachedTextInputTokens *int64  `json:"uncached_text_input_tokens"`
	CachedTextInputTokens   *int64  `json:"cached_text_input_tokens"`
	TextOutputTokens        *int64  `json:"text_output_tokens"`
	TextTotalTokens         *int64  `json:"text_total_tokens"`
}
type officialCountsClient struct {
	ClientID string `json:"client_id"`
	officialCounts
}
type officialBreakdownResponse struct {
	Data []officialBreakdownDay `json:"data"`
}
type officialBreakdownDay struct {
	Date   string                   `json:"date"`
	Models []officialBreakdownModel `json:"models"`
}
type officialBreakdownModel struct {
	Model   string  `json:"model"`
	Speed   string  `json:"speed"`
	Percent float64 `json:"credits"`
}

func (s *AccountUsageService) GetOfficialUsage(ctx context.Context, accountID int64, rangeKey string, refresh bool) (*AccountOfficialUsageResponse, error) {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get account failed: %w", err)
	}
	days, normalized, ok := normalizeOfficialUsageRange(rangeKey)
	if !ok {
		return nil, fmt.Errorf("invalid official usage range %q", rangeKey)
	}
	if state, message := officialUsageEligibility(account); state != OfficialUsageStateOK {
		return emptyOfficialUsageResponse(normalized, days, state, message), nil
	}
	if s.cache == nil {
		s.cache = NewUsageCache()
	}
	key := fmt.Sprintf("%d:%s", accountID, normalized)
	if !refresh {
		if cached := loadOfficialUsageCache(s.cache, key); cached != nil && time.Since(cached.timestamp) < openAIOfficialUsageCacheTTL {
			return cloneOfficialUsageResponse(cached.response), nil
		}
	}
	value, _, _ := s.cache.officialUsageFlight.Do(key, func() (any, error) {
		if !refresh {
			if cached := loadOfficialUsageCache(s.cache, key); cached != nil && time.Since(cached.timestamp) < openAIOfficialUsageCacheTTL {
				return cloneOfficialUsageResponse(cached.response), nil
			}
		}
		result, fetchErr := s.fetchOfficialUsage(ctx, account, normalized, days)
		if fetchErr == nil {
			s.cache.officialUsageCache.Store(key, &officialUsageCacheEntry{response: result, timestamp: result.FetchedAt})
			return cloneOfficialUsageResponse(result), nil
		}
		state, message := classifyOfficialUsageError(fetchErr)
		if cached := loadOfficialUsageCache(s.cache, key); cached != nil {
			stale := cloneOfficialUsageResponse(cached.response)
			stale.State = OfficialUsageStateStale
			stale.Stale = true
			stale.RefreshError = message
			return stale, nil
		}
		slog.Warn("openai_official_usage_fetch_failed", "account_id", accountID, "state", state, "error", fetchErr)
		return emptyOfficialUsageResponse(normalized, days, state, message), nil
	})
	return value.(*AccountOfficialUsageResponse), nil
}

func (s *AccountUsageService) fetchOfficialUsage(ctx context.Context, account *Account, rangeKey string, days int) (*AccountOfficialUsageResponse, error) {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	end := now.Format("2006-01-02")
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var counts officialCountsResponse
	var breakdown officialBreakdownResponse
	countsCh := make(chan error, 1)
	breakdownCh := make(chan error, 1)
	go func() {
		countsCh <- s.queryOfficialUsageEndpoint(requestCtx, account, officialUsageCountsURL, start, end, true, &counts)
	}()
	go func() {
		breakdownCh <- s.queryOfficialUsageEndpoint(requestCtx, account, officialUsageBreakdownURL, start, end, false, &breakdown)
	}()
	countsErr := <-countsCh
	if countsErr != nil {
		cancel()
		<-breakdownCh
		return nil, countsErr
	}
	breakdownErr := <-breakdownCh
	result := buildOfficialUsageResponse(rangeKey, days, now, counts, breakdown)
	result.Cycle = buildOfficialUsageCycle(account, result.Daily, now)
	if breakdownErr != nil {
		result.BreakdownState, result.BreakdownError = classifyOfficialUsageError(breakdownErr)
	} else {
		result.BreakdownState = OfficialUsageStateOK
	}
	return result, nil
}

func (s *AccountUsageService) queryOfficialUsageEndpoint(ctx context.Context, account *Account, baseURL, start, end string, workspaceUser bool, out any) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return &officialUsageFetchError{state: OfficialUsageStateUnavailable, message: "官方结算服务地址无效", cause: err}
	}
	query := u.Query()
	query.Set("start_date", start)
	query.Set("end_date", end)
	query.Set("group_by", "day")
	if workspaceUser {
		query.Set("workspace_user", "true")
	}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return &officialUsageFetchError{state: OfficialUsageStateUnavailable, message: "无法创建官方结算请求", cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+account.GetOpenAIAccessToken())
	req.Header.Set("Accept", "application/json")
	canonical := resolveCodexOutboundIdentity("")
	req.Header.Set("Originator", canonical.originator)
	req.Header.Set("Version", canonical.version)
	req.Header.Set("User-Agent", canonical.userAgent)
	enforceCodexIdentityHeadersWithUA(req.Header, account.GetOpenAIUserAgent())
	setOpenAIChatGPTAccountHeaders(req.Header, account)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	client, err := httppool.GetClient(httppool.Options{ProxyURL: proxyURL, Timeout: 20 * time.Second, ResponseHeaderTimeout: 15 * time.Second})
	if err != nil {
		return &officialUsageFetchError{state: OfficialUsageStateUnavailable, message: "无法建立官方结算连接", cause: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		state, message := OfficialUsageStateUnavailable, "官方结算服务暂时不可用"
		var netErr net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
			state, message = OfficialUsageStateTimeout, "官方结算请求超时"
		}
		return &officialUsageFetchError{state: state, message: message, cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state, message := OfficialUsageStateUnavailable, "官方结算服务暂时不可用"
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			state, message = OfficialUsageStateUnauthorized, "账号授权已失效或无权读取官方结算数据"
		case http.StatusTooManyRequests:
			state, message = OfficialUsageStateRateLimited, "官方结算接口请求过于频繁，请稍后重试"
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			state, message = OfficialUsageStateTimeout, "官方结算请求超时"
		}
		return &officialUsageFetchError{state: state, message: message, cause: fmt.Errorf("upstream status %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return &officialUsageFetchError{state: OfficialUsageStateUnavailable, message: "读取官方结算数据失败", cause: err}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return &officialUsageFetchError{state: OfficialUsageStateUnavailable, message: "官方结算数据格式异常", cause: err}
	}
	return nil
}

func buildOfficialUsageResponse(rangeKey string, days int, fetchedAt time.Time, counts officialCountsResponse, breakdown officialBreakdownResponse) *AccountOfficialUsageResponse {
	result := emptyOfficialUsageResponse(rangeKey, days, OfficialUsageStateOK, "")
	result.FetchedAt = fetchedAt
	breakdownByDay := make(map[string]officialBreakdownDay, len(breakdown.Data))
	for _, day := range breakdown.Data {
		breakdownByDay[day.Date] = day
	}
	clients := map[string]*OfficialUsageClient{}
	models := map[string]*OfficialUsageModel{}
	today := fetchedAt.Format("2006-01-02")
	for _, row := range counts.Data {
		day := OfficialUsageDay{
			Date:                row.Date,
			Settled:             row.Date != "" && row.Date < today && row.Totals.TextTotalTokens != nil,
			Credits:             row.Totals.Credits,
			USD:                 row.Totals.Credits / openAIOfficialCreditsPerUSD,
			Users:               row.Totals.Users,
			Threads:             row.Totals.Threads,
			Turns:               row.Totals.Turns,
			UncachedInputTokens: int64Value(row.Totals.UncachedTextInputTokens),
			CachedInputTokens:   int64Value(row.Totals.CachedTextInputTokens),
			OutputTokens:        int64Value(row.Totals.TextOutputTokens),
			TotalTokens:         int64Value(row.Totals.TextTotalTokens),
		}
		result.Totals.Credits += day.Credits
		result.Totals.USD += day.USD
		result.Totals.Tokens += day.TotalTokens
		result.Totals.Turns += int64(day.Turns)
		result.Totals.Threads += int64(day.Threads)
		if day.Settled {
			result.Totals.SettledDays++
		}
		for _, client := range row.Clients {
			id := strings.TrimSpace(client.ClientID)
			if id == "" {
				id = "CODEX_UNKNOWN_DEFAULT"
			}
			item := clients[id]
			if item == nil {
				item = &OfficialUsageClient{ClientID: id}
				clients[id] = item
			}
			item.Credits += client.Credits
			item.USD += client.Credits / openAIOfficialCreditsPerUSD
			item.Tokens += int64Value(client.TextTotalTokens)
			item.Turns += int64(client.Turns)
		}
		if detail, ok := breakdownByDay[row.Date]; ok {
			totalPercent := 0.0
			for _, model := range detail.Models {
				if model.Percent > 0 {
					totalPercent += model.Percent
				}
			}
			if totalPercent > 0 {
				for _, model := range detail.Models {
					name := strings.TrimSpace(model.Model)
					if name == "" || model.Percent <= 0 {
						continue
					}
					speed := strings.ToLower(strings.TrimSpace(model.Speed))
					if speed == "" {
						speed = "standard"
					}
					share := model.Percent / totalPercent
					credits := share * row.Totals.Credits
					dailyModel := OfficialUsageModel{Model: name, Speed: speed, Share: share, Credits: credits, USD: credits / openAIOfficialCreditsPerUSD}
					day.Models = append(day.Models, dailyModel)
					key := name + "\x00" + speed
					item := models[key]
					if item == nil {
						item = &OfficialUsageModel{Model: name, Speed: speed}
						models[key] = item
					}
					item.Credits += credits
					item.USD += credits / openAIOfficialCreditsPerUSD
				}
				sort.Slice(day.Models, func(i, j int) bool { return day.Models[i].USD > day.Models[j].USD })
			}
		}
		result.Daily = append(result.Daily, day)
	}
	for _, item := range clients {
		result.Clients = append(result.Clients, *item)
	}
	for _, item := range models {
		result.Models = append(result.Models, *item)
	}
	sort.Slice(result.Daily, func(i, j int) bool { return result.Daily[i].Date > result.Daily[j].Date })
	sort.Slice(result.Clients, func(i, j int) bool { return result.Clients[i].USD > result.Clients[j].USD })
	sort.Slice(result.Models, func(i, j int) bool { return result.Models[i].USD > result.Models[j].USD })
	return result
}

func buildOfficialUsageCycle(account *Account, daily []OfficialUsageDay, now time.Time) OfficialUsageCycle {
	cycle := OfficialUsageCycle{Reason: "quota_window_unavailable"}
	usage := &UsageInfo{}
	applyExtraToUsage(usage, account.Extra, now)
	if usage.SevenDay == nil || usage.SevenDay.ResetsAt == nil {
		return cycle
	}
	cycle.Utilization = usage.SevenDay.Utilization
	cycle.ResetsAt = usage.SevenDay.ResetsAt
	start := usage.SevenDay.ResetsAt.Add(-7 * 24 * time.Hour)
	cycle.StartsAt = &start
	if raw := fmt.Sprint(account.Extra["codex_usage_updated_at"]); raw != "" && raw != "<nil>" {
		if parsed, err := parseTime(raw); err == nil {
			cycle.UsageUpdatedAt = &parsed
		}
	}
	if cycle.UsageUpdatedAt == nil {
		cycle.Reason = "quota_snapshot_unavailable"
		return cycle
	}
	startDay := time.Date(start.UTC().Year(), start.UTC().Month(), start.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for _, day := range daily {
		date, err := time.Parse("2006-01-02", day.Date)
		if err == nil && !date.Before(startDay) {
			cycle.UsedUSD += day.USD
		}
	}
	if cycle.Utilization <= 0 || cycle.UsedUSD <= 0 {
		cycle.Reason = "usage_or_cost_unavailable"
		return cycle
	}
	if now.Sub(cycle.UsageUpdatedAt.UTC()) > 30*time.Minute {
		cycle.Reason = "quota_snapshot_stale"
		return cycle
	}
	p := math.Max(0.1, cycle.Utilization)
	cycle.EstimatedUSD = cycle.UsedUSD / (p / 100)
	cycle.EstimatedLowUSD = cycle.UsedUSD / (math.Min(100, p+0.5) / 100)
	cycle.EstimatedHighUSD = cycle.UsedUSD / (math.Max(0.1, p-0.5) / 100)
	cycle.Available = true
	cycle.Reliable = cycle.Utilization >= 10
	if !cycle.Reliable {
		cycle.Reason = "low_utilization"
	} else {
		cycle.Reason = ""
	}
	return cycle
}

func officialUsageEligibility(account *Account) (OfficialUsageState, string) {
	if account == nil || !account.IsOpenAIOAuth() {
		return OfficialUsageStateUnsupported, "仅普通 OpenAI OAuth 账号支持官方结算"
	}
	if account.IsShadow() || account.IsOpenAIAgentIdentity() || account.IsOpenAIPersonalAccessToken() {
		return OfficialUsageStateUnsupported, "当前账号类型不支持官方结算"
	}
	if strings.TrimSpace(account.GetOpenAIAccessToken()) == "" {
		return OfficialUsageStateUnauthorized, "账号没有可用的 access token，请重新授权"
	}
	return OfficialUsageStateOK, ""
}

func normalizeOfficialUsageRange(value string) (int, string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "30d":
		return 30, "30d", true
	case "7d":
		return 7, "7d", true
	case "90d":
		return 90, "90d", true
	case "all":
		return openAIOfficialUsageRetention, "all", true
	default:
		return 0, "", false
	}
}

func emptyOfficialUsageResponse(rangeKey string, days int, state OfficialUsageState, message string) *AccountOfficialUsageResponse {
	return &AccountOfficialUsageResponse{
		State:         state,
		Message:       message,
		Range:         rangeKey,
		Days:          days,
		Source:        "chatgpt-wham",
		FetchedAt:     time.Now().UTC(),
		RetentionDays: openAIOfficialUsageRetention,
		CreditsPerUSD: openAIOfficialCreditsPerUSD,
		Daily:         []OfficialUsageDay{},
		Clients:       []OfficialUsageClient{},
		Models:        []OfficialUsageModel{},
		Cycle:         OfficialUsageCycle{Reason: "quota_window_unavailable"},
	}
}

func classifyOfficialUsageError(err error) (OfficialUsageState, string) {
	var fetchErr *officialUsageFetchError
	if errors.As(err, &fetchErr) {
		return fetchErr.state, fetchErr.message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return OfficialUsageStateTimeout, "官方结算请求超时"
	}
	return OfficialUsageStateUnavailable, "官方结算服务暂时不可用"
}

func loadOfficialUsageCache(cache *UsageCache, key string) *officialUsageCacheEntry {
	if cache == nil {
		return nil
	}
	value, ok := cache.officialUsageCache.Load(key)
	if !ok {
		return nil
	}
	entry, _ := value.(*officialUsageCacheEntry)
	return entry
}

func cloneOfficialUsageResponse(source *AccountOfficialUsageResponse) *AccountOfficialUsageResponse {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
