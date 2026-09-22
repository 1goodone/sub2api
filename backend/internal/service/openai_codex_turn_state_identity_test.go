package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func poolEntryExpiring(plan string, now time.Time, remaining time.Duration) codexTurnStateCacheEntry {
	obtained := now.Add(-time.Minute)
	return codexTurnStateCacheEntry{
		State:          poolTestState(plan, obtained),
		ObtainedAt:     obtained,
		ExpiresAt:      now.Add(remaining),
		ProxyID:        1,
		CredentialHash: codexTurnStateCredentialHash("tok"),
		Source:         "automatic",
	}
}

func TestCodexTurnStateHarvestPhaseUsesOldSessionForFirstTenMinutes(t *testing.T) {
	now := time.Date(2026, 9, 22, 4, 0, 0, 0, time.UTC)
	stored := codexTurnStateOutboundIdentity{SessionID: "sess", ThreadID: "sess", WindowID: "sess:0", ExpiresAt: now.Add(time.Hour)}
	require.Equal(t, codexTurnStateHarvestOld, codexTurnStateHarvestPhase(stored, now.Add(21*time.Minute), true, false, false, now))
	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(stored, now.Add(20*time.Minute), true, false, false, now))
	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(stored, now.Add(19*time.Minute), true, false, false, now))

	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(codexTurnStateOutboundIdentity{}, now.Add(21*time.Minute), true, false, false, now))
	require.Equal(t, codexTurnStateHarvestOld, codexTurnStateHarvestPhase(codexTurnStateOutboundIdentity{}, now.Add(21*time.Minute), true, true, false, now))
	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(codexTurnStateOutboundIdentity{}, now.Add(19*time.Minute), true, true, false, now))

	deleted := stored
	deleted.ExpiresAt = now.Add(21 * time.Minute)
	require.Equal(t, codexTurnStateHarvestOld, codexTurnStateHarvestPhase(deleted, time.Time{}, false, false, false, now))
	deleted.ExpiresAt = now.Add(19 * time.Minute)
	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(deleted, time.Time{}, false, false, false, now))

	require.Equal(t, codexTurnStateHarvestOld, codexTurnStateHarvestPhase(codexTurnStateOutboundIdentity{}, time.Time{}, false, true, true, now))
	require.Equal(t, codexTurnStateHarvestRandom, codexTurnStateHarvestPhase(codexTurnStateOutboundIdentity{}, time.Time{}, false, true, false, now))
}

func TestCodexTurnStateBundleReadyWaitsForEveryLiveModel(t *testing.T) {
	now := time.Now().UTC()
	live := map[string]codexTurnStateCacheEntry{
		"gpt-5.6-sol": poolEntryExpiring("plus", now, 90*time.Second),
		"gpt-6-astra": poolEntryExpiring("plus", now, 90*time.Second),
	}
	one := map[string]codexTurnStateCacheEntry{"gpt-5.6-sol": poolEntryExpiring("plus", now, 2*time.Minute)}
	require.False(t, codexTurnStateBundleReady(live, one, 292, "tok", now))
	both := map[string]codexTurnStateCacheEntry{
		"gpt-5.6-sol": poolEntryExpiring("plus", now, 2*time.Minute),
		"gpt-6-astra": poolEntryExpiring("plus", now, 2*time.Minute),
	}
	require.True(t, codexTurnStateBundleReady(live, both, 292, "tok", now))
	require.True(t, codexTurnStateBundleReady(nil, one, 292, "tok", now))
	stale := map[string]codexTurnStateCacheEntry{"gpt-5.6-sol": poolEntryExpiring("plus", now, -time.Minute)}
	require.False(t, codexTurnStateBundleReady(nil, stale, 292, "tok", now))
}

