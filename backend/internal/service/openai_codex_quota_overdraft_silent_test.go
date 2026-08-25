package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexQuotaOverdraftSilentTestService(t *testing.T) *OpenAIGatewayService {
	t.Helper()
	previous := CodexQuotaOverdraftEnabled()
	SetCodexQuotaOverdraftEnabled(true)
	t.Cleanup(func() { SetCodexQuotaOverdraftEnabled(previous) })
	return &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			CodexQuotaOverdraftEnabled: true,
			MaxLineSize:                defaultMaxLineSize,
			StreamKeepaliveInterval:    3600,
		}},
		toolCorrector: NewCodexToolCorrector(),
	}
}

func newCodexQuotaOverdraftSilentTestAccount(status string, used float64, reset time.Time) *Account {
	reset = reset.UTC()
	state := &CodexQuotaOverdraftProbeState{
		Status:            status,
		QuotaWindow:       "5h",
		CycleKey:          fmt.Sprintf("5h:%d", reset.Unix()),
		RecoverAt:         codexQuotaOverdraftTimePtr(reset),
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(reset),
	}
	return &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			CodexQuotaOverdraftEnabledExtraKey: true,
			"codex_5h_used_percent":            used,
			"codex_5h_reset_at":                reset.Format(time.RFC3339Nano),
			CodexQuotaOverdraftProbeExtraKey:   state,
		},
	}
}

func newCodexQuotaOverdraftSilentTestContext(t *testing.T, ctx context.Context) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	return c, recorder
}

func TestCodexQuotaOverdraftSilentEligibility(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(4 * time.Hour)
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())

	tests := []struct {
		name    string
		account *Account
		stream  bool
		ctx     context.Context
		want    bool
	}{
		{name: "passed current cycle at 100 percent", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, reset), stream: true, ctx: ctx, want: true},
		{name: "pending", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePending, 100, reset), stream: true, ctx: ctx},
		{name: "recovered", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbeRecovered, 100, reset), stream: true, ctx: ctx},
		{name: "below 100 percent", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 99.99, reset), stream: true, ctx: ctx},
		{name: "non streaming", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, reset), stream: false, ctx: ctx},
		{name: "ordinary request context", account: newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, reset), stream: true, ctx: context.Background()},
		{name: "ordinary api key account", account: &Account{ID: 7, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, stream: true, ctx: ctx},
	}

	stale := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, reset)
	stale.Extra[CodexQuotaOverdraftProbeExtraKey] = &CodexQuotaOverdraftProbeState{
		Status:            codexQuotaOverdraftProbePassed,
		QuotaWindow:       "5h",
		CycleKey:          fmt.Sprintf("5h:%d", reset.Add(time.Hour).Unix()),
		FiveHourRecoverAt: codexQuotaOverdraftTimePtr(reset.Add(time.Hour)),
	}
	tests = append(tests, struct {
		name    string
		account *Account
		stream  bool
		ctx     context.Context
		want    bool
	}{name: "state from another quota cycle", account: stale, stream: true, ctx: ctx})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, svc.shouldUseCodexQuotaOverdraftSilentRetry(tc.ctx, tc.account, tc.stream, now))
		})
	}
}

func TestCodexQuotaOverdraftSilentConvertedStreamDropsFailedPartialAttempt(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	account := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, time.Now().Add(4*time.Hour))
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	c, recorder := newCodexQuotaOverdraftSilentTestContext(t, ctx)
	silent := svc.StartCodexQuotaOverdraftSilentResponse(c, account, true)
	require.NotNil(t, silent)
	defer func() { require.NoError(t, silent.Close()) }()

	failed := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_failed"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"discard-me"}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"server_is_overloaded","message":"overloaded"}}}`,
		"",
	}, "\n")
	_, err := svc.handleStreamingResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"failed-request"}},
		Body:       io.NopCloser(strings.NewReader(failed)),
	}, c, account, time.Now(), "gpt-test", "gpt-test")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, recorder.Body.String())
	require.NoError(t, silent.ResetAttempt())

	succeeded := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_ok"}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"final-only"}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	_, err = svc.handleStreamingResponse(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"successful-request"}},
		Body:       io.NopCloser(strings.NewReader(succeeded)),
	}, c, account, time.Now(), "gpt-test", "gpt-test")
	require.NoError(t, err)
	require.NoError(t, silent.Commit())
	require.Contains(t, recorder.Body.String(), "final-only")
	require.NotContains(t, recorder.Body.String(), "discard-me")
	require.NotContains(t, recorder.Body.String(), "server_is_overloaded")
}

