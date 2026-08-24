package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/redis/go-redis/v9"
)

const (
	cloudcatMerchantAPIBaseURL      = "https://catfk.com"
	cloudcatMerchantSessionTTL      = 365 * 24 * time.Hour
	cloudcatMerchantHTTPTimeout     = 15 * time.Second
	cloudcatMerchantMaxResponseSize = 1 << 20
	cloudcatMerchantUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

var (
	ErrCloudcatMerchantUnavailable = infraerrors.ServiceUnavailable("CLOUDCAT_MERCHANT_UNAVAILABLE", "云猫商户中心暂时不可用")
	ErrCloudcatMerchantLoginFailed = infraerrors.BadRequest("CLOUDCAT_MERCHANT_LOGIN_FAILED", "云猫商户登录失败，请检查账号或二次验证码")
	ErrCloudcatMerchantCredentials = infraerrors.BadRequest("CLOUDCAT_MERCHANT_CREDENTIALS_REQUIRED", "云猫商户账号和密码不能为空")
)

// CloudcatMerchantLoginRequest contains credentials accepted by CatFK's merchant login endpoint.
// Passwords are used only for the outbound request and are never persisted.
type CloudcatMerchantLoginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	SafeCode  string `json:"safecode,omitempty"`
	EmailCode string `json:"email_code,omitempty"`
}

