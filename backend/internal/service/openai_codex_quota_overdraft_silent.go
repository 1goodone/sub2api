package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// CodexQuotaOverdraftSilentMaxAttempts includes the first upstream call.
	CodexQuotaOverdraftSilentMaxAttempts = 3

	codexQuotaOverdraftSilentDefaultKeepaliveInterval = 10 * time.Second
	codexQuotaOverdraftSilentResponseKey              = "codex_quota_overdraft_silent_response"
)

// CodexQuotaOverdraftSilentResponse stages one logical Responses stream across
// bounded retries. Only SSE comments reach the client before the final attempt
// is committed, so a failed partial response cannot trigger client reconnect UI.
type CodexQuotaOverdraftSilentResponse struct {
	mu sync.Mutex

	c               *gin.Context
	original        gin.ResponseWriter
	writer          *codexQuotaOverdraftSilentWriter
	initialHeader   http.Header
	header          http.Header
	status          int
	size            int
	stage           *openAIFirstOutputStage
	stageErr        error
	clientErr       error
	cleanupErr      error
	active          bool
	closed          bool
	clientCommitted bool
	heartbeatBytes  int

	originalSizeAtStart int
	adjustedSizeAtStart int
	keepaliveInterval   time.Duration
	stop                chan struct{}
	stopOnce            sync.Once
}

type codexQuotaOverdraftSilentWriter struct {
	session *CodexQuotaOverdraftSilentResponse
}

func (s *OpenAIGatewayService) shouldUseCodexQuotaOverdraftSilentRetry(
	ctx context.Context,
	account *Account,
	stream bool,
	now time.Time,
) bool {
	if !stream || s == nil || s.cfg == nil || !s.cfg.Gateway.CodexQuotaOverdraftEnabled ||
		!codexQuotaOverdraftSchedulingEnabled(ctx) || !CodexQuotaOverdraftEnabledForAccount(account) {
		return false
	}
	state, ok := codexQuotaOverdraftStateFromAccount(account)
	if !ok || state.Status != codexQuotaOverdraftProbePassed {
		return false
	}
	signal, exhausted := codexQuotaOverdraftSignalFromAccount(account, state, now.UTC())
	return exhausted && codexQuotaOverdraftStateCoversSignal(state, signal)
}

// StartCodexQuotaOverdraftSilentResponse enables request-level staging only for
// a currently confirmed 100%-used overdraft cycle. A nil result means the
// caller must use the upstream behavior unchanged.
func (s *OpenAIGatewayService) StartCodexQuotaOverdraftSilentResponse(
	c *gin.Context,
	account *Account,
	stream bool,
) *CodexQuotaOverdraftSilentResponse {
	if c == nil || c.Writer == nil || c.Request == nil ||
		!s.shouldUseCodexQuotaOverdraftSilentRetry(c.Request.Context(), account, stream, time.Now()) {
		return nil
	}

	interval := codexQuotaOverdraftSilentDefaultKeepaliveInterval
	if s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		interval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}
	original := c.Writer
	q := &CodexQuotaOverdraftSilentResponse{
		c:                   c,
		original:            original,
		initialHeader:       cloneCodexQuotaOverdraftSilentHeader(original.Header()),
		active:              true,
		originalSizeAtStart: original.Size(),
		adjustedSizeAtStart: OpenAICompactKeepaliveAdjustedWrittenSize(c),
		keepaliveInterval:   interval,
		stop:                make(chan struct{}),
	}
	q.resetAttemptLocked()
	q.writer = &codexQuotaOverdraftSilentWriter{session: q}
	c.Set(codexQuotaOverdraftSilentResponseKey, q)
	c.Writer = q.writer

	var requestDone <-chan struct{}
	if c.Request != nil {
		requestDone = c.Request.Context().Done()
	}
	go q.runKeepalive(requestDone)
	return q
}

// PrepareCodexQuotaOverdraftSilentRetryBody injects once before the retry loop.
// Reusing the returned bytes keeps call_id, prompt_cache_key and request shape
// stable across every attempt.
func (s *OpenAIGatewayService) PrepareCodexQuotaOverdraftSilentRetryBody(
	ctx context.Context,
	account *Account,
	body []byte,
) []byte {
	return s.prepareCodexQuotaOverdraftBody(ctx, account, false, body)
}

