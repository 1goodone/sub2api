package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newCodexQuotaOverdraftSilentHandlerTestService(t *testing.T) *service.OpenAIGatewayService {
	t.Helper()
	previous := service.CodexQuotaOverdraftEnabled()
	t.Cleanup(func() { service.SetCodexQuotaOverdraftEnabled(previous) })
	cfg := &config.Config{Gateway: config.GatewayConfig{
		CodexQuotaOverdraftEnabled: true,
		StreamKeepaliveInterval:    3600,
	}}
	return service.NewOpenAIGatewayService(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

func newCodexQuotaOverdraftSilentHandlerTestAccount() *service.Account {
	reset := time.Now().UTC().Add(4 * time.Hour).Truncate(time.Second)
	state := &service.CodexQuotaOverdraftProbeState{
		Status:            "passed",
		QuotaWindow:       "5h",
		CycleKey:          fmt.Sprintf("5h:%d", reset.Unix()),
		RecoverAt:         &reset,
		FiveHourRecoverAt: &reset,
	}
	return &service.Account{
		ID:       314,
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Extra: map[string]any{
			service.CodexQuotaOverdraftEnabledExtraKey: true,
			"codex_5h_used_percent":                    100,
			"codex_5h_reset_at":                        reset.Format(time.RFC3339Nano),
			service.CodexQuotaOverdraftProbeExtraKey:   state,
		},
	}
}

func newCodexQuotaOverdraftSilentHandlerTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request = request.WithContext(service.WithCodexQuotaOverdraftScheduling(request.Context()))
	return c, recorder
}

func codexQuotaOverdraftSilentHandlerTestBody() []byte {
	return []byte(`{"model":"gpt-5.4","stream":true,"prompt_cache_key":"stable-cache-key","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
}

func TestCodexQuotaOverdraftSilentRetryStopsAfterThreeAttemptsWithoutSwitching(t *testing.T) {
	gateway := newCodexQuotaOverdraftSilentHandlerTestService(t)
	h := &OpenAIGatewayHandler{gatewayService: gateway}
	account := newCodexQuotaOverdraftSilentHandlerTestAccount()
	c, recorder := newCodexQuotaOverdraftSilentHandlerTestContext(t)
	var bodies [][]byte
	attempts := 0

	_, err := h.forwardResponsesWithCodexQuotaOverdraftSilentRetry(
		c,
		account,
		codexQuotaOverdraftSilentHandlerTestBody(),
		true,
		zap.NewNop(),
		func(body []byte) (*service.OpenAIForwardResult, error) {
			attempts++
			bodies = append(bodies, bytes.Clone(body))
			_, writeErr := fmt.Fprintf(c.Writer, "data: failed-partial-%d\n\n", attempts)
			require.NoError(t, writeErr)
			return nil, &service.UpstreamFailoverError{
				StatusCode:             http.StatusBadGateway,
				ResponseBody:           []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`),
				RequestScopedTransient: true,
			}
		},
	)
	require.Error(t, err)
	require.Equal(t, service.CodexQuotaOverdraftSilentMaxAttempts, attempts)
	require.Len(t, bodies, service.CodexQuotaOverdraftSilentMaxAttempts)
	for _, body := range bodies[1:] {
		require.Equal(t, bodies[0], body, "all retries must reuse the exact injected request body")
	}
	require.Contains(t, string(bodies[0]), "stable-cache-key")
	require.Contains(t, string(bodies[0]), "call_sub2api_overdraft_")
	require.Empty(t, recorder.Body.String(), "failed partial attempts must never reach the client")

	var failoverErr *service.UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, service.NextAccountStop, failoverErr.NextAccountAction)
	h.handleFailoverExhausted(c, failoverErr, false)
	require.JSONEq(t, `{"error":{"type":"upstream_error","message":"Upstream service temporarily unavailable"}}`, recorder.Body.String())
}

func TestCodexQuotaOverdraftSilentRetryCommitsOnlyRecoveredAttempt(t *testing.T) {
	gateway := newCodexQuotaOverdraftSilentHandlerTestService(t)
	h := &OpenAIGatewayHandler{gatewayService: gateway}
	account := newCodexQuotaOverdraftSilentHandlerTestAccount()
	c, recorder := newCodexQuotaOverdraftSilentHandlerTestContext(t)
	attempts := 0
	var firstBody []byte

	result, err := h.forwardResponsesWithCodexQuotaOverdraftSilentRetry(
		c,
		account,
		codexQuotaOverdraftSilentHandlerTestBody(),
		true,
		zap.NewNop(),
		func(body []byte) (*service.OpenAIForwardResult, error) {
			attempts++
			if attempts == 1 {
				firstBody = bytes.Clone(body)
				_, writeErr := c.Writer.WriteString("data: discard-first\n\n")
				require.NoError(t, writeErr)
				return nil, &service.UpstreamFailoverError{StatusCode: http.StatusServiceUnavailable}
			}
			require.Equal(t, firstBody, body)
			_, writeErr := c.Writer.WriteString("data: recovered-final\n\n")
			require.NoError(t, writeErr)
			return &service.OpenAIForwardResult{Stream: true}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 2, attempts)
	require.Equal(t, "data: recovered-final\n\n", recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "discard-first")
}

func TestCodexQuotaOverdraftSilentDoesNotRetryAuthOrQuotaErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			gateway := newCodexQuotaOverdraftSilentHandlerTestService(t)
			h := &OpenAIGatewayHandler{gatewayService: gateway}
			account := newCodexQuotaOverdraftSilentHandlerTestAccount()
			c, recorder := newCodexQuotaOverdraftSilentHandlerTestContext(t)
			attempts := 0

			_, err := h.forwardResponsesWithCodexQuotaOverdraftSilentRetry(
				c,
				account,
				codexQuotaOverdraftSilentHandlerTestBody(),
				true,
				zap.NewNop(),
				func([]byte) (*service.OpenAIForwardResult, error) {
					attempts++
					return nil, &service.UpstreamFailoverError{StatusCode: status}
				},
			)
			require.Error(t, err)
			require.Equal(t, 1, attempts)
			require.Empty(t, recorder.Body.String())
			var failoverErr *service.UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Equal(t, service.NextAccountLegacyRetry, failoverErr.NextAccountAction)
		})
	}
}

func TestCodexQuotaOverdraftSilentLeavesOrdinaryAccountPathUntouched(t *testing.T) {
	gateway := newCodexQuotaOverdraftSilentHandlerTestService(t)
	h := &OpenAIGatewayHandler{gatewayService: gateway}
	account := &service.Account{ID: 7, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}
	c, recorder := newCodexQuotaOverdraftSilentHandlerTestContext(t)
	attempts := 0

	_, err := h.forwardResponsesWithCodexQuotaOverdraftSilentRetry(
		c,
		account,
		codexQuotaOverdraftSilentHandlerTestBody(),
		true,
		zap.NewNop(),
		func([]byte) (*service.OpenAIForwardResult, error) {
			attempts++
			_, writeErr := c.Writer.WriteString("ordinary-path")
			require.NoError(t, writeErr)
			return nil, &service.UpstreamFailoverError{StatusCode: http.StatusBadGateway}
		},
	)
	require.Error(t, err)
	require.Equal(t, 1, attempts)
	require.Equal(t, "ordinary-path", recorder.Body.String())
}
