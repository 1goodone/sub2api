package service

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	accountOpsErrorCloseMin     = 2
	accountOpsErrorCloseMax     = 50
	accountOpsErrorCloseCodeMax = 40
)

// Built-in matches are operational failures: rate limits, overload, stream
// breaks, and transport failures. Quota exhaustion and user-caused requests
// are excluded even when a custom code repeats them.
var accountOpsErrorCloseDefaultCodes = map[string]bool{
	"rate_limit_error": true, "rate_limit_exceeded": true, "ratelimitexceeded": true,
	"overloaded_error": true, "overloaded": true, "server_error": true, "api_error": true,
	"stream_error": true, "stream_timeout": true, "upstream_timeout": true,
}

var accountOpsErrorCloseExcludedCodes = map[string]bool{
	"insufficient_quota": true, "insufficient_balance": true, "balance_not_enough": true,
	"insufficient_credit": true, "credit_balance_too_low": true, "account_balance_insufficient": true,
	"balance_exhausted": true, "weekly_limit_exceeded": true, "weekly_quota_exceeded": true,
	"weekly_quota_exhausted": true, "weekly_usage_limit_reached": true, "quota_exceeded": true,
	"usage_limit_reached": true, "billing_hard_limit_reached": true, "invalid_request_error": true,
	"content_policy_violation": true, "content_filter": true, "moderation_blocked": true,
	"context_length_exceeded": true, "context_length": true, "invalid_prompt": true,
	"image_generation_user_error": true,
}

func normalizeAccountOpsErrorCloseCodes(codes []string) ([]string, error) {
	if len(codes) > accountOpsErrorCloseCodeMax {
		return nil, fmt.Errorf("at most %d custom error codes", accountOpsErrorCloseCodeMax)
	}
	out := make([]string, 0, len(codes))
	seen := map[string]bool{}
	for _, code := range codes {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" || seen[code] {
			continue
		}
		if len(code) > 64 || !opsWindowID.MatchString(code) {
			return nil, fmt.Errorf("invalid error code")
		}
		seen[code] = true
		out = append(out, code)
	}
	return out, nil
}

func accountOpsErrorCloseDecision(platform string, status int, headers http.Header, body []byte, extra []string) string {
	if status > 0 && status < 400 {
		return "reset"
	}
	if kind, _ := classifyAccountOpsFailure(platform, status, headers, body); kind != "" {
		return "ignore"
	}
	codes, message := accountOpsErrorCloseSignals(body)
	if accountOpsErrorCloseUserText(message) {
		return "ignore"
	}
	for _, code := range codes {
		if accountOpsErrorCloseExcludedCodes[code] {
			return "ignore"
		}
	}
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return "ignore"
	}
	for _, code := range codes {
		if accountOpsErrorCloseDefaultCodes[code] || accountOpsErrorCloseListed(extra, code) {
			return "count"
		}
	}
	if accountOpsErrorCloseListed(extra, fmt.Sprintf("%d", status)) {
		return "count"
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return "count"
	default:
		return "ignore"
	}
}

func accountOpsErrorCloseListed(extra []string, code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	for _, item := range extra {
		if strings.ToLower(strings.TrimSpace(item)) == code && code != "" {
			return true
		}
	}
	return false
}