// IsCodexQuotaOverdraftSilentRetryable restricts silent retries to transient
// provider/transport failures. Quota, auth, policy and deterministic request
// failures retain the existing account handling path.
func IsCodexQuotaOverdraftSilentRetryable(err error) bool {
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) || failoverErr == nil ||
		failoverErr.IsCredentialFailure() || failoverErr.NextAccountAction == NextAccountStop {
		return false
	}
	switch failoverErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests, http.StatusRequestEntityTooLarge:
		return false
	case http.StatusBadRequest:
		message := extractUpstreamErrorMessage(failoverErr.ResponseBody)
		return isOpenAITransientProcessingError(failoverErr.StatusCode, message, failoverErr.ResponseBody)
	}
	return failoverErr.RequestScopedTransient || failoverErr.StatusCode == 529 ||
		failoverErr.StatusCode >= http.StatusInternalServerError
}

func (q *CodexQuotaOverdraftSilentResponse) runKeepalive(requestDone <-chan struct{}) {
	timer := time.NewTimer(q.keepaliveInterval)
	defer timer.Stop()
	for {
		select {
		case <-q.stop:
			return
		case <-requestDone:
			return
		case <-timer.C:
		}
		if !q.beat() {
			return
		}
		timer.Reset(q.keepaliveInterval)
	}
}

func (q *CodexQuotaOverdraftSilentResponse) beat() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if !q.clientCommitted {
		header := q.original.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-cache")
		header.Set("Connection", "keep-alive")
		header.Set("X-Accel-Buffering", "no")
		q.original.WriteHeader(http.StatusOK)
		q.clientCommitted = true
	}
	n, err := q.original.Write([]byte(": keepalive\n\n"))
	q.heartbeatBytes += n
	if err != nil {
		q.clientErr = fmt.Errorf("write Codex overdraft silent keepalive: %w", err)
		return false
	}
	q.original.Flush()
	return true
}

// ResetAttempt discards the failed attempt and opens a fresh bounded stage.
func (q *CodexQuotaOverdraftSilentResponse) ResetAttempt() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return io.ErrClosedPipe
	}
	var cleanupErr error
	if q.stage != nil {
		cleanupErr = q.stage.Close()
	}
	q.resetAttemptLocked()
	return cleanupErr
}

func (q *CodexQuotaOverdraftSilentResponse) resetAttemptLocked() {
	q.header = cloneCodexQuotaOverdraftSilentHeader(q.initialHeader)
	q.status = http.StatusOK
	q.size = -1
	q.stage = newDefaultOpenAIFirstOutputStage()
	q.stageErr = nil
}

// AttemptError reports staging or downstream keepalive failures observed while
// Forward was running.
func (q *CodexQuotaOverdraftSilentResponse) AttemptError() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return errors.Join(q.stageErr, q.clientErr)
}

func (q *CodexQuotaOverdraftSilentResponse) HasBufferedResponse() bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size >= 0
}

// Commit publishes only the final successful/protocol-complete attempt.
func (q *CodexQuotaOverdraftSilentResponse) Commit() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return io.ErrClosedPipe
	}
	q.stopLocked()
	q.active = false
	if err := errors.Join(q.stageErr, q.clientErr); err != nil {
		q.cleanupErr = errors.Join(q.cleanupErr, q.closeStageLocked())
		q.restoreLocked()
		q.closed = true
		return err
	}
	if !q.clientCommitted {
		copyCodexQuotaOverdraftSilentHeader(q.original.Header(), q.header)
		q.original.WriteHeader(q.status)
		q.clientCommitted = true
	}
	err := q.stage.CommitTo(q.original)
	q.cleanupErr = errors.Join(q.cleanupErr, q.stage.Close())
	q.stage = nil
	q.original.Flush()
	q.restoreLocked()
	q.closed = true
	return err
}

// Discard removes the current attempt without exposing any staged semantic data.
func (q *CodexQuotaOverdraftSilentResponse) Discard() error {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return q.takeCleanupErrLocked()
	}
	q.stopLocked()
	q.active = false
	err := errors.Join(q.closeStageLocked(), q.takeCleanupErrLocked())
	q.restoreLocked()
	q.closed = true
	return err
}

// Close is an idempotent safety net for handler early returns.
func (q *CodexQuotaOverdraftSilentResponse) Close() error {
	return q.Discard()
}

func (q *CodexQuotaOverdraftSilentResponse) stopLocked() {
	q.stopOnce.Do(func() { close(q.stop) })
}