// CloudcatMerchantOverview is the read-only merchant dashboard exposed to admins.
type CloudcatMerchantOverview struct {
	Connected          bool       `json:"connected"`
	NeedsLogin         bool       `json:"needs_login"`
	TodayOrderCount    int        `json:"today_order_count"`
	TodayOrderAmount   float64    `json:"today_order_amount"`
	WithdrawableAmount float64    `json:"withdrawable_amount"`
	FrozenAmount       float64    `json:"frozen_amount"`
	Currency           string     `json:"currency"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
}

type CloudcatMerchantService struct {
	redisClient *redis.Client
	encryptor   SecretEncryptor
	baseURL     string
	httpClient  *http.Client
	now         func() time.Time
	sessionTTL  time.Duration
}

func NewCloudcatMerchantService(redisClient *redis.Client, encryptor SecretEncryptor) *CloudcatMerchantService {
	return newCloudcatMerchantServiceWithBaseURL(redisClient, encryptor, cloudcatMerchantAPIBaseURL)
}

// CloudcatMerchantService keeps a server-side CatFK merchant session and reads
// the official dashboard and wallet endpoints. The production base URL is fixed
// to CatFK; the unexported constructor variant exists only for package tests.
func newCloudcatMerchantServiceWithBaseURL(redisClient *redis.Client, encryptor SecretEncryptor, baseURL string) *CloudcatMerchantService {
	return &CloudcatMerchantService{
		redisClient: redisClient,
		encryptor:   encryptor,
		baseURL:     strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: &http.Client{
			Timeout: cloudcatMerchantHTTPTimeout,
			// Merchant credentials must never be forwarded to an arbitrary redirect target.
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return errors.New("cloudcat merchant redirect denied")
			},
		},
		now:        time.Now,
		sessionTTL: cloudcatMerchantSessionTTL,
	}
}

type cloudcatMerchantSession struct {
	MerchantToken string                   `json:"merchant_token"`
	Cookies       []cloudcatMerchantCookie `json:"cookies,omitempty"`
}

type cloudcatMerchantCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type cloudcatMerchantHTTPResponse struct {
	StatusCode int
	Body       []byte
	Cookies    []*http.Cookie
}

type cloudcatMerchantEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type cloudcatMerchantStatistics struct {
	TodayAmount json.RawMessage `json:"today_amount"`
	TodayCount  json.RawMessage `json:"today_count"`
}

type cloudcatMerchantWorkplaceData struct {
	Statistics *cloudcatMerchantStatistics    `json:"statistics"`
	Data       *cloudcatMerchantWorkplaceData `json:"data"`
	Response   *cloudcatMerchantWorkplaceData `json:"response"`
}

type cloudcatMerchantPlatform struct {
	AvailableMoney json.RawMessage `json:"available_money"`
	FreezeMoney    json.RawMessage `json:"freeze_money"`
}

type cloudcatMerchantWalletData struct {
	Platform *cloudcatMerchantPlatform   `json:"platform"`
	Data     *cloudcatMerchantWalletData `json:"data"`
	Response *cloudcatMerchantWalletData `json:"response"`
}

func (s *CloudcatMerchantService) sessionKey(adminID int64) string {
	return "cloudcat:merchant:session:" + strconv.FormatInt(adminID, 10)
}

func (s *CloudcatMerchantService) ready() error {
	if s == nil || s.redisClient == nil || s.encryptor == nil || strings.TrimSpace(s.baseURL) == "" {
		return ErrCloudcatMerchantUnavailable
	}
	return nil
}

func (s *CloudcatMerchantService) Login(ctx context.Context, adminID int64, input CloudcatMerchantLoginRequest) (*CloudcatMerchantOverview, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if adminID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ADMIN_ID", "管理员身份无效")
	}
	input.Username = strings.TrimSpace(input.Username)
	if input.Username == "" || strings.TrimSpace(input.Password) == "" {
		return nil, ErrCloudcatMerchantCredentials
	}

	result, err := s.postJSON(ctx, "/merchantApi/user/login", map[string]string{
		"username":   input.Username,
		"password":   input.Password,
		"safecode":   strings.TrimSpace(input.SafeCode),
		"email_code": strings.TrimSpace(input.EmailCode),
	}, nil)
	if err != nil {
		return nil, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	envelope, err := decodeCloudcatMerchantEnvelope(result.Body)
	if err != nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices || envelope.Code != 1 {
		return nil, ErrCloudcatMerchantLoginFailed
	}
	token := extractMerchantToken(envelope.Data)
	if token == "" {
		return nil, ErrCloudcatMerchantLoginFailed
	}

	session := &cloudcatMerchantSession{
		MerchantToken: token,
		Cookies:       cookiesFromResponse(result.Cookies),
	}
	if err := s.saveSession(ctx, adminID, session); err != nil {
		return nil, err
	}

	overview, invalid, err := s.fetchOverview(ctx, session)
	if err != nil {
		return nil, err
	}
	if invalid {
		_ = s.deleteSession(ctx, adminID)
		return nil, ErrCloudcatMerchantLoginFailed
	}
	if err := s.saveSession(ctx, adminID, session); err != nil {
		return nil, err
	}
	return overview, nil
}

func (s *CloudcatMerchantService) GetOverview(ctx context.Context, adminID int64) (*CloudcatMerchantOverview, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if adminID <= 0 {
		return nil, infraerrors.BadRequest("INVALID_ADMIN_ID", "管理员身份无效")
	}
	session, found, err := s.loadSession(ctx, adminID)
	if err != nil {
		return nil, err
	}
	if !found {
		return cloudcatMerchantNeedsLogin(), nil
	}
	overview, invalid, err := s.fetchOverview(ctx, session)
	if err != nil {
		return nil, err
	}
	if invalid {
		_ = s.deleteSession(ctx, adminID)
		return cloudcatMerchantNeedsLogin(), nil
	}
	if err := s.saveSession(ctx, adminID, session); err != nil {
		return nil, err
	}
	return overview, nil
}

func (s *CloudcatMerchantService) Refresh(ctx context.Context, adminID int64) (*CloudcatMerchantOverview, error) {
	return s.GetOverview(ctx, adminID)
}

func (s *CloudcatMerchantService) Logout(ctx context.Context, adminID int64) error {
	if err := s.ready(); err != nil {
		return err
	}
	if adminID <= 0 {
		return infraerrors.BadRequest("INVALID_ADMIN_ID", "管理员身份无效")
	}
	return s.deleteSession(ctx, adminID)
}

func cloudcatMerchantNeedsLogin() *CloudcatMerchantOverview {
	return &CloudcatMerchantOverview{NeedsLogin: true, Currency: "CNY"}
}

func (s *CloudcatMerchantService) fetchOverview(ctx context.Context, session *cloudcatMerchantSession) (*CloudcatMerchantOverview, bool, error) {
	check, err := s.postJSON(ctx, "/merchantApi/user/checkToken", map[string]any{}, session)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	checkEnvelope, err := decodeCloudcatMerchantEnvelope(check.Body)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	mergeCookies(session, check.Cookies)
	if cloudcatMerchantTokenInvalid(check.StatusCode, checkEnvelope) {
		return nil, true, nil
	}

	workplace, err := s.postJSON(ctx, "/merchantApi/dashboard/workplace", map[string]any{}, session)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	workplaceEnvelope, err := decodeCloudcatMerchantEnvelope(workplace.Body)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	mergeCookies(session, workplace.Cookies)
	if cloudcatMerchantResponseInvalid(workplace.StatusCode, workplaceEnvelope) {
		return nil, true, nil
	}
	todayCount, todayAmount, err := parseWorkplaceStats(workplaceEnvelope.Data)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}

	wallet, err := s.postJSON(ctx, "/merchantApi/wallet/info", map[string]any{}, session)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	walletEnvelope, err := decodeCloudcatMerchantEnvelope(wallet.Body)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}
	mergeCookies(session, wallet.Cookies)
	if cloudcatMerchantResponseInvalid(wallet.StatusCode, walletEnvelope) {
		return nil, true, nil
	}
	withdrawable, frozen, err := parseWalletStats(walletEnvelope.Data)
	if err != nil {
		return nil, false, ErrCloudcatMerchantUnavailable.WithCause(err)
	}

	now := s.now
	if now == nil {
		now = time.Now
	}
	syncedAt := now().UTC()
	return &CloudcatMerchantOverview{
		Connected:          true,
		TodayOrderCount:    todayCount,
		TodayOrderAmount:   todayAmount,
		WithdrawableAmount: withdrawable,
		FrozenAmount:       frozen,
		Currency:           "CNY",
		LastSyncedAt:       &syncedAt,
	}, false, nil
}

func (s *CloudcatMerchantService) postJSON(ctx context.Context, path string, payload any, session *cloudcatMerchantSession) (*cloudcatMerchantHTTPResponse, error) {
	endpoint, err := url.Parse(strings.TrimRight(s.baseURL, "/") + "/" + strings.TrimLeft(path, "/"))
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("invalid cloudcat merchant endpoint")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal cloudcat merchant request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cloudcat merchant request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", cloudcatMerchantUserAgent)
	req.Header.Set("Origin", cloudcatMerchantAPIBaseURL)
	req.Header.Set("Referer", cloudcatMerchantAPIBaseURL+"/merchant")
	if session != nil {
		req.Header.Set("Merchant-Token", session.MerchantToken)
		for _, cookie := range session.Cookies {
			if cookie.Name != "" {
				req.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
			}
		}
	}
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	limited := io.LimitReader(resp.Body, cloudcatMerchantMaxResponseSize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read cloudcat merchant response: %w", err)
	}
	if len(raw) > cloudcatMerchantMaxResponseSize {
		return nil, fmt.Errorf("cloudcat merchant response is too large")
	}
	return &cloudcatMerchantHTTPResponse{StatusCode: resp.StatusCode, Body: raw, Cookies: resp.Cookies()}, nil
}

func decodeCloudcatMerchantEnvelope(raw []byte) (cloudcatMerchantEnvelope, error) {
	var envelope cloudcatMerchantEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return cloudcatMerchantEnvelope{}, fmt.Errorf("decode cloudcat merchant response: %w", err)
	}
	return envelope, nil
}

func extractMerchantToken(raw json.RawMessage) string {
	var data struct {
		MerchantToken string `json:"merchant_token"`
		Token         string `json:"token"`
		Data          *struct {
			MerchantToken string `json:"merchant_token"`
			Token         string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &data)
	if data.Data != nil {
		return cloudcatFirstNonEmpty(data.MerchantToken, data.Token, data.Data.MerchantToken, data.Data.Token)
	}
	return cloudcatFirstNonEmpty(data.MerchantToken, data.Token)
}

func cloudcatFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func cloudcatMerchantResponseInvalid(status int, envelope cloudcatMerchantEnvelope) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || envelope.Code != 1
}

// CatFK's checkToken endpoint treats only code 0 as an invalid session. Its
// dashboard endpoints use code 1 for successful data responses, so they need a
// separate predicate above.
func cloudcatMerchantTokenInvalid(status int, envelope cloudcatMerchantEnvelope) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || envelope.Code == 0
}

func parseWorkplaceStats(raw json.RawMessage) (int, float64, error) {
	var data cloudcatMerchantWorkplaceData
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, 0, err
	}
	stats := findMerchantStatistics(&data)
	if stats == nil {
		return 0, 0, fmt.Errorf("cloudcat workplace statistics are missing")
	}
	count, err := parseJSONInt(stats.TodayCount)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cloudcat today order count: %w", err)
	}
	amount, err := parseJSONFloat(stats.TodayAmount)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cloudcat today order amount: %w", err)
	}
	return count, amount, nil
}

func findMerchantStatistics(data *cloudcatMerchantWorkplaceData) *cloudcatMerchantStatistics {
	if data == nil {
		return nil
	}
	if data.Statistics != nil {
		return data.Statistics
	}
	if stats := findMerchantStatistics(data.Data); stats != nil {
		return stats
	}
	return findMerchantStatistics(data.Response)
}

func parseWalletStats(raw json.RawMessage) (float64, float64, error) {
	var data cloudcatMerchantWalletData
	if err := json.Unmarshal(raw, &data); err != nil {
		return 0, 0, err
	}
	platform := findMerchantPlatform(&data)
	if platform == nil {
		return 0, 0, fmt.Errorf("cloudcat wallet platform data is missing")
	}
	available, err := parseJSONFloat(platform.AvailableMoney)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cloudcat available money: %w", err)
	}
	frozen, err := parseJSONFloat(platform.FreezeMoney)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cloudcat frozen money: %w", err)
	}
	return available, frozen, nil
}

func findMerchantPlatform(data *cloudcatMerchantWalletData) *cloudcatMerchantPlatform {
	if data == nil {
		return nil
	}
	if data.Platform != nil {
		return data.Platform
	}
	if platform := findMerchantPlatform(data.Data); platform != nil {
		return platform
	}
	return findMerchantPlatform(data.Response)
}

func parseJSONFloat(raw json.RawMessage) (float64, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return 0, fmt.Errorf("number is missing")
	}
	if strings.HasPrefix(text, "\"") {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return 0, err
		}
		text = strings.TrimSpace(value)
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("invalid number")
	}
	return value, nil
}

func parseJSONInt(raw json.RawMessage) (int, error) {
	value, err := parseJSONFloat(raw)
	if err != nil || math.Trunc(value) != value || value < 0 || value > math.MaxInt {
		return 0, fmt.Errorf("invalid integer")
	}
	return int(value), nil
}

func cookiesFromResponse(cookies []*http.Cookie) []cloudcatMerchantCookie {
	result := make([]cloudcatMerchantCookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" || cookie.Value == "" {
			continue
		}
		result = append(result, cloudcatMerchantCookie{Name: cookie.Name, Value: cookie.Value})
	}
	return result
}

func mergeCookies(session *cloudcatMerchantSession, cookies []*http.Cookie) {
	if session == nil || len(cookies) == 0 {
		return
	}
	byName := make(map[string]int, len(session.Cookies))
	for i, cookie := range session.Cookies {
		byName[cookie.Name] = i
	}
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		if cookie.MaxAge < 0 {
			if index, ok := byName[cookie.Name]; ok {
				session.Cookies = append(session.Cookies[:index], session.Cookies[index+1:]...)
				byName = make(map[string]int, len(session.Cookies))
				for i, item := range session.Cookies {
					byName[item.Name] = i
				}
			}
			continue
		}
		item := cloudcatMerchantCookie{Name: cookie.Name, Value: cookie.Value}
		if index, ok := byName[cookie.Name]; ok {
			session.Cookies[index] = item
		} else {
			byName[cookie.Name] = len(session.Cookies)
			session.Cookies = append(session.Cookies, item)
		}
	}
}

func (s *CloudcatMerchantService) loadSession(ctx context.Context, adminID int64) (*cloudcatMerchantSession, bool, error) {
	raw, err := s.redisClient.Get(ctx, s.sessionKey(adminID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load cloudcat merchant session: %w", err)
	}
	plaintext, err := s.encryptor.Decrypt(string(raw))
	if err != nil {
		_ = s.redisClient.Del(ctx, s.sessionKey(adminID)).Err()
		return nil, false, nil
	}
	var session cloudcatMerchantSession
	if err := json.Unmarshal([]byte(plaintext), &session); err != nil || strings.TrimSpace(session.MerchantToken) == "" {
		_ = s.redisClient.Del(ctx, s.sessionKey(adminID)).Err()
		return nil, false, nil
	}
	return &session, true, nil
}

func (s *CloudcatMerchantService) saveSession(ctx context.Context, adminID int64, session *cloudcatMerchantSession) error {
	if session == nil || strings.TrimSpace(session.MerchantToken) == "" {
		return ErrCloudcatMerchantLoginFailed
	}
	plaintext, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("marshal cloudcat merchant session: %w", err)
	}
	ciphertext, err := s.encryptor.Encrypt(string(plaintext))
	if err != nil {
		return fmt.Errorf("encrypt cloudcat merchant session: %w", err)
	}
	if err := s.redisClient.Set(ctx, s.sessionKey(adminID), ciphertext, s.sessionTTL).Err(); err != nil {
		return fmt.Errorf("save cloudcat merchant session: %w", err)
	}
	return nil
}

func (s *CloudcatMerchantService) deleteSession(ctx context.Context, adminID int64) error {
	if err := s.redisClient.Del(ctx, s.sessionKey(adminID)).Err(); err != nil {
		return fmt.Errorf("delete cloudcat merchant session: %w", err)
	}
	return nil
}
