package requestcapture

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type contextKey struct{}

func WithSession(ctx context.Context, s *Session) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, s)
}
func FromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(contextKey{}).(*Session)
	return s
}

type event struct {
	s       *Session
	part    Part
	data    []byte
	end     bool
	targets []string
	charge  int64
}
type inputChunk struct {
	part Part
	data []byte
	end  bool
}
type partState struct {
	part   Part
	filter *bodyFilter
	ended  bool
	charge int64
}
type recordState struct {
	Record
	parts   map[string]*partState
	indexed bool
	dirty   bool
	blocked bool
}
type Session struct {
	frameMu         sync.Mutex
	frameStreams    map[string]*Stream
	m               *Manager
	mu              sync.Mutex
	meta            Meta
	candidates      []*runtimeTask
	matched         map[string]bool
	inbound         []inputChunk
	inboundBytes    int64
	hasAccounts     bool
	clientObserved  bool
	reason          string
	closed          bool
	failed          bool
	status          int
	attempts        []Attempt
	nextPart        int
	turn            int
	usage           map[string]int64
	errorCode       string
	resultBuffer    []byte
	resultSkip      bool
	resultError     bool
	done            atomic.Bool
	pending         atomic.Int64
	streams         map[*Stream]struct{}
	finishedTargets map[string]bool
	records         map[string]*recordState // Worker-owned, never accessed by forwarding goroutines.
}

func (m *Manager) Begin(meta Meta) *Session {
	// Never wait for administrative operations on the forwarding path.
	if !m.enabled.Load() {
		return nil
	}
	if !m.mu.TryLock() {
		m.admissionSkipped.Add(1)
		return nil
	}
	defer m.mu.Unlock()
	if !m.config.Enabled || m.unhealthy.Load() || m.stopping.Load() {
		return nil
	}
	candidates := []*runtimeTask{}
	matched := map[string]bool{}
	accounts := false
	for id, t := range m.tasks {
		if t.task.Status != "running" || !time.Now().Before(t.task.ExpiresAt) {
			continue
		}
		hit := t.task.TargetType == "user" && t.task.TargetID == meta.UserID || t.task.TargetType == "group" && t.task.TargetID == meta.GroupID
		if hit || t.task.TargetType == "account" {
			candidates = append(candidates, t)
			matched[id] = hit
			if t.task.TargetType == "account" {
				accounts = true
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if len(m.sessions) >= MaxSessions || !m.reserve(64<<10) {
		m.admissionSkipped.Add(1)
		for _, t := range candidates {
			if matched[t.task.ID] {
				t.task.Skipped++
				t.dirty = true
			}
		}
		return nil
	}
	meta.RequestID = bounded(meta.RequestID, 128)
	meta.ClientRequestID = bounded(meta.ClientRequestID, 128)
	meta.Path = bounded(SafeURL(meta.Path), 2048)
	meta.Method = bounded(meta.Method, 16)
	s := &Session{m: m, meta: meta, candidates: candidates, matched: matched, hasAccounts: accounts, streams: map[*Stream]struct{}{}, records: map[string]*recordState{}, finishedTargets: map[string]bool{}, usage: map[string]int64{}}
	for _, t := range candidates {
		t.refs++
	}
	m.sessions[s] = struct{}{}
	return s
}
func bounded(v string, n int) string {
	if len(v) > n {
		return v[:n]
	}
	return v
}
func (s *Session) SetRoutedGroup(id int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.meta.RoutedGroupID = id
	s.mu.Unlock()
}
func (s *Session) MarkPartial(reason string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.reason == "" {
		s.reason = reason
	}
	s.mu.Unlock()
}
func (s *Session) RequireClientInput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.clientObserved && s.reason == "" {
		s.reason = "client_input_unavailable"
	}
}
func (s *Session) SetProtocol(protocol string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.meta.Protocol = protocol
	s.mu.Unlock()
}

func (s *Session) ClientRequest(body []byte, ct string, h http.Header) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.clientObserved || s.closed {
		s.mu.Unlock()
		return
	}
	s.clientObserved = true
	s.meta.Model = bounded(gjson.GetBytes(body, "model").String(), 256)
	s.mu.Unlock()
	stream := s.NewStream("client_request", 0, 0, ct, h)
	_, _ = stream.Write(body)
	_ = stream.Close()
}
func (s *Session) ClientFrame(body []byte) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if gjson.GetBytes(body, "type").String() == "response.create" {
		s.clientObserved = true
		s.turn++
		s.releaseInputLocked()
		s.meta.Model = bounded(gjson.GetBytes(body, "model").String(), 256)
	}
	turn := s.turn
	s.mu.Unlock()
	stream := s.NewStream("client_request", 0, turn, "application/json", nil)
	_, _ = stream.Write(body)
	_ = stream.Close()
}
func (s *Session) BeginAttempt(account int64) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0
	}
	if len(s.attempts) >= 256 {
		if s.reason == "" {
			s.reason = "attempt_limit"
		}
		return 0
	}
	n := len(s.attempts) + 1
	s.attempts = append(s.attempts, Attempt{Number: n, AccountID: account, StartedAt: time.Now().UTC()})
	s.bindAccountLocked(account)
	return n
}
func (s *Session) BindAccount(account int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.bindAccountLocked(account)
	}
}
func (s *Session) bindAccountLocked(account int64) {
	for _, t := range s.candidates {
		if !s.finishedTargets[t.task.ID] && t.active.Load() && t.task.TargetType == "account" && t.task.TargetID == account && !s.matched[t.task.ID] {
			s.matched[t.task.ID] = true
			for _, chunk := range s.inbound {
				s.enqueueLocked(chunk.part, chunk.data, chunk.end, []string{t.task.ID})
			}
		}
	}
}

