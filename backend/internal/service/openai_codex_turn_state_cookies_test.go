package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func liveTestRoutingCookies(now time.Time) codexTurnStateRoutingCookies {
	return codexTurnStateRoutingCookies{CFLB: "cf-test", OAILB: "oai-test", ExpiresAt: now.Add(2 * time.Minute)}
}

func TestCodexRoutingCookiesUseTheEarlierExpiry(t *testing.T) {
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	header := http.Header{}
	header.Add("Set-Cookie", "__cf_lb=cf-value; Max-Age=120; Path=/; HttpOnly")
	header.Add("Set-Cookie", "__oai_lb=oai-value; Max-Age=90; Path=/; HttpOnly")
	header.Add("Set-Cookie", "other=keep; Path=/")
	got, ok := codexRoutingCookiesFromResponse(header, now)
	require.True(t, ok)
	require.Equal(t, "cf-value", got.CFLB)
	require.Equal(t, "oai-value", got.OAILB)
	require.Equal(t, now.Add(90*time.Second), got.ExpiresAt)

	missing := header.Clone()
	missing.Del("Set-Cookie")
	missing.Add("Set-Cookie", "__cf_lb=only-cf; Path=/")
	_, ok = codexRoutingCookiesFromResponse(missing, now)
	require.False(t, ok)

	bare := http.Header{}
	bare.Add("Set-Cookie", "__cf_lb=cf-bare; Path=/")
	bare.Add("Set-Cookie", "__oai_lb=oai-bare; Path=/")
	got, ok = codexRoutingCookiesFromResponse(bare, now)
	require.True(t, ok)
	require.Equal(t, now.Add(codexRoutingCookieDefaultTTL), got.ExpiresAt)
}

func TestMergeCodexRoutingCookiesKeepsUnrelatedPairs(t *testing.T) {
	merged := mergeCodexRoutingCookieHeader("session=abc; __cf_lb=old; theme=dark", codexTurnStateRoutingCookies{CFLB: "cf-new", OAILB: "oai-new"})
	require.Equal(t, "session=abc; theme=dark; __cf_lb=cf-new; __oai_lb=oai-new", merged)
	require.NotContains(t, merged, "old")
}

func TestCodexStateModeRefreshesDuringTheLastMinute(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fresh := poolEntryExpiring("plus", now, 90*time.Second)
	require.Equal(t, "idle", codexStateMode(fresh, 292, "tok", now))
	due := poolEntryExpiring("plus", now, 20*time.Second)
	require.Equal(t, "continuous", codexStateMode(due, 292, "tok", now))
	require.False(t, codexRoutingCookieDue(map[string]any{CodexTurnStateRoutingCookieExtraKey: liveTestRoutingCookies(now).extraValue()}, now))
	require.True(t, codexRoutingCookieDue(nil, now))
}

func TestStatePoolOmitsTicketWhenRoutingCookieIsDead(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := poolTestAccount("plus")
	delete(account.Extra, CodexTurnStateRoutingCookieExtraKey)
	ctx := svc.PinCodexTurnStateOwner(context.Background(), account)
	header := http.Header{}
	header.Set(openAICodexTurnStateHeader, "client-state")
	header.Set("Cookie", "session=abc")
	require.NoError(t, svc.applyCodexTurnStateForHTTPRequest(ctx, account, "gpt-6-astra", "tok", header))
	require.Empty(t, header.Get(openAICodexTurnStateHeader))
	require.Equal(t, "session=abc", header.Get("Cookie"))

	frame := []byte(`{"type":"response.create","model":"gpt-6-astra","client_metadata":{"keep":"value"}}`)
	out, err := applyCodexTurnStateToWSFrame(account, CodexTurnStateModeStatePool, "tok", frame)
	require.NoError(t, err)
	require.Equal(t, frame, out)
}

func TestStatePoolSendsTicketWithAccountRoutingCookies(t *testing.T) {
	svc := ticketTestService(t, config.OpenAICodexTicketConfig{Enabled: true, FailClosed: true}, nil)
	account := poolTestAccount("plus")
	ctx := svc.PinCodexTurnStateOwner(context.Background(), account)
	header := http.Header{}
	header.Set("Cookie", "session=abc; __cf_lb=stale")
	require.NoError(t, svc.applyCodexTurnStateForHTTPRequest(ctx, account, "gpt-6-astra", "tok", header))
	require.Equal(t, codexTurnStateCacheFromExtra(account.Extra)["gpt-6-astra"].State, header.Get(openAICodexTurnStateHeader))
	require.Contains(t, header.Get("Cookie"), "session=abc")
	require.Contains(t, header.Get("Cookie"), "__cf_lb=cf-test")
	require.Contains(t, header.Get("Cookie"), "__oai_lb=oai-test")
	require.NotContains(t, header.Get("Cookie"), "stale")
}