func TestCodexQuotaOverdraftSilentPassthroughDropsFailedPartialAttempt(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	account := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, time.Now().Add(4*time.Hour))
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	c, recorder := newCodexQuotaOverdraftSilentTestContext(t, ctx)
	silent := svc.StartCodexQuotaOverdraftSilentResponse(c, account, true)
	require.NotNil(t, silent)
	defer func() { require.NoError(t, silent.Close()) }()

	failed := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_failed"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"discard-passthrough"}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_failed","status":"failed","error":{"code":"server_is_overloaded","message":"overloaded"}}}`,
		"",
	}, "\n")
	_, err := svc.handleStreamingResponsePassthrough(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"failed-request"}},
		Body:       io.NopCloser(strings.NewReader(failed)),
	}, c, account, time.Now(), "gpt-test", "gpt-test")
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Empty(t, recorder.Body.String())
	require.NoError(t, silent.ResetAttempt())

	succeeded := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_ok"}}`,
		"",
		`data: {"type":"response.output_text.delta","delta":"final-passthrough"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_ok","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
	}, "\n")
	_, err = svc.handleStreamingResponsePassthrough(ctx, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"successful-request"}},
		Body:       io.NopCloser(strings.NewReader(succeeded)),
	}, c, account, time.Now(), "gpt-test", "gpt-test")
	require.NoError(t, err)
	require.NoError(t, silent.Commit())
	require.Contains(t, recorder.Body.String(), "final-passthrough")
	require.NotContains(t, recorder.Body.String(), "discard-passthrough")
	require.NotContains(t, recorder.Body.String(), "server_is_overloaded")
}

func TestCodexQuotaOverdraftSilentKeepaliveIsNonSemantic(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	account := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, time.Now().Add(4*time.Hour))
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	c, recorder := newCodexQuotaOverdraftSilentTestContext(t, ctx)
	silent := svc.StartCodexQuotaOverdraftSilentResponse(c, account, true)
	require.NotNil(t, silent)

	require.True(t, silent.beat())
	require.Equal(t, ": keepalive\n\n", recorder.Body.String())
	require.True(t, CodexQuotaOverdraftSilentResponseCommitted(c))
	require.False(t, openAIStreamClientOutputStarted(c, true))
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c))

	_, err := c.Writer.WriteString("data: final\n\n")
	require.NoError(t, err)
	require.Equal(t, -1, OpenAICompactKeepaliveAdjustedWrittenSize(c), "staged bytes are not client-visible")
	require.NoError(t, silent.Commit())
	require.Contains(t, recorder.Body.String(), "data: final")
	require.Greater(t, OpenAICompactKeepaliveAdjustedWrittenSize(c), 0)
	require.True(t, openAIStreamClientOutputStarted(c, true))
}

func TestCodexQuotaOverdraftSilentConcurrentBeatWriteAndReset(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	account := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, time.Now().Add(4*time.Hour))
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	c, _ := newCodexQuotaOverdraftSilentTestContext(t, ctx)
	silent := svc.StartCodexQuotaOverdraftSilentResponse(c, account, true)
	require.NotNil(t, silent)

	errCh := make(chan error, 200)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			if !silent.beat() {
				errCh <- errors.New("keepalive stopped before cleanup")
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			if _, err := c.Writer.WriteString("data: staged\n\n"); err != nil {
				errCh <- err
				return
			}
			if err := silent.ResetAttempt(); err != nil {
				errCh <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	require.NoError(t, silent.Discard())
}

func TestCodexQuotaOverdraftSilentStageLimitDiscardsEverything(t *testing.T) {
	svc := newCodexQuotaOverdraftSilentTestService(t)
	account := newCodexQuotaOverdraftSilentTestAccount(codexQuotaOverdraftProbePassed, 100, time.Now().Add(4*time.Hour))
	ctx := WithCodexQuotaOverdraftScheduling(context.Background())
	c, recorder := newCodexQuotaOverdraftSilentTestContext(t, ctx)
	silent := svc.StartCodexQuotaOverdraftSilentResponse(c, account, true)
	require.NotNil(t, silent)
	stage := silent.stage

	_, err := c.Writer.Write(bytes.Repeat([]byte{'x'}, openAIFirstOutputStageMaxBytes+1))
	require.ErrorIs(t, err, errOpenAIFirstOutputStageLimit)
	require.ErrorIs(t, silent.AttemptError(), errOpenAIFirstOutputStageLimit)
	require.NoError(t, silent.Discard())
	require.True(t, stage.closed)
	require.Empty(t, recorder.Body.String())
}

func TestIsCodexQuotaOverdraftSilentRetryable(t *testing.T) {
	overloaded400 := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "request scoped transient", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway, RequestScopedTransient: true}, want: true},
		{name: "http 500", err: &UpstreamFailoverError{StatusCode: http.StatusInternalServerError}, want: true},
		{name: "http 529", err: &UpstreamFailoverError{StatusCode: 529}, want: true},
		{name: "overloaded http 400", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: overloaded400}, want: true},
		{name: "ordinary http 400", err: &UpstreamFailoverError{StatusCode: http.StatusBadRequest, ResponseBody: []byte(`{"error":{"code":"invalid_request_error"}}`)}},
		{name: "401", err: &UpstreamFailoverError{StatusCode: http.StatusUnauthorized}},
		{name: "403", err: &UpstreamFailoverError{StatusCode: http.StatusForbidden}},
		{name: "429", err: &UpstreamFailoverError{StatusCode: http.StatusTooManyRequests, RequestScopedTransient: true}},
		{name: "413", err: &UpstreamFailoverError{StatusCode: http.StatusRequestEntityTooLarge}},
		{name: "credential failure", err: &UpstreamFailoverError{StatusCode: http.StatusBadGateway, Stage: GatewayFailureStageAccountAuth}},
		{name: "explicit stop", err: &UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable, NextAccountAction: NextAccountStop}},
		{name: "plain error", err: errors.New("network")},
		{name: "client canceled", err: context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, IsCodexQuotaOverdraftSilentRetryable(tc.err))
		})
	}
}