func TestCodexTurnStateOutboundIdentityKeepsTurnID(t *testing.T) {
	now := time.Now().UTC()
	identity := codexTurnStateOutboundIdentity{SessionID: "sess-new", ThreadID: "sess-new", WindowID: "sess-new:0", ExpiresAt: now.Add(time.Hour)}
	header := http.Header{}
	header.Set("x-codex-window-id", "random-window")
	header.Set("x-codex-turn-metadata", `{"turn_id":"keep-me","session_id":"old","thread_id":"old","window_id":"old:0","turn_started_at_unix_ms":5}`)
	applyCodexTurnStateOutboundIdentityHeaders(header, identity)
	require.Equal(t, "sess-new", header.Get("session_id"))
	require.Equal(t, "sess-new", header.Get("session-id"))
	require.Equal(t, "sess-new", header.Get("thread-id"))
	require.Equal(t, "sess-new:0", header.Get("x-codex-window-id"))
	require.Equal(t, "sess-new", header.Get("x-client-request-id"))
	metadata := header.Get("x-codex-turn-metadata")
	require.Equal(t, "keep-me", gjson.Get(metadata, "turn_id").String())
	require.Equal(t, "sess-new", gjson.Get(metadata, "session_id").String())
	require.Equal(t, "sess-new:0", gjson.Get(metadata, "window_id").String())
	require.Equal(t, int64(5), gjson.Get(metadata, "turn_started_at_unix_ms").Int())

	account := ticketTestAccount(41)
	account.Extra = map[string]any{CodexTurnStateOutboundIdentityExtraKey: identity.extraValue()}
	body, err := json.Marshal(map[string]any{
		"model": "gpt-6-astra",
		"client_metadata": map[string]any{
			"session_id":            "old",
			"thread_id":             "old",
			"turn_id":               "keep",
			"x-codex-window-id":     "old:0",
			"x-codex-turn-metadata": `{"turn_id":"keep-me","session_id":"old","thread_id":"old","window_id":"old:0","turn_started_at_unix_ms":5}`,
		},
	})
	require.NoError(t, err)
	rewritten, changed, err := applyCodexTurnStateOutboundIdentityBody(account, body, now)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, rewritten)
	require.Equal(t, "old", gjson.GetBytes(rewritten, "client_metadata.session_id").String())
	require.Equal(t, "keep", gjson.GetBytes(rewritten, "client_metadata.turn_id").String())
	header.Set("session_id", "client-session")
	applyLiveCodexTurnStateOutboundIdentityHeaders(header, account, now)
	require.Equal(t, "client-session", header.Get("session_id"))
}

func TestAcquireStateSendsPinnedSessionInsteadOfProbeWindow(t *testing.T) {
	var header http.Header
	var body []byte
	svc := &CodexTurnStateProbeService{requestDo: func(req *http.Request, _ string) (*http.Response, error) {
		header = req.Header.Clone()
		var err error
		body, err = io.ReadAll(req.Body)
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}}
	account := ticketTestAccount(41)
	proxy := &Proxy{Protocol: "http", Host: "127.0.0.1", Port: 8080}
	identity := codexTurnStateOutboundIdentity{SessionID: "sess-pin", ThreadID: "sess-pin", WindowID: "sess-pin:0"}
	out := svc.acquireState(context.Background(), account, proxy, "tok", "gpt-5.6-sol", "", identity)
	require.Error(t, out.err)
	require.Equal(t, "sess-pin", header.Get("session_id"))
	require.Equal(t, "sess-pin", header.Get("thread-id"))
	require.Equal(t, "sess-pin:0", header.Get("x-codex-window-id"))
	require.Equal(t, "sess-pin", gjson.GetBytes(body, "client_metadata.session_id").String())
	require.Equal(t, "sess-pin", gjson.GetBytes(body, "client_metadata.thread_id").String())
	require.Equal(t, "sess-pin:0", gjson.GetBytes(body, "client_metadata.x-codex-window-id").String())

	header = nil
	body = nil
	out = svc.acquireState(context.Background(), account, proxy, "tok", "gpt-5.6-sol", "", codexTurnStateOutboundIdentity{})
	require.Error(t, out.err)
	require.Empty(t, header.Get("session_id"))
	require.NotEmpty(t, header.Get("x-codex-window-id"))
	require.False(t, gjson.GetBytes(body, "client_metadata").Exists())
}

func TestPrepareHarvestIdentityKeepsOneReplacementSession(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	account := poolTestAccount("plus")
	account.Extra[codexFingerprintModeExtraKey] = string(codexFingerprintFull)
	account.Extra[codexFingerprintSeedExtraKey] = "11111111-1111-4111-8111-111111111111"
	account.Extra[CodexTurnStateProbeCacheExtraKey] = codexTurnStateCacheToExtra(map[string]codexTurnStateCacheEntry{
		"gpt-6-astra": poolEntryExpiring("plus", now, 90*time.Second),
	})
	svc := &CodexTurnStateProbeService{}
	fingerprint, ok := codexTurnStateFingerprintIdentity(account)
	require.True(t, ok)
	first, writeThrough := svc.prepareHarvestIdentity(account, "tok", now)
	require.False(t, writeThrough)
	require.NotEqual(t, fingerprint.SessionID, first.SessionID)
	require.Equal(t, first.SessionID, first.ThreadID)
	require.Equal(t, first.SessionID+":0", first.WindowID)
	second, _ := svc.prepareHarvestIdentity(account, "tok", now)
	require.Equal(t, first.SessionID, second.SessionID)

	stored := codexTurnStateOutboundIdentity{SessionID: "stored-sess", ThreadID: "stored-sess", WindowID: "stored-sess:0", ExpiresAt: now.Add(time.Hour)}
	account.Extra[CodexTurnStateOutboundIdentityExtraKey] = stored.extraValue()
	published, writeThrough := svc.prepareHarvestIdentity(account, "tok", now)
	require.False(t, writeThrough)
	require.Equal(t, first.SessionID, published.SessionID)
}