// Selection failures have no request body sent upstream, but still identify the attempted account.
func (s *Session) SelectionFailed(account int64, status int, headers http.Header, body []byte, err error) {
	if s == nil {
		return
	}
	n := s.BeginAttempt(account)
	s.AttemptResponse(n, status, headers, err)
	if len(body) > 0 {
		st := s.NewStream("upstream_response", n, 0, headers.Get("Content-Type"), headers)
		_, _ = st.Write(body)
		_ = st.Close()
	}
}
func (s *Session) AttemptResponse(n, status int, h http.Header, err error) {
	if s == nil || n <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.attempts) {
		return
	}
	a := &s.attempts[n-1]
	a.Status = status
	a.UpstreamRequestID = bounded(h.Get("x-request-id"), 128)
	if a.UpstreamRequestID == "" {
		a.UpstreamRequestID = bounded(h.Get("request-id"), 128)
	}
	// Transport errors may contain credentials or a signed URL. Preserve class only.
	if err != nil {
		a.Error = "transport_error"
	}
}
func (s *Session) Finish(status int) {
	if s == nil {
		return
	}
	s.closeFrameStreams()
	s.mu.Lock()
	s.status = status
	s.done.Store(true)
	s.mu.Unlock()
	s.m.signal()
}
func (s *Session) releaseInputLocked() {
	s.m.buffer.Add(-s.inboundBytes)
	s.inboundBytes = 0
	s.inbound = nil
}

type Stream struct {
	s         *Session
	part      Part
	mu        sync.Mutex
	buffer    []byte
	lastFlush time.Time
	closed    bool
}