func accountOpsErrorCloseUserText(message string) bool {
	message = strings.ToLower(message)
	for _, phrase := range []string{
		"context window", "context length", "maximum context", "content policy",
		"content filter", "moderation", "invalid request", "prompt is too long",
	} {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

func accountOpsErrorCloseSignals(body []byte) ([]string, string) {
	if len(body) == 0 || len(body) > 32*1024 {
		return nil, ""
	}
	var envelope struct {
		Code    string          `json:"code"`
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil, ""
	}
	message := envelope.Message
	codes := []string{envelope.Code, envelope.Type}
	if len(envelope.Error) > 0 && envelope.Error[0] == '{' {
		var inner struct {
			Code, Type, Message string
		}
		if json.Unmarshal(envelope.Error, &inner) == nil {
			codes = append(codes, inner.Code, inner.Type)
			if inner.Message != "" {
				message = inner.Message
			}
		}
	} else if len(envelope.Error) > 0 && envelope.Error[0] == '"' {
		var text string
		if json.Unmarshal(envelope.Error, &text) == nil {
			message = text
		}
	}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.ToLower(strings.TrimSpace(code))
		if code != "" {
			out = append(out, code)
		}
	}
	return out, message
}

func (s *AccountOpsService) NoteSuccess(accountID int64) {
	if s == nil || accountID <= 0 || !s.currentConfig().ErrorCloseEnabled {
		return
	}
	s.errorCloseMu.Lock()
	delete(s.errorCloseStreak, accountID)
	s.errorCloseMu.Unlock()
}

func (s *AccountOpsService) NoteStreamBreak(account *Account) {
	s.noteErrorClose(account, 0, nil, []byte(`{"error":{"type":"stream_timeout","code":"stream_timeout"}}`))
}

func (s *AccountOpsService) NoteTransportFailure(account *Account) {
	s.noteErrorClose(account, 0, nil, []byte(`{"error":{"type":"upstream_timeout","code":"upstream_timeout"}}`))
}

func (s *AccountOpsService) noteErrorClose(account *Account, status int, headers http.Header, body []byte) {
	if s == nil || account == nil || account.ID <= 0 || account.Status == StatusError {
		return
	}
	cfg := s.currentConfig()
	if !cfg.ErrorCloseEnabled {
		return
	}
	switch accountOpsErrorCloseDecision(account.Platform, status, headers, body, cfg.ErrorCloseCodes) {
	case "reset":
		s.NoteSuccess(account.ID)
	case "count":
		s.advanceErrorClose(account, cfg, status, body)
	}
}

func (s *AccountOpsService) advanceErrorClose(account *Account, cfg AccountOpsConfig, status int, body []byte) {
	threshold := cfg.ErrorCloseConsecutive
	if threshold < accountOpsErrorCloseMin {
		threshold = accountOpsErrorCloseMin
	}
	s.errorCloseMu.Lock()
	if s.errorCloseStreak == nil {
		s.errorCloseStreak = map[int64]int{}
	}
	if s.errorClosePending == nil {
		s.errorClosePending = map[int64]bool{}
	}
	if s.errorClosePending[account.ID] {
		s.errorCloseMu.Unlock()
		return
	}
	s.errorCloseStreak[account.ID]++
	count := s.errorCloseStreak[account.ID]
	if count < threshold {
		s.errorCloseMu.Unlock()
		return
	}
	s.errorCloseStreak[account.ID] = threshold
	s.errorClosePending[account.ID] = true
	s.errorCloseMu.Unlock()

	signal := accountOpsErrorCloseLabel(status, body)
	message := fmt.Sprintf("连续 %d 次账号异常（%s），已自动关闭", threshold, signal)
	err := s.closeAccountForErrorStreak(account.ID, message)
	s.errorCloseMu.Lock()
	delete(s.errorClosePending, account.ID)
	if err != nil {
		if _, ok := s.errorCloseStreak[account.ID]; ok {
			s.errorCloseStreak[account.ID] = threshold
		}
		s.errorCloseMu.Unlock()
		slog.Warn("account_ops_error_close_failed", "account_id", account.ID, "error", err)
		return
	}
	delete(s.errorCloseStreak, account.ID)
	s.errorCloseMu.Unlock()
	slog.Warn("account_ops_error_closed", "account_id", account.ID, "count", count, "signal", signal)
	if cfg.ErrorCloseNotifyEmail && cfg.Recipient != "" && s.email != nil {
		name := account.Name
		recipient := cfg.Recipient
		go s.sendErrorCloseEmail(recipient, name, account.ID, threshold, signal)
	}
}

func (s *AccountOpsService) closeAccountForErrorStreak(id int64, message string) error {
	if s.errorClose != nil {
		return s.errorClose(context.Background(), id, message)
	}
	if s.accounts == nil {
		return fmt.Errorf("account repository unavailable")
	}
	return s.accounts.SetError(context.Background(), id, message)
}

func accountOpsErrorCloseLabel(status int, body []byte) string {
	codes, _ := accountOpsErrorCloseSignals(body)
	for _, code := range codes {
		if code != "" {
			return code
		}
	}
	if status > 0 {
		return fmt.Sprintf("HTTP %d", status)
	}
	return "stream_timeout"
}

func (s *AccountOpsService) sendErrorCloseEmail(recipient, name string, id int64, count int, signal string) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	safeName := html.EscapeString(name)
	safeSignal := html.EscapeString(signal)
	subject := fmt.Sprintf("账号已自动关闭：%s", name)
	body := fmt.Sprintf("<p>账号 %s（ID %d）连续 %d 次出现限流、断流或其他已配置的账号异常，已自动关闭。</p><p>最近信号：%s</p>", safeName, id, count, safeSignal)
	if err := s.email.SendEmail(ctx, recipient, subject, body); err != nil {
		slog.Warn("account_ops_error_close_email_failed", "account_id", id, "error", err)
	}
}

func (s *RateLimitService) NoteAccountOpsSuccess(account *Account) {
	if s == nil || s.accountOps == nil || account == nil {
		return
	}
	s.accountOps.NoteSuccess(account.ID)
}

func (s *RateLimitService) NoteAccountOpsTransportFailure(account *Account) {
	if s == nil || s.accountOps == nil || account == nil {
		return
	}
	s.accountOps.NoteTransportFailure(account)
}

func (s *GatewayService) NoteAccountOpsSuccess(account *Account) {
	if s == nil || s.rateLimitService == nil {
		return
	}
	s.rateLimitService.NoteAccountOpsSuccess(account)
}

func (s *OpenAIGatewayService) NoteAccountOpsSuccess(account *Account) {
	if s == nil || s.rateLimitService == nil {
		return
	}
	s.rateLimitService.NoteAccountOpsSuccess(account)
}
