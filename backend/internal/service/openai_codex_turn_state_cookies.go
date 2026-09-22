package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

const (
	// CodexTurnStateRoutingCookieExtraKey is the account-level __cf_lb / __oai_lb
	// pair. It is not stored on each ticket. Business traffic sends whatever pair
	// is still live; a dead pair means the cached ticket is not injected.
	CodexTurnStateRoutingCookieExtraKey = "codex_turn_state_routing_cookies"

	codexRoutingCookieCFLB          = "__cf_lb"
	codexRoutingCookieOAILB         = "__oai_lb"
	codexRoutingCookieDefaultTTL    = 180 * time.Second
	codexRoutingCookieRefreshBefore = 30 * time.Second
	codexRoutingCookieValueLimit    = 512
)

// codexTurnStateRoutingCookies is one shared routing credential for the account.
// Ticket expiry and this expiry are independent.
type codexTurnStateRoutingCookies struct {
	CFLB      string
	OAILB     string
	ExpiresAt time.Time
}

func (c codexTurnStateRoutingCookies) complete() bool {
	return validCodexRoutingCookieValue(c.CFLB) && validCodexRoutingCookieValue(c.OAILB) && !c.ExpiresAt.IsZero()
}

func (c codexTurnStateRoutingCookies) live(now time.Time) bool {
	return c.complete() && now.Before(c.ExpiresAt)
}

func validCodexRoutingCookieValue(value string) bool {
	if value == "" || len(value) > codexRoutingCookieValueLimit {
		return false
	}
	for _, r := range value {
		if r <= ' ' || r == ';' || r == ',' || r == '\\' {
			return false
		}
	}
	return true
}

func (c codexTurnStateRoutingCookies) extraValue() map[string]any {
	if !c.complete() {
		return nil
	}
	return map[string]any{
		"cf_lb":      c.CFLB,
		"oai_lb":     c.OAILB,
		"expires_at": c.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
}

func codexRoutingCookiesFromExtra(extra map[string]any) (codexTurnStateRoutingCookies, bool) {
	if extra == nil {
		return codexTurnStateRoutingCookies{}, false
	}
	raw, ok := extra[CodexTurnStateRoutingCookieExtraKey]
	if !ok || raw == nil {
		return codexTurnStateRoutingCookies{}, false
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return codexTurnStateRoutingCookies{}, false
	}
	cf, _ := values["cf_lb"].(string)
	oai, _ := values["oai_lb"].(string)
	expiresAt, _ := parseCodexTurnStateTime(values["expires_at"])
	cookies := codexTurnStateRoutingCookies{CFLB: strings.TrimSpace(cf), OAILB: strings.TrimSpace(oai), ExpiresAt: expiresAt}
	if !cookies.complete() {
		return codexTurnStateRoutingCookies{}, false
	}
	return cookies, true
}

func codexRoutingCookiesLive(account *Account, now time.Time) bool {
	cookies, ok := codexRoutingCookiesFromExtra(accountExtra(account))
	return ok && cookies.live(now)
}

// codexRoutingCookieDue is true when the pair is missing, expired, or inside
// the refresh lead. The ticket clock does not affect this.
func codexRoutingCookieDue(extra map[string]any, now time.Time) bool {
	cookies, ok := codexRoutingCookiesFromExtra(extra)
	if !ok || !cookies.live(now) {
		return true
	}
	return !cookies.ExpiresAt.After(now.Add(codexRoutingCookieRefreshBefore))
}

func codexRoutingCookiesFromResponse(header http.Header, now time.Time) (codexTurnStateRoutingCookies, bool) {
	if header == nil {
		return codexTurnStateRoutingCookies{}, false
	}
	var cf, oai string
	var cfExp, oaiExp time.Time
	var cfOK, oaiOK bool
	for _, raw := range header.Values("Set-Cookie") {
		parsed, err := http.ParseSetCookie(raw)
		if err != nil || parsed == nil || !validCodexRoutingCookieValue(parsed.Value) {
			continue
		}
		exp := routingCookieExpiry(parsed, now)
		if !exp.After(now) {
			continue
		}
		switch parsed.Name {
		case codexRoutingCookieCFLB:
			cf, cfExp, cfOK = parsed.Value, exp, true
		case codexRoutingCookieOAILB:
			oai, oaiExp, oaiOK = parsed.Value, exp, true
		}
	}
	if !cfOK || !oaiOK {
		return codexTurnStateRoutingCookies{}, false
	}
	exp := cfExp
	if oaiExp.Before(exp) {
		exp = oaiExp
	}
	return codexTurnStateRoutingCookies{CFLB: cf, OAILB: oai, ExpiresAt: exp}, true
}

func routingCookieExpiry(cookie *http.Cookie, now time.Time) time.Time {
	if cookie == nil {
		return time.Time{}
	}
	if cookie.MaxAge > 0 {
		return now.Add(time.Duration(cookie.MaxAge) * time.Second)
	}
	if cookie.MaxAge < 0 {
		return time.Time{}
	}
	if !cookie.Expires.IsZero() {
		return cookie.Expires.UTC()
	}
	return now.Add(codexRoutingCookieDefaultTTL)
}

func applyCodexRoutingCookies(header http.Header, account *Account, now time.Time) {
	if header == nil {
		return
	}
	cookies, ok := codexRoutingCookiesFromExtra(accountExtra(account))
	if !ok || !cookies.live(now) {
		return
	}
	header.Set("Cookie", mergeCodexRoutingCookieHeader(header.Get("Cookie"), cookies))
}

func mergeCodexRoutingCookieHeader(existing string, cookies codexTurnStateRoutingCookies) string {
	parts := strings.Split(existing, ";")
	kept := make([]string, 0, len(parts)+2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, _, _ := strings.Cut(part, "=")
		switch strings.TrimSpace(name) {
		case codexRoutingCookieCFLB, codexRoutingCookieOAILB:
			continue
		default:
			kept = append(kept, part)
		}
	}
	kept = append(kept, codexRoutingCookieCFLB+"="+cookies.CFLB, codexRoutingCookieOAILB+"="+cookies.OAILB)
	return strings.Join(kept, "; ")
}

func (s *CodexTurnStateProbeService) saveRoutingCookiesOn(ctx context.Context, current *Account, cookies codexTurnStateRoutingCookies) (bool, error) {
	if s == nil || current == nil || !cookies.complete() {
		return false, nil
	}
	updates := codexTurnStateProbeRuntimeUpdates(codexTurnStateCacheFromExtra(current.Extra), codexTurnStateProbeFailuresFromExtra(current.Extra))
	updates[CodexTurnStateRoutingCookieExtraKey] = cookies.extraValue()
	return saveCodexTurnStateProbeRuntime(ctx, s.accountRepo, current, updates)
}

func (s *CodexTurnStateProbeService) persistRoutingCookies(ctx context.Context, expected *Account, cookies codexTurnStateRoutingCookies) bool {
	if s == nil || expected == nil || s.accountRepo == nil || !cookies.complete() {
		return false
	}
	for range 3 {
		current, err := s.accountRepo.GetByID(ctx, expected.ID)
		if err != nil || current == nil || !codexTurnStateProbeEnabled(current) || current.Status != StatusActive || codexTurnStateOwnerIdentity(current) != codexTurnStateOwnerIdentity(expected) {
			return false
		}
		saved, err := s.saveRoutingCookiesOn(ctx, current, cookies)
		if err != nil {
			return false
		}
		if saved {
			return true
		}
	}
	return false
}
