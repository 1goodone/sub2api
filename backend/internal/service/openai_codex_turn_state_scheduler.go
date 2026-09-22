package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CodexStateImportJob struct {
	ID        string    `json:"id"`
	AccountID int64     `json:"account_id"`
	Model     string    `json:"model"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	stateHash string
}

func (s *CodexTurnStateProbeService) QueueImportState(ctx context.Context, id int64, model, state string) (*CodexStateImportJob, error) {
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	model = codexTurnStateModelKey(account, model)
	if !codexTurnStateProbeEnabled(account) || account.Status != StatusActive || CodexTurnStateHealthyLength(account) == 0 || !containsString(CodexTurnStateProbeModels(account), model) {
		return nil, errors.New("account_not_eligible")
	}
	state = strings.TrimSpace(state)
	digest := sha256.Sum256([]byte(state))
	stateHash := hex.EncodeToString(digest[:])
	now := time.Now().UTC()
	s.importMu.Lock()
	if err := s.ctx.Err(); err != nil {
		s.importMu.Unlock()
		return nil, err
	}
	if s.importJobs == nil {
		s.importJobs = make(map[string]*CodexStateImportJob)
	}
	if s.importPending == nil {
		s.importPending = make(map[int64]bool)
	}
	for _, job := range s.importJobs {
		if job.AccountID == id && job.Model == model && job.Status == "running" {
			if job.stateHash == stateHash {
				copy := *job
				s.importMu.Unlock()
				return &copy, nil
			}
			// A different value is queued behind the current validation. The
			// account remains single-flight, so the new job will wait for it.
			continue
		}
		if job.AccountID == id && job.Model == model && job.Status == "queued" {
			if job.stateHash == stateHash {
				copy := *job
				s.importMu.Unlock()
				return &copy, nil
			}
			job.Status, job.Reason, job.UpdatedAt = "failed", "superseded", now
		}
	}
	job := &CodexStateImportJob{ID: fmt.Sprintf("import-%d-%d", id, now.UnixNano()), AccountID: id, Model: model, Status: "queued", CreatedAt: now, UpdatedAt: now}
	job.stateHash = stateHash
	s.importJobs[job.ID] = job
	s.importPending[id] = true
	copy := *job
	s.wg.Add(1)
	s.importMu.Unlock()
	s.Wake()
	go s.runImportJob(job.ID, id, model, state)
	return &copy, nil
}

func (s *CodexTurnStateProbeService) runImportJob(jobID string, id int64, model, state string) {
	defer s.wg.Done()
	for {
		s.importMu.Lock()
		job := s.importJobs[jobID]
		if job == nil || job.Status != "queued" {
			s.importMu.Unlock()
			return
		}
		if job != nil {
			job.Status = "running"
			job.UpdatedAt = time.Now().UTC()
		}
		s.importMu.Unlock()
		_, err := s.ImportState(s.ctx, id, model, state)
		if err != nil && err.Error() == "account_busy" {
			s.importMu.Lock()
			job.Status = "queued"
			s.importMu.Unlock()
			select {
			case <-time.After(250 * time.Millisecond):
				continue
			case <-s.ctx.Done():
				err = errors.New("import_cancelled")
			}
		}
		s.importMu.Lock()
		if job := s.importJobs[jobID]; job != nil {
			job.UpdatedAt = time.Now().UTC()
			if err == nil {
				job.Status, job.Reason = "succeeded", "replaced"
			} else {
				job.Status, job.Reason = "failed", err.Error()
			}
		}
		pending := false
		for _, pendingJob := range s.importJobs {
			if pendingJob.AccountID == id && (pendingJob.Status == "queued" || pendingJob.Status == "running") {
				pending = true
				break
			}
		}
		if !pending {
			delete(s.importPending, id)
		}
		s.importMu.Unlock()
		s.Wake()
		return
	}
}

func (s *CodexTurnStateProbeService) ImportJob(id string) (*CodexStateImportJob, bool) {
	s.importMu.Lock()
	defer s.importMu.Unlock()
	job, ok := s.importJobs[id]
	if !ok {
		return nil, false
	}
	copy := *job
	return &copy, true
}

type codexStateSchedule struct {
	Next          time.Time
	Remaining     int
	Attempt       int
	Manual        bool
	ManualPending bool
	NoValid       bool
	Mode          string
}

type codexStateProbeTask struct {
	account Account
	model   string
	due     time.Time
}

func fairCodexStateProbeTasks(tasks []codexStateProbeTask, cursor int) ([]codexStateProbeTask, int) {
	if len(tasks) == 0 {
		return nil, 0
	}
	unique := make([]codexStateProbeTask, 0, len(tasks))
	seen := make(map[int64]struct{}, len(tasks))
	for _, task := range tasks {
		if _, ok := seen[task.account.ID]; ok {
			continue
		}
		seen[task.account.ID] = struct{}{}
		unique = append(unique, task)
	}
	start := cursor % len(unique)
	ordered := append(make([]codexStateProbeTask, 0, len(unique)), unique[start:]...)
	ordered = append(ordered, unique[:start]...)
	return ordered, (start + 1) % len(unique)
}

func codexStateMode(entry codexTurnStateCacheEntry, expected int, token string, now time.Time) string {
	if !validCodexTurnStateCacheEntry(entry, expected, token, now) {
		return "continuous"
	}
	if codexStateEffectiveExpiry(entry, now).Sub(now) <= codexTurnStateRefreshBefore {
		return "continuous"
	}
	return "idle"
}
func (s *CodexTurnStateProbeService) reserveStateAccount(id int64) bool {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	if s.active == nil {
		s.active = map[int64]bool{}
	}
	if s.active[id] || len(s.active) >= 4 {
		return false
	}
	s.active[id] = true
	return true
}
func (s *CodexTurnStateProbeService) releaseStateAccount(id int64) {
	s.scheduleMu.Lock()
	delete(s.active, id)
	s.scheduleMu.Unlock()
	s.Wake()
}

func (s *CodexTurnStateProbeService) dispatchStateProbes(ctx context.Context) {
	if s.accountRepo == nil || s.proxyRepo == nil || ctx.Err() != nil {
		return
	}
	now := time.Now()
	if now.Sub(s.lastCleanup) > time.Hour {
		if repo, ok := s.accountRepo.(CodexStateEventRepository); ok {
			if repo.CleanupCodexStateEvents(ctx, now.Add(-7*24*time.Hour)) == nil {
				s.lastCleanup = now
			}
		}
	}
	global, err := s.GetGlobalConfig(ctx)
	if err != nil || !global.Available {
		return
	}
	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformOpenAI)
	if err != nil {
		return
	}
	tasks := []codexStateProbeTask{}
	for _, account := range accounts {
		if !codexTurnStateProbeEnabled(&account) || CodexTurnStateHealthyLength(&account) == 0 {
			s.pauseStateAccount(account.ID)
			continue
		}
		for _, model := range CodexTurnStateProbeModels(&account) {
			key := codexTurnStatePoolKey{account.ID, model}
			entry := codexTurnStateCacheFromExtra(account.Extra)[model]
			mode := codexStateMode(entry, CodexTurnStateHealthyLength(&account), account.GetOpenAIAccessToken(), now)
			queued := s.takeManualRefresh(account.ID, model)
			s.scheduleMu.Lock()
			if s.schedules == nil {
				s.schedules = map[codexTurnStatePoolKey]*codexStateSchedule{}
			}
			state := s.schedules[key]
			if state == nil {
				state = &codexStateSchedule{Next: now}
				s.schedules[key] = state
			}
			previous := state.Mode
			state.NoValid = !validCodexTurnStateCacheEntry(entry, CodexTurnStateHealthyLength(&account), account.GetOpenAIAccessToken(), now)
			if queued {
				state.Manual = true
				state.Remaining = 25
				state.Attempt = 0
				state.Next = now
			}
			if state.ManualPending {
				state.ManualPending = false
				state.Manual = true
				state.Remaining = 25
				state.Attempt = 0
				state.Next = now
				mode = "manual"
			}
			if mode == "idle" && state.Manual {
				mode = "manual"
			}
			if !global.Available || account.Status != StatusActive {
				mode = "paused"
			}
			if mode == "continuous" && previous != "continuous" {
				state.Next = now
			}
			if previous == "paused" && mode != "paused" {
				state.Next = now
			}
			// Cookie expiry is independent of the ticket. A fresh ticket does
			// not keep a dead or nearly dead routing pair from being refreshed.
			if mode == "idle" && codexRoutingCookieDue(account.Extra, now) {
				mode = "periodic"
				state.Next = now
			}
			state.Mode = mode
			if mode == "idle" {
				state.Next = codexStateEffectiveExpiry(entry, now).Add(-codexTurnStateRefreshBefore)
			}
			s.importMu.Lock()
			importPending := s.importPending[account.ID]
			s.importMu.Unlock()
			if mode != "idle" && mode != "paused" && !importPending && !state.Next.After(now) && !s.active[account.ID] {
				tasks = append(tasks, codexStateProbeTask{account, model, state.Next})
			}
			s.scheduleMu.Unlock()
			if previous != "" && previous != mode && (previous == "paused" || mode == "paused") {
				recordCodexStateEvent(s.accountRepo, CodexStateEvent{AccountID: account.ID, Model: model, Kind: mode, Reason: mode})
			}
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].due.Before(tasks[j].due) })
	tasks, s.dispatchCursor = fairCodexStateProbeTasks(tasks, s.dispatchCursor)
	for _, task := range tasks {
		if !s.reserveStateAccount(task.account.ID) {
			continue
		}
		s.wg.Add(1)
		go func(id int64, model string) {
			defer s.wg.Done()
			defer s.releaseStateAccount(id)
			s.executeStateAttempt(ctx, id, model)
		}(task.account.ID, task.model)
	}
}

func (s *CodexTurnStateProbeService) executeStateAttempt(ctx context.Context, id int64, model string) {
	key := codexTurnStatePoolKey{id, model}
	s.scheduleMu.Lock()
	state := s.schedules[key]
	if state == nil {
		s.scheduleMu.Unlock()
		return
	}
	state.Attempt++
	attempt := state.Attempt
	if state.Remaining <= 0 {
		state.Remaining = 25
	}
	manual := state.Manual
	s.scheduleMu.Unlock()
	source := "automatic"
	if manual {
		source = "manual_probe"
	}
	s.setProbeRuntime(id, model, "running", attempt, "")
	outcome := "probe_unavailable"
	replaced := false
	cookiesSaved := false
	var replacedExpiry time.Time
	defer func() {
		s.scheduleMu.Lock()
		defer s.scheduleMu.Unlock()
		// A saved cookie or a newer ticket ends this attempt. Keep hunting only
		// while the model still has no usable ticket.
		if replaced || (cookiesSaved && !state.NoValid) {
			state.Mode = "idle"
			if replaced && time.Until(replacedExpiry) <= codexTurnStateRefreshBefore {
				state.Mode = "continuous"
			}
			state.Manual = false
			state.Remaining = 0
			state.Attempt = 0
			state.Next = time.Now().Add(time.Second)
			s.setProbeRuntime(id, model, "idle", attempt, outcome)
			return
		}
		advanceCodexStateSchedule(state, false, time.Now())
		s.setProbeRuntime(id, model, "idle", attempt, outcome)
	}()
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return
	}
	if !codexTurnStateProbeEnabled(account) || account.Status != StatusActive || !containsString(CodexTurnStateProbeModels(account), model) {
		return
	}
	global, err := s.GetGlobalConfig(ctx)
	if err != nil || !global.Available {
		return
	}
	proxy, err := s.proxyRepo.GetByID(ctx, global.ProxyID)
	if err != nil || proxy == nil {
		return
	}
	token, err := s.probeToken(ctx, account)
	if err != nil {
		recordCodexStateEvent(s.accountRepo, CodexStateEvent{AccountID: id, Model: model, Kind: "probe_result", Reason: "credentials_unavailable", Source: source})
		return
	}
	event := CodexStateEvent{AccountID: id, Model: model, Source: source, Attempt: attempt, Kind: "probe_started", ProxyID: proxy.ID, ProxyName: proxy.Name, ProxyAddress: fmt.Sprintf("%s://%s:%d", proxy.Protocol, proxy.Host, proxy.Port)}
	recordCodexStateEvent(s.accountRepo, event)
	start := time.Now()
	result := s.acquireState(ctx, account, proxy, token, model, "", codexTurnStateOutboundIdentity{})
	event.Kind = "probe_result"
	event.HTTPStatus = result.status
	event.ReturnedLength = result.returnedLength
	event.DurationMS = time.Since(start).Milliseconds()
	if result.err != nil {
		outcome = result.err.Error()
		if result.routing.complete() {
			cookiesSaved = s.persistRoutingCookies(ctx, account, result.routing)
		}
	} else {
		replaced, outcome, cookiesSaved = s.replaceState(ctx, account, model, token, result.state, result.obtained, proxy.ID, source, result.routing)
		_, expires, _ := codexStateTimes(result.state, result.obtained, time.Now())
		event.ExpiresAt = &expires
		if replaced {
			replacedExpiry = expires
		}
	}
	event.Reason = outcome
	s.referenceIP(ctx, proxy, &event)
	recordCodexStateEvent(s.accountRepo, event)
}

func (s *CodexTurnStateProbeService) replaceState(ctx context.Context, expected *Account, model, token, state string, obtained time.Time, proxyID int64, source string, cookies codexTurnStateRoutingCookies) (bool, string, bool) {
	_, expires, err := codexStateTimes(state, obtained, time.Now())
	if err != nil {
		return false, err.Error(), false
	}
	for range 3 {
		current, err := s.accountRepo.GetByID(ctx, expected.ID)
		if err != nil {
			return false, "account_unavailable", false
		}
		if !codexTurnStateProbeEnabled(current) || current.Status != StatusActive || codexTurnStateOwnerIdentity(current) != codexTurnStateOwnerIdentity(expected) || !containsString(CodexTurnStateProbeModels(current), model) || CodexTurnStateHealthyLength(current) != len(state) {
			return false, "account_changed", false
		}
		cache := codexTurnStateCacheFromExtra(current.Extra)
		old := cache[model]
		now := time.Now()
		if old.State == state || (validCodexTurnStateCacheEntry(old, CodexTurnStateHealthyLength(current), token, now) && !expires.After(codexStateEffectiveExpiry(old, now))) {
			reason := "not_newer"
			if old.State == state {
				reason = "same_state"
			}
			if !cookies.complete() {
				return false, reason, false
			}
			saved, err := s.saveRoutingCookiesOn(ctx, current, cookies)
			if err != nil {
				return false, "save_failed", false
			}
			if saved {
				return false, reason, true
			}
			continue
		}
		cache[model] = codexTurnStateCacheEntry{State: state, ObtainedAt: obtained, ExpiresAt: expires, ProxyID: proxyID, CredentialHash: codexTurnStateCredentialHash(token), Source: source}
		failures := codexTurnStateProbeFailuresFromExtra(current.Extra)
		delete(failures, model)
		updates := codexTurnStateProbeRuntimeUpdates(cache, failures)
		if cookies.complete() {
			updates[CodexTurnStateRoutingCookieExtraKey] = cookies.extraValue()
		}
		saved, err := saveCodexTurnStateProbeRuntime(ctx, s.accountRepo, current, updates)
		if err != nil {
			return false, "save_failed", false
		}
		if saved {
			return true, "replaced", cookies.complete()
		}
	}
	return false, "concurrent_update", false
}

func cloneCodexTurnStateCache(in map[string]codexTurnStateCacheEntry) map[string]codexTurnStateCacheEntry {
	out := make(map[string]codexTurnStateCacheEntry, len(in))
	for model, entry := range in {
		out[model] = entry
	}
	return out
}

// stageCodexTurnStatePending keeps a ticket minted under a session that is
// not published yet. The live cache changes only when every model that still
// has a ticket has a newer one under that same session.
func (s *CodexTurnStateProbeService) stageCodexTurnStatePending(ctx context.Context, expected *Account, model, token, state string, obtained time.Time, proxyID int64, source string, identity codexTurnStateOutboundIdentity) (bool, string) {
	_, expires, err := codexStateTimes(state, obtained, time.Now())
	if err != nil {
		return false, err.Error()
	}
	if expected == nil || !identity.complete() {
		return false, "identity_changed"
	}
	entry := codexTurnStateCacheEntry{State: state, ObtainedAt: obtained, ExpiresAt: expires, ProxyID: proxyID, CredentialHash: codexTurnStateCredentialHash(token), Source: source}
	s.scheduleMu.Lock()
	bundle := s.outboundPending[expected.ID]
	if bundle == nil || bundle.identity.SessionID != identity.SessionID {
		s.scheduleMu.Unlock()
		return false, "identity_changed"
	}
	if bundle.tickets == nil {
		bundle.tickets = map[string]codexTurnStateCacheEntry{}
	}
	if old, ok := bundle.tickets[model]; ok {
		if old.State == state || (validCodexTurnStateCacheEntry(old, len(state), token, time.Now()) && !expires.After(codexStateEffectiveExpiry(old, time.Now()))) {
			s.scheduleMu.Unlock()
			if old.State == state {
				return false, "pending_bundle"
			}
			return false, "not_newer"
		}
	}
	bundle.tickets[model] = entry
	s.scheduleMu.Unlock()
	return s.commitCodexTurnStatePending(ctx, expected, token, identity)
}

func (s *CodexTurnStateProbeService) commitCodexTurnStatePending(ctx context.Context, expected *Account, token string, identity codexTurnStateOutboundIdentity) (bool, string) {
	for range 3 {
		current, err := s.accountRepo.GetByID(ctx, expected.ID)
		if err != nil {
			return false, "account_unavailable"
		}
		if !codexTurnStateProbeEnabled(current) || current.Status != StatusActive || codexTurnStateOwnerIdentity(current) != codexTurnStateOwnerIdentity(expected) || CodexTurnStateHealthyLength(current) == 0 {
			return false, "account_changed"
		}
		s.scheduleMu.Lock()
		bundle := s.outboundPending[current.ID]
		if bundle == nil || bundle.identity.SessionID != identity.SessionID {
			s.scheduleMu.Unlock()
			return false, "identity_changed"
		}
		pending := cloneCodexTurnStateCache(bundle.tickets)
		s.scheduleMu.Unlock()
		models := CodexTurnStateProbeModels(current)
		live := codexTurnStateCacheFromExtra(current.Extra)
		required := make(map[string]codexTurnStateCacheEntry, len(live))
		for model, entry := range live {
			if containsString(models, model) {
				required[model] = entry
			}
		}
		probedPending := make(map[string]codexTurnStateCacheEntry, len(pending))
		for model, entry := range pending {
			if containsString(models, model) {
				probedPending[model] = entry
			}
		}
		now := time.Now()
		if !codexTurnStateBundleReady(required, probedPending, CodexTurnStateHealthyLength(current), token, now) {
			return false, "pending_bundle"
		}
		nextCache := make(map[string]codexTurnStateCacheEntry, len(probedPending))
		var latest time.Time
		for model, entry := range probedPending {
			if !validCodexTurnStateCacheEntry(entry, CodexTurnStateHealthyLength(current), token, now) {
				continue
			}
			nextCache[model] = entry
			if expires := codexStateEffectiveExpiry(entry, now); expires.After(latest) {
				latest = expires
			}
		}
		if len(nextCache) == 0 || latest.IsZero() {
			return false, "pending_bundle"
		}
		published := identity
		published.ExpiresAt = latest
		failures := codexTurnStateProbeFailuresFromExtra(current.Extra)
		for model := range nextCache {
			delete(failures, model)
		}
		saved, err := saveCodexTurnStateProbeRuntime(ctx, s.accountRepo, current, codexTurnStateProbeRuntimeUpdatesWithIdentity(nextCache, failures, published.extraValue()))
		if err != nil {
			return false, "save_failed"
		}
		if !saved {
			continue
		}
		s.scheduleMu.Lock()
		if currentBundle := s.outboundPending[current.ID]; currentBundle != nil && currentBundle.identity.SessionID == identity.SessionID {
			delete(s.outboundPending, current.ID)
		}
		s.scheduleMu.Unlock()
		return true, "replaced"
	}
	return false, "concurrent_update"
}

func (s *CodexTurnStateProbeService) ImportState(ctx context.Context, id int64, model, state string) (string, error) {
	state = strings.TrimSpace(state)
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	if !codexTurnStateProbeEnabled(account) || account.Status != StatusActive || !containsString(CodexTurnStateProbeModels(account), model) || CodexTurnStateHealthyLength(account) == 0 {
		return "", errors.New("account_not_eligible")
	}
	if !s.reserveStateAccount(id) {
		return "", errors.New("account_busy")
	}
	defer s.releaseStateAccount(id)
	event := CodexStateEvent{AccountID: id, Model: model, Kind: "import_result", Source: "manual_import", SentLength: len(state)}
	defer func() { recordCodexStateEvent(s.accountRepo, event) }()
	reject := func(reason string) (string, error) { event.Reason = reason; return "", errors.New(reason) }
	now := time.Now().UTC()
	if len(state) != CodexTurnStateHealthyLength(account) {
		return reject("unexpected_state_length")
	}
	_, _, err = codexStateTimes(state, now, now)
	if err != nil {
		return reject(err.Error())
	}
	global, err := s.GetGlobalConfig(ctx)
	if err != nil || !global.Available {
		return reject("proxy_unavailable")
	}
	proxy, err := s.proxyRepo.GetByID(ctx, global.ProxyID)
	if err != nil || proxy == nil {
		return reject("proxy_unavailable")
	}
	token, err := s.probeToken(ctx, account)
	if err != nil {
		return reject("credentials_unavailable")
	}
	event.Attempt = 1
	_, expires, _ := codexStateTimes(state, now, now)
	event.ExpiresAt = &expires
	event.ProxyID = proxy.ID
	event.ProxyName = proxy.Name
	event.ProxyAddress = fmt.Sprintf("%s://%s:%d", proxy.Protocol, proxy.Host, proxy.Port)
	started := event
	started.Kind = "import_started"
	recordCodexStateEvent(s.accountRepo, started)
	start := time.Now()
	result := s.acquireState(ctx, account, proxy, token, model, state, codexTurnStateOutboundIdentity{})
	event.HTTPStatus = result.status
	event.ReturnedLength = result.returnedLength
	event.DurationMS = time.Since(start).Milliseconds()
	if result.err != nil {
		event.Reason = result.err.Error()
		if result.routing.complete() {
			s.persistRoutingCookies(ctx, account, result.routing)
		}
	} else {
		_, event.Reason, _ = s.replaceState(ctx, account, model, token, state, now, proxy.ID, "manual_import", result.routing)
	}
	s.referenceIP(ctx, proxy, &event)
	if event.Reason != "replaced" {
		return "", errors.New(event.Reason)
	}
	s.scheduleMu.Lock()
	delete(s.schedules, codexTurnStatePoolKey{id, model})
	s.scheduleMu.Unlock()
	s.setProbeRuntime(id, model, "idle", 0, "")
	return event.Reason, nil
}

func advanceCodexStateSchedule(state *codexStateSchedule, replaced bool, now time.Time) {
	state.Next = now.Add(time.Second)
	if replaced {
		state.Manual = false
		state.Remaining = 0
		state.Attempt = 0
		switch state.Mode {
		case "periodic":
			state.Next = now.Add(5 * time.Minute)
		case "idle":
			state.Next = now
		}
		return
	}
	if state.Mode != "continuous" || state.NoValid {
		state.Remaining--
		if state.Remaining <= 0 {
			state.Next = now.Add(5 * time.Minute)
			state.Manual = false
			state.Attempt = 0
		}
	}
}

func (s *CodexTurnStateProbeService) pauseStateAccount(id int64) {
	events := []CodexStateEvent{}
	s.scheduleMu.Lock()
	delete(s.outboundPending, id)
	for key, state := range s.schedules {
		if key.accountID == id && state.Mode != "paused" {
			state.Mode = "paused"
			events = append(events, CodexStateEvent{AccountID: id, Model: key.model, Kind: "paused", Reason: "account_or_plan_unavailable"})
		}
	}
	s.scheduleMu.Unlock()
	for _, event := range events {
		recordCodexStateEvent(s.accountRepo, event)
	}
}