func (s *Session) NewStream(stage string, attempt, turn int, ct string, h http.Header) *Stream {
	if s == nil {
		return &Stream{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed {
		return &Stream{}
	}
	s.nextPart++
	n := s.nextPart
	if turn == 0 && s.meta.Protocol == "websocket" {
		turn = s.turn
	}
	st := &Stream{s: s, part: Part{Name: fmt.Sprintf("%06d-%s.txt", n, stage), Stage: stage, Attempt: attempt, Turn: turn, ContentType: bounded(ct, 256), Headers: SafeHeaders(h)}, lastFlush: time.Now()}
	s.streams[st] = struct{}{}
	return st
}
func (st *Stream) Write(p []byte) (int, error) {
	n := len(p)
	if st == nil || st.s == nil {
		return n, nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return n, nil
	}
	st.s.mu.Lock()
	inactive := st.s.closed || st.s.failed
	st.s.mu.Unlock()
	if inactive {
		return n, nil
	}
	if st.buffer == nil {
		if !st.s.m.reserve(ChunkSize) {
			st.s.failCapture("buffer_limit")
			return n, nil
		}
		st.buffer = make([]byte, 0, ChunkSize)
	}
	for len(p) > 0 {
		count := ChunkSize - len(st.buffer)
		if count > len(p) {
			count = len(p)
		}
		st.buffer = append(st.buffer, p[:count]...)
		p = p[count:]
		if len(st.buffer) == ChunkSize {
			st.s.send(st.part, st.buffer, false)
			st.buffer = st.buffer[:0]
			st.lastFlush = time.Now()
		}
	}
	if time.Since(st.lastFlush) >= 250*time.Millisecond && len(st.buffer) > 0 {
		st.s.send(st.part, st.buffer, false)
		st.buffer = st.buffer[:0]
		st.lastFlush = time.Now()
	}
	return n, nil
}
func (st *Stream) Close() error {
	if st == nil || st.s == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.closed {
		return nil
	}
	st.closed = true
	st.s.send(st.part, st.buffer, true)
	if st.buffer != nil {
		st.s.m.buffer.Add(-ChunkSize)
		st.buffer = nil
	}
	st.s.mu.Lock()
	delete(st.s.streams, st)
	st.s.mu.Unlock()
	return nil
}
func (st *Stream) discard() {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.closed = true
	if st.buffer != nil {
		st.s.m.buffer.Add(-ChunkSize)
		st.buffer = nil
	}
}
func (s *Session) failCapture(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = true
	if s.reason == "" {
		s.reason = reason
	}
}
func (s *Session) send(part Part, p []byte, end bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.failed || s.m.stopping.Load() {
		return
	}
	targets := []string{}
	for _, t := range s.candidates {
		if s.finishedTargets[t.task.ID] || !t.active.Load() || !s.matched[t.task.ID] {
			continue
		}
		if strings.HasPrefix(part.Stage, "upstream_") && t.task.TargetType == "account" {
			if part.Attempt <= 0 || part.Attempt > len(s.attempts) || s.attempts[part.Attempt-1].AccountID != t.task.TargetID {
				continue
			}
		}
		targets = append(targets, t.task.ID)
	}
	if part.Stage == "client_request" && s.hasAccounts {
		charge := int64(len(p) + 512)
		if s.m.reserve(charge) {
			s.inbound = append(s.inbound, inputChunk{part, append([]byte(nil), p...), end})
			s.inboundBytes += charge
		} else {
			s.failed = true
			if s.reason == "" {
				s.reason = "buffer_limit"
			}
		}
	}
	if len(targets) > 0 {
		s.enqueueLocked(part, p, end, targets)
	}
}
func (s *Session) enqueueLocked(part Part, p []byte, end bool, targets []string) {
	if s.failed {
		return
	}
	charge := int64(len(p) + 512)
	if !s.m.reserve(charge) {
		s.failed = true
		if s.reason == "" {
			s.reason = "buffer_limit"
		}
		return
	}
	e := event{s: s, part: part, data: append([]byte(nil), p...), end: end, targets: targets, charge: charge}
	s.pending.Add(1)
	select {
	case s.m.queue <- e:
	default:
		s.pending.Add(-1)
		s.m.buffer.Add(-charge)
		s.failed = true
		if s.reason == "" {
			s.reason = "queue_full"
		}
	}
}
func (s *Session) snapshot(r *Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.Meta = s.meta
	r.Status = s.status
	r.IsError = s.status >= 400 || s.resultError
	r.Attempts = append([]Attempt(nil), s.attempts...)
	for _, a := range s.attempts {
		if a.Status >= 400 || a.Error != "" {
			r.IsError = true
		}
	}
	r.ErrorCode = s.errorCode
	r.Usage = map[string]int64{}
	for k, v := range s.usage {
		r.Usage[k] = v
	}
	if s.reason != "" {
		r.Partial = true
		r.Reason = s.reason
	}
}

// Keep at most 16 KiB of a partial diagnostic event, charged to the session budget.
func (s *Session) ObserveResult(body []byte) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	for _, b := range body {
		if b == '\n' {
			if !s.resultSkip {
				s.parseResultLocked(s.resultBuffer)
			}
			s.resultBuffer = s.resultBuffer[:0]
			s.resultSkip = false
			continue
		}
		if s.resultSkip {
			continue
		}
		if len(s.resultBuffer) == 16<<10 {
			s.resultBuffer = s.resultBuffer[:0]
			s.resultSkip = true
			continue
		}
		s.resultBuffer = append(s.resultBuffer, b)
	}
	if len(s.resultBuffer) > 0 && gjson.ValidBytes(s.resultBuffer) {
		s.parseResultLocked(s.resultBuffer)
		s.resultBuffer = s.resultBuffer[:0]
	}
}
func (s *Session) parseResultLocked(body []byte) {
	line := strings.TrimSpace(strings.TrimPrefix(string(body), "data:"))
	if !gjson.Valid(line) {
		return
	}
	event := gjson.Get(line, "type").String()
	if gjson.Get(line, "error").Exists() || gjson.Get(line, "response.error").Exists() || event == "response.failed" || event == "error" {
		s.resultError = true
	}
	for _, prefix := range []string{"error.", "response.error."} {
		if code := gjson.Get(line, prefix+"code"); code.Exists() {
			s.errorCode = bounded(code.String(), 128)
		}
	}
	for _, prefix := range []string{"usage.", "response.usage.", "message.usage."} {
		for _, key := range []string{"input_tokens", "output_tokens", "total_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "prompt_tokens", "completion_tokens"} {
			if v := gjson.Get(line, prefix+key); v.Exists() {
				s.usage[key] = v.Int()
			}
		}
	}
}

func (m *Manager) process(e event) {
	defer func() { m.buffer.Add(-e.charge); e.s.pending.Add(-1) }()
	for _, target := range e.targets {
		if e.s.finishedTargets[target] {
			continue
		}
		m.mu.Lock()
		t := m.tasks[target]
		running := t != nil && t.task.Status == "running" && time.Now().Before(t.task.ExpiresAt)
		m.mu.Unlock()
		r := e.s.records[target]
		if !running {
			if r != nil {
				r.Partial = true
				if r.Reason == "" {
					r.Reason = "task_stopped"
				}
				r.dirty = true
			}
			continue
		}
		if r == nil {
			r = &recordState{Record: Record{ID: uuid.NewString(), TaskID: target, InstanceID: m.instance, CreatedAt: time.Now().UTC()}, parts: map[string]*partState{}, dirty: true}
			e.s.snapshot(&r.Record)
			e.s.records[target] = r
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			m.storageMu.Lock()
			m.mu.Lock()
			exists := m.tasks[target] != nil
			m.mu.Unlock()
			var err error
			if exists {
				err = m.store.SaveRecord(ctx, &r.Record)
			}
			m.storageMu.Unlock()
			cancel()
			if !exists {
				continue
			}
			if err != nil {
				r.Partial = true
				r.Reason = "index_write_failed"
				r.blocked = true
				m.unhealthy.Store(true)
				m.stopAll("index_write_failed")
				continue
			}
			r.indexed = true
			m.mu.Lock()
			t.task.Requests++
			t.dirty = true
			m.mu.Unlock()
		}
		if r.blocked {
			continue
		}
		part := r.parts[e.part.Name]
		if part == nil {
			if len(r.parts) >= 512 || !m.reserve(24<<10) {
				r.Partial = true
				r.Reason = "part_or_buffer_limit"
				r.blocked = true
				m.finishTarget(e.s, t)
				continue
			}
			part = &partState{part: e.part, filter: newBodyFilter(e.part.ContentType, t.task.SaveMedia), charge: 24 << 10}
			r.parts[e.part.Name] = part
		}
		if part.ended {
			continue
		}
		data := part.filter.Write(e.data)
		if e.end {
			tail, reason := part.filter.End()
			data = append(data, tail...)
			part.ended = true
			part.part.Omitted = reason
			if reason != "" && reason != "media_metadata_only" {
				r.Partial = true
				if r.Reason == "" {
					r.Reason = reason
				}
			}
		}
		if err := m.writePart(r, part, data); err != nil {
			r.Partial = true
			r.Reason = err.Error()
			r.blocked = true
		}
		if e.end {
			part.filter = nil
			m.buffer.Add(-(part.charge - 1024))
			part.charge = 1024
		}
		r.dirty = true
		if r.blocked {
			m.finishTarget(e.s, t)
		}
	}
}
func (m *Manager) writePart(r *recordState, p *partState, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	m.storageMu.Lock()
	defer m.storageMu.Unlock()
	m.mu.Lock()
	t := m.tasks[r.TaskID]
	running := t != nil && t.task.Status == "running"
	quota := m.config.QuotaMiB << 20
	m.mu.Unlock()
	if !running {
		return fmt.Errorf("task_stopped")
	}
	if r.Bytes+int64(len(data)) > RecordLimit {
		return fmt.Errorf("record_limit")
	}
	// The reservation remains charged during append; readers see only persisted bytes.
	if m.used.Load()+int64(len(data)) > quota {
		m.stopAll("quota_exceeded")
		return fmt.Errorf("quota_exceeded")
	}
	dir := filepath.Join(m.dir, r.TaskID, r.ID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		m.unhealthy.Store(true)
		m.stopAll("disk_write_failed")
		return fmt.Errorf("disk_write_failed")
	}
	file, err := os.OpenFile(filepath.Join(dir, p.part.Name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		m.unhealthy.Store(true)
		m.stopAll("disk_write_failed")
		return fmt.Errorf("disk_write_failed")
	}
	m.used.Add(int64(len(data)))
	n, err := file.Write(data)
	closeErr := file.Close()
	m.used.Add(int64(n - len(data)))
	r.Bytes += int64(n)
	p.part.Bytes += int64(n)
	m.mu.Lock()
	if t = m.tasks[r.TaskID]; t != nil {
		t.task.Bytes += int64(n)
		t.dirty = true
	}
	m.mu.Unlock()
	if err != nil || closeErr != nil {
		m.unhealthy.Store(true)
		m.stopAll("disk_write_failed")
		return fmt.Errorf("disk_write_failed")
	}
	return nil
}
func (m *Manager) flushRecord(s *Session, r *recordState, finished bool) {
	s.snapshot(&r.Record)
	r.Parts = nil
	for _, p := range r.parts {
		r.Parts = append(r.Parts, p.part)
	}
	sort.Slice(r.Parts, func(i, j int) bool { return r.Parts[i].Name < r.Parts[j].Name })
	if finished {
		now := time.Now().UTC()
		r.FinishedAt = &now
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if !r.indexed {
		return
	}
	m.storageMu.Lock()
	defer m.storageMu.Unlock()
	m.mu.Lock()
	exists := m.tasks[r.TaskID] != nil
	m.mu.Unlock()
	if !exists {
		return
	}
	if err := m.store.SaveRecord(ctx, &r.Record); err != nil {
		m.unhealthy.Store(true)
		m.stopAll("index_write_failed")
	} else {
		r.dirty = false
	}
}
func (m *Manager) finish(s *Session) {
	s.mu.Lock()
	if s.closed || s.pending.Load() != 0 {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if !s.done.Load() && s.reason == "" {
		s.reason = "task_stopped"
	}
	s.releaseInputLocked()
	streams := make([]*Stream, 0, len(s.streams))
	for st := range s.streams {
		streams = append(streams, st)
	}
	s.streams = nil
	s.mu.Unlock()
	for _, st := range streams {
		st.discard()
	}
	for _, target := range s.candidates {
		m.finishTarget(s, target)
	}
	m.mu.Lock()
	delete(m.sessions, s)
	m.mu.Unlock()
	m.buffer.Add(-(64 << 10))
}

// The worker finalizes each target independently, including overlapping tasks.
func (m *Manager) finishTarget(s *Session, t *runtimeTask) {
	id := t.task.ID
	s.mu.Lock()
	if s.finishedTargets[id] {
		s.mu.Unlock()
		return
	}
	s.finishedTargets[id] = true
	hit := s.matched[id]
	s.mu.Unlock()
	r := s.records[id]
	if hit && r == nil {
		m.mu.Lock()
		exists := m.tasks[id] != nil
		m.mu.Unlock()
		if exists {
			r = &recordState{Record: Record{ID: uuid.NewString(), TaskID: id, InstanceID: m.instance, CreatedAt: time.Now().UTC(), Partial: true, Reason: "no_payload_recorded"}, parts: map[string]*partState{}, indexed: true, dirty: true}
			s.snapshot(&r.Record)
			m.mu.Lock()
			t.task.Requests++
			m.mu.Unlock()
		}
	}
	if r != nil {
		if !s.done.Load() || !t.active.Load() {
			r.Partial = true
			if r.Reason == "" {
				r.Reason = "task_stopped"
			}
		}
		for _, p := range r.parts {
			if !p.ended {
				tail, reason := p.filter.End()
				p.part.Omitted = reason
				r.Partial = true
				if r.Reason == "" {
					r.Reason = "stream_not_completed"
				}
				if t.active.Load() {
					if err := m.writePart(r, p, tail); err != nil && r.Reason == "" {
						r.Reason = err.Error()
					}
				}
			}
			m.buffer.Add(-p.charge)
		}
		m.flushRecord(s, r, true)
		m.mu.Lock()
		if r.Partial {
			t.task.Partial++
		}
		m.mu.Unlock()
		delete(s.records, id)
	}
	m.mu.Lock()
	t.refs--
	t.dirty = true
	m.mu.Unlock()
}

// WrapBody observes only bytes actually read and preserves the original Close.
type observedBody struct {
	io.ReadCloser
	stream *Stream
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.stream.Write(p[:n])
	}
	if err != nil && err != io.EOF {
		b.stream.s.MarkPartial("upstream_read_failed")
	}
	if err == io.EOF {
		_ = b.stream.Close()
	}
	return n, err
}
func (b *observedBody) Close() error { _ = b.stream.Close(); return b.ReadCloser.Close() }
func ObserveBody(body io.ReadCloser, stream *Stream) io.ReadCloser {
	if body == nil || stream == nil || stream.s == nil {
		return body
	}
	return &observedBody{ReadCloser: body, stream: stream}
}

func (s *Session) ObserveHTTPRequest(req *http.Request, account int64) (int, func(*http.Response, error)) {
	if s == nil {
		return 0, func(*http.Response, error) {}
	}
	n := s.BeginAttempt(account)
	st := s.NewStream("upstream_request", n, 0, req.Header.Get("Content-Type"), req.Header)
	st.part.URL = SafeURL(req.URL.String())
	req.Body = ObserveBody(req.Body, st)
	return n, func(resp *http.Response, err error) {
		_ = st.Close()
		status := 0
		var h http.Header
		if resp != nil {
			status = resp.StatusCode
			h = resp.Header
			resp.Body = ObserveBody(resp.Body, s.NewStream("upstream_response", n, 0, h.Get("Content-Type"), h))
		}
		s.AttemptResponse(n, status, h, err)
	}
}

type accountContextKey struct{}

func WithAccount(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, accountContextKey{}, id)
}
func AccountFromContext(ctx context.Context) int64 {
	v, _ := ctx.Value(accountContextKey{}).(int64)
	return v
}
