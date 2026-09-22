package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// CodexTurnStateOutboundIdentityExtraKey remains so older rows still parse.
	// Business traffic does not copy it onto the request. A ticket is injected
	// without requiring the session, thread, or window that minted it.
	CodexTurnStateOutboundIdentityExtraKey = "codex_turn_state_outbound_identity"

	// Kept for the unused harvest-phase helper. Ticket lifetime is
	// codexTurnStateTTL, so these windows no longer gate refresh.
	codexTurnStateOldSessionWindow   = 10 * time.Minute
	codexTurnStateHarvestRandomAfter = 20 * time.Minute

	codexTurnStateHarvestOld    = "old"
	codexTurnStateHarvestRandom = "random"
)

// codexTurnStateOutboundIdentity is the outbound Codex session bound to a
// ticket bundle. turn_id is not part of it and stays per request.
type codexTurnStateOutboundIdentity struct {
	SessionID string
	ThreadID  string
	WindowID  string
	ExpiresAt time.Time
}

func (id codexTurnStateOutboundIdentity) complete() bool {
	return id.SessionID != "" && id.ThreadID != "" && id.WindowID != ""
}

func (id codexTurnStateOutboundIdentity) live(now time.Time) bool {
	return id.complete() && !id.ExpiresAt.IsZero() && now.Before(id.ExpiresAt)
}