func (q *CodexQuotaOverdraftSilentResponse) closeStageLocked() error {
	if q.stage == nil {
		return nil
	}
	err := q.stage.Close()
	q.stage = nil
	return err
}

func (q *CodexQuotaOverdraftSilentResponse) takeCleanupErrLocked() error {
	err := q.cleanupErr
	q.cleanupErr = nil
	return err
}

func (q *CodexQuotaOverdraftSilentResponse) restoreLocked() {
	if q.c != nil && q.c.Writer == q.writer {
		q.c.Writer = q.original
	}
}

func codexQuotaOverdraftSilentResponseActive(c *gin.Context) bool {
	q, ok := codexQuotaOverdraftSilentResponseFromContext(c)
	if !ok {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.active && !q.closed
}

// CodexQuotaOverdraftSilentResponseCommitted reports whether this request has
// already sent the SSE response headers/keepalive or a committed final stage.
// The handler uses this after the wrapper is discarded to choose the correct
// terminal response format.
func CodexQuotaOverdraftSilentResponseCommitted(c *gin.Context) bool {
	q, ok := codexQuotaOverdraftSilentResponseFromContext(c)
	if !ok {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.clientCommitted
}

func codexQuotaOverdraftSilentAdjustedWrittenSize(c *gin.Context) (int, bool) {
	q, ok := codexQuotaOverdraftSilentResponseFromContext(c)
	if !ok {
		return 0, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	current := q.original.Size()
	delta := 0
	if current >= 0 {
		start := q.originalSizeAtStart
		if start < 0 {
			start = 0
		}
		delta = current - start - q.heartbeatBytes
		if delta < 0 {
			delta = 0
		}
	}
	if delta == 0 {
		return q.adjustedSizeAtStart, true
	}
	if q.adjustedSizeAtStart < 0 {
		return delta, true
	}
	return q.adjustedSizeAtStart + delta, true
}

func codexQuotaOverdraftSilentResponseFromContext(c *gin.Context) (*CodexQuotaOverdraftSilentResponse, bool) {
	if c == nil {
		return nil, false
	}
	value, ok := c.Get(codexQuotaOverdraftSilentResponseKey)
	if !ok {
		return nil, false
	}
	q, ok := value.(*CodexQuotaOverdraftSilentResponse)
	return q, ok && q != nil
}

func cloneCodexQuotaOverdraftSilentHeader(src http.Header) http.Header {
	if src == nil {
		return make(http.Header)
	}
	return src.Clone()
}

func copyCodexQuotaOverdraftSilentHeader(dst, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func (w *codexQuotaOverdraftSilentWriter) Header() http.Header {
	return w.session.header
}

func (w *codexQuotaOverdraftSilentWriter) WriteHeader(code int) {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.size >= 0 || code <= 0 {
		return
	}
	q.status = code
}

func (w *codexQuotaOverdraftSilentWriter) WriteHeaderNow() {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed && q.size < 0 {
		q.size = 0
	}
}

func (w *codexQuotaOverdraftSilentWriter) Write(p []byte) (int, error) {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.stage == nil {
		return 0, io.ErrClosedPipe
	}
	if q.size < 0 {
		q.size = 0
	}
	n, err := q.stage.Write(p)
	q.size += n
	if err != nil {
		q.stageErr = errors.Join(q.stageErr, err)
	}
	return n, err
}

func (w *codexQuotaOverdraftSilentWriter) WriteString(value string) (int, error) {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed || q.stage == nil {
		return 0, io.ErrClosedPipe
	}
	if q.size < 0 {
		q.size = 0
	}
	n, err := q.stage.WriteString(value)
	q.size += n
	if err != nil {
		q.stageErr = errors.Join(q.stageErr, err)
	}
	return n, err
}

func (w *codexQuotaOverdraftSilentWriter) Status() int {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.status
}

func (w *codexQuotaOverdraftSilentWriter) Size() int {
	q := w.session
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

func (w *codexQuotaOverdraftSilentWriter) Written() bool {
	return w.Size() >= 0
}

func (w *codexQuotaOverdraftSilentWriter) Flush() {
	w.WriteHeaderNow()
}

func (w *codexQuotaOverdraftSilentWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.session.original.Hijack()
}

func (w *codexQuotaOverdraftSilentWriter) CloseNotify() <-chan bool {
	return w.session.original.CloseNotify()
}

func (w *codexQuotaOverdraftSilentWriter) Pusher() http.Pusher {
	return w.session.original.Pusher()
}