func (id codexTurnStateOutboundIdentity) extraValue() map[string]any {
	if !id.complete() {
		return nil
	}
	return map[string]any{
		"session_id": id.SessionID,
		"thread_id":  id.ThreadID,
		"window_id":  id.WindowID,
		"expires_at": id.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
}

func codexTurnStateOutboundIdentityFromExtra(extra map[string]any) (codexTurnStateOutboundIdentity, bool) {
	if extra == nil {
		return codexTurnStateOutboundIdentity{}, false
	}
	raw, ok := extra[CodexTurnStateOutboundIdentityExtraKey]
	if !ok || raw == nil {
		return codexTurnStateOutboundIdentity{}, false
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return codexTurnStateOutboundIdentity{}, false
	}
	sessionID, _ := values["session_id"].(string)
	threadID, _ := values["thread_id"].(string)
	windowID, _ := values["window_id"].(string)
	expiresAt, _ := parseCodexTurnStateTime(values["expires_at"])
	identity := codexTurnStateOutboundIdentity{
		SessionID: strings.TrimSpace(sessionID),
		ThreadID:  strings.TrimSpace(threadID),
		WindowID:  strings.TrimSpace(windowID),
		ExpiresAt: expiresAt,
	}
	if !identity.complete() {
		return codexTurnStateOutboundIdentity{}, false
	}
	return identity, true
}

func newCodexTurnStateRandomIdentity() codexTurnStateOutboundIdentity {
	sessionID := uuid.NewString()
	return codexTurnStateOutboundIdentity{
		SessionID: sessionID,
		ThreadID:  sessionID,
		WindowID:  sessionID + ":0",
	}
}

// codexTurnStateFingerprintIdentity is the account-stable full-mode session.
// Harvest can keep using it for the first 10 minutes of a refresh because
// business traffic already sends that set. Session mode is not reusable: its
// thread id follows the client, which a probe does not have.
func codexTurnStateFingerprintIdentity(account *Account) (codexTurnStateOutboundIdentity, bool) {
	if account == nil || account.GetCodexFingerprintMode() != codexFingerprintFull {
		return codexTurnStateOutboundIdentity{}, false
	}
	ids := resolveCodexFingerprintIDs(account, "", codexFingerprintFull)
	if ids == nil {
		return codexTurnStateOutboundIdentity{}, false
	}
	identity := codexTurnStateOutboundIdentity{
		SessionID: strings.TrimSpace(ids.sessionID),
		ThreadID:  strings.TrimSpace(ids.threadID),
		WindowID:  strings.TrimSpace(ids.windowID),
	}
	if !identity.complete() {
		return codexTurnStateOutboundIdentity{}, false
	}
	return identity, true
}

// codexTurnStateHarvestPhase reports whether the next probe should keep the
// current session or start the single replacement session.
// implicitOld is the fingerprint-full session, used only when nothing has been
// stored yet. implicitBudgetOpen is the cold-start 10 minute try of that session.
func codexTurnStateHarvestPhase(stored codexTurnStateOutboundIdentity, earliest time.Time, hasValid, implicitOld, implicitBudgetOpen bool, now time.Time) string {
	if stored.live(now) {
		anchor := stored.ExpiresAt
		if hasValid && !earliest.IsZero() {
			anchor = earliest
		}
		if anchor.Sub(now) > codexTurnStateHarvestRandomAfter {
			return codexTurnStateHarvestOld
		}
		return codexTurnStateHarvestRandom
	}
	if implicitOld {
		if hasValid && !earliest.IsZero() {
			if earliest.Sub(now) > codexTurnStateHarvestRandomAfter {
				return codexTurnStateHarvestOld
			}
			return codexTurnStateHarvestRandom
		}
		if implicitBudgetOpen {
			return codexTurnStateHarvestOld
		}
	}
	return codexTurnStateHarvestRandom
}

func earliestCodexTurnStateExpiry(cache map[string]codexTurnStateCacheEntry, expected int, token string, now time.Time) (time.Time, bool) {
	var earliest time.Time
	found := false
	for _, entry := range cache {
		if !validCodexTurnStateCacheEntry(entry, expected, token, now) {
			continue
		}
		expires := codexStateEffectiveExpiry(entry, now)
		if expires.IsZero() {
			continue
		}
		if !found || expires.Before(earliest) {
			earliest = expires
			found = true
		}
	}
	return earliest, found
}

// codexTurnStateBundleReady is true when a not-yet-published session has a
// fresh ticket for every model that still has a live ticket. With no live
// ticket, the first fresh pending ticket is enough to install the bundle.
// Models that never return a healthy ticket are not required.
func codexTurnStateBundleReady(live, pending map[string]codexTurnStateCacheEntry, expected int, token string, now time.Time) bool {
	if len(pending) == 0 {
		return false
	}
	required := 0
	for model, entry := range live {
		if !validCodexTurnStateCacheEntry(entry, expected, token, now) {
			continue
		}
		required++
		got, ok := pending[model]
		if !ok || !validCodexTurnStateCacheEntry(got, expected, token, now) {
			return false
		}
	}
	if required > 0 {
		return true
	}
	for _, entry := range pending {
		if validCodexTurnStateCacheEntry(entry, expected, token, now) {
			return true
		}
	}
	return false
}

type codexTurnStatePendingBundle struct {
	identity        codexTurnStateOutboundIdentity
	tickets         map[string]codexTurnStateCacheEntry
	implicitStarted time.Time
}

func (s *CodexTurnStateProbeService) harvestDecisionLocked(account *Account, token string, now time.Time) (phase string, writeThrough bool, implicit codexTurnStateOutboundIdentity) {
	expected := CodexTurnStateHealthyLength(account)
	cache := codexTurnStateCacheFromExtra(account.Extra)
	earliest, hasValid := earliestCodexTurnStateExpiry(cache, expected, token, now)
	stored, _ := codexTurnStateOutboundIdentityFromExtra(account.Extra)
	implicit, hasImplicit := codexTurnStateFingerprintIdentity(account)
	started := time.Time{}
	if s.outboundPending != nil {
		if bundle := s.outboundPending[account.ID]; bundle != nil {
			started = bundle.implicitStarted
		}
	}
	budgetOpen := started.IsZero() || now.Sub(started) < codexTurnStateOldSessionWindow
	phase = codexTurnStateHarvestPhase(stored, earliest, hasValid, hasImplicit, budgetOpen, now)
	writeThrough = phase == codexTurnStateHarvestOld && stored.live(now)
	if phase == codexTurnStateHarvestOld && !writeThrough && !implicit.complete() {
		phase = codexTurnStateHarvestRandom
	}
	return phase, writeThrough, implicit
}

// prepareHarvestIdentity returns the session the probe must send.
// writeThrough means the session is already the account's published identity,
// so a newer ticket updates that model's cache in place. Otherwise the ticket
// stays pending until the whole bundle can be swapped.
func (s *CodexTurnStateProbeService) prepareHarvestIdentity(account *Account, token string, now time.Time) (codexTurnStateOutboundIdentity, bool) {
	if s == nil || account == nil {
		return codexTurnStateOutboundIdentity{}, false
	}
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	if s.outboundPending == nil {
		s.outboundPending = map[int64]*codexTurnStatePendingBundle{}
	}
	phase, writeThrough, implicit := s.harvestDecisionLocked(account, token, now)
	if writeThrough {
		delete(s.outboundPending, account.ID)
		stored, _ := codexTurnStateOutboundIdentityFromExtra(account.Extra)
		return stored, true
	}
	bundle := s.outboundPending[account.ID]
	if bundle == nil {
		bundle = &codexTurnStatePendingBundle{}
		s.outboundPending[account.ID] = bundle
	}
	expected := CodexTurnStateHealthyLength(account)
	_, hasValid := earliestCodexTurnStateExpiry(codexTurnStateCacheFromExtra(account.Extra), expected, token, now)
	if phase == codexTurnStateHarvestOld {
		if bundle.identity.SessionID != implicit.SessionID {
			bundle.tickets = nil
			bundle.identity = implicit
		}
		if !hasValid && bundle.implicitStarted.IsZero() {
			bundle.implicitStarted = now
		}
		return bundle.identity, false
	}
	// Closing the cold-start budget here stops an expired ticket from sending
	// the next probe back to the fingerprint session after a random one exists.
	if bundle.implicitStarted.IsZero() {
		bundle.implicitStarted = now.Add(-codexTurnStateOldSessionWindow - time.Second)
	}
	if !bundle.identity.complete() || (implicit.complete() && bundle.identity.SessionID == implicit.SessionID) {
		bundle.identity = newCodexTurnStateRandomIdentity()
		bundle.tickets = nil
	}
	return bundle.identity, false
}

func (s *CodexTurnStateProbeService) codexTurnStateHarvestCollection(account *Account, token string, now time.Time) (collecting bool, fresh map[string]struct{}) {
	fresh = map[string]struct{}{}
	if s == nil || account == nil {
		return false, fresh
	}
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	phase, writeThrough, implicit := s.harvestDecisionLocked(account, token, now)
	if writeThrough {
		return false, fresh
	}
	bundle := s.outboundPending[account.ID]
	if bundle == nil || !bundle.identity.complete() {
		return true, fresh
	}
	if phase == codexTurnStateHarvestOld && bundle.identity.SessionID != implicit.SessionID {
		return true, fresh
	}
	if phase == codexTurnStateHarvestRandom && implicit.complete() && bundle.identity.SessionID == implicit.SessionID {
		return true, fresh
	}
	expected := CodexTurnStateHealthyLength(account)
	for model, entry := range bundle.tickets {
		if validCodexTurnStateCacheEntry(entry, expected, token, now) {
			fresh[model] = struct{}{}
		}
	}
	return true, fresh
}

func applyCodexTurnStateOutboundIdentityHeaders(h http.Header, identity codexTurnStateOutboundIdentity) {
	if h == nil || !identity.complete() {
		return
	}
	h.Set("session-id", identity.SessionID)
	h.Set("session_id", identity.SessionID)
	h.Set("thread-id", identity.ThreadID)
	h.Set("x-codex-window-id", identity.WindowID)
	h.Set("x-client-request-id", identity.ThreadID)
	fields := map[string]any{
		"session_id": identity.SessionID,
		"thread_id":  identity.ThreadID,
		"window_id":  identity.WindowID,
	}
	if strings.TrimSpace(h.Get("x-codex-turn-metadata")) == "" {
		if raw, err := json.Marshal(fields); err == nil {
			h.Set("x-codex-turn-metadata", string(raw))
		}
		return
	}
	rewriteCodexTurnMetadataFields(h, fields)
}

func applyLiveCodexTurnStateOutboundIdentityHeaders(http.Header, *Account, time.Time) {
}

func accountExtra(account *Account) map[string]any {
	if account == nil {
		return nil
	}
	return account.Extra
}

func overlayCodexTurnStateOutboundIdentityMetadata(existing map[string]any, identity codexTurnStateOutboundIdentity, ensureEmbedded bool) {
	if existing == nil || !identity.complete() {
		return
	}
	existing["session_id"] = identity.SessionID
	existing["thread_id"] = identity.ThreadID
	existing["x-codex-window-id"] = identity.WindowID
	fields := map[string]any{
		"session_id": identity.SessionID,
		"thread_id":  identity.ThreadID,
		"window_id":  identity.WindowID,
	}
	raw, ok := existing["x-codex-turn-metadata"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		if ensureEmbedded {
			if rebuilt, err := json.Marshal(fields); err == nil {
				existing["x-codex-turn-metadata"] = string(rebuilt)
			}
		}
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		metadata = make(map[string]any, len(fields))
	}
	for key, value := range fields {
		metadata[key] = value
	}
	if rebuilt, err := json.Marshal(metadata); err == nil {
		existing["x-codex-turn-metadata"] = string(rebuilt)
	}
}

func applyCodexTurnStateOutboundIdentityProbePayload(payload map[string]any, identity codexTurnStateOutboundIdentity) {
	if payload == nil || !identity.complete() {
		return
	}
	existing, _ := payload["client_metadata"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	overlayCodexTurnStateOutboundIdentityMetadata(existing, identity, true)
	payload["client_metadata"] = existing
}

func applyLiveCodexTurnStateOutboundIdentityClientMetadata(*Account, map[string]any, time.Time) {
}

func applyCodexTurnStateOutboundIdentityBody(_ *Account, body []byte, _ time.Time) ([]byte, bool, error) {
	return body, false, nil
}

func codexTurnStateOutboundIdentityHeadersMatch(h http.Header, identity codexTurnStateOutboundIdentity) bool {
	if h == nil || !identity.complete() {
		return false
	}
	return h.Get("session_id") == identity.SessionID &&
		h.Get("session-id") == identity.SessionID &&
		h.Get("thread-id") == identity.ThreadID &&
		h.Get("x-codex-window-id") == identity.WindowID
}

func alignCodexTurnStateOutboundIdentityRequest(*http.Request, *Account, time.Time) error {
	return nil
}
