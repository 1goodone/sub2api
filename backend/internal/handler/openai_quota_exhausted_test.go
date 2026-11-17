//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestUpstreamQuotaExhaustionReturns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, send := range map[string]func(*gin.Context, *service.UpstreamFailoverError, bool){
		"responses":        (&OpenAIGatewayHandler{}).handleFailoverExhausted,
		"messages":         (&OpenAIGatewayHandler{}).handleAnthropicFailoverExhausted,
		"responses_compat": (&GatewayHandler{}).handleResponsesFailoverExhausted,
		"chat_compat":      (&GatewayHandler{}).handleCCFailoverExhausted,
		"gateway": func(c *gin.Context, failure *service.UpstreamFailoverError, stream bool) {
			(&GatewayHandler{}).handleFailoverExhausted(c, failure, service.PlatformOpenAI, stream)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			send(c, &service.UpstreamFailoverError{
				StatusCode:      http.StatusTooManyRequests,
				ResponseBody:    []byte(`{"error":{"type":"usage_limit_reached","message":"The usage limit has been reached","plan_type":"plus","limit_window_minutes":300,"resets_in_seconds":666}}`),
				ResponseHeaders: http.Header{"Retry-After": {"666"}},
			}, false)
			require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
			require.Equal(t, "666", rec.Header().Get("Retry-After"))
			require.NotContains(t, rec.Body.String(), "rate_limit_error")
			require.NotContains(t, rec.Body.String(), "plus")
		})
	}
}

func TestOpenAIQuotaExhaustionStartedStream(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Header("Content-Type", "text/event-stream")
	_, err := c.Writer.WriteString(": keepalive\n\n")
	require.NoError(t, err)
	c.Writer.Flush()
	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"type":"usage_limit_reached"}}`),
	}, true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, strings.Count(rec.Body.String(), "event: response.failed"))
	require.Contains(t, rec.Body.String(), `"code":"server_error"`)
	require.NotContains(t, rec.Body.String(), "rate_limit_error")
	upstreamStatus, ok := c.Get(service.OpsUpstreamStatusCodeKey)
	require.True(t, ok)
	require.Equal(t, http.StatusTooManyRequests, upstreamStatus)
}

func TestOpenAIQuotaExhaustionStillSwitchesAccounts(t *testing.T) {
	for _, healthy := range []bool{false, true} {
		t.Run(map[bool]string{false: "exhausted", true: "healthy"}[healthy], func(t *testing.T) {
			upstream := &poolCapacityTransport{reply: func(_ *http.Request, id int64) *http.Response {
				if healthy && id == 2 {
					return poolCapacityResponse(http.StatusOK, "application/json", poolCapacityCompleted, "")
				}
				return poolCapacityResponse(http.StatusTooManyRequests, "application/json", `{"error":{"type":"usage_limit_reached","resets_in_seconds":666}}`, "666")
			}}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newOpenAIResponsesFailoverTestContext(t, nil)
			h.Responses(c)
			require.Equal(t, []int64{1, 2}, upstream.hits)
			if healthy {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			} else {
				require.Equal(t, http.StatusServiceUnavailable, rec.Code, rec.Body.String())
				require.Equal(t, "server_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
			}
		})
	}
}

func TestOpenAIQuotaExhaustionPreservesTransient429(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"type":"rate_limit_exceeded","message":"Too many requests"}}`),
	}, false)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "rate_limit_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
}

func TestUpstreamQuotaExhaustionClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure *service.UpstreamFailoverError
		want    bool
	}{
		{name: "nil"},
		{name: "usage_limit", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"error":{"type":"usage_limit_reached"}}`)}, want: true},
		{name: "api_quota", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"error":{"type":"rate_limit_error","code":"insufficient_quota"}}`)}, want: true},
		{name: "stream_quota", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"response":{"error":{"code":"usage_limit_reached"}}}`)}, want: true},
		{name: "classified_quota_headers", failure: &service.UpstreamFailoverError{StatusCode: 429, UpstreamQuotaExhausted: true}, want: true},
		{name: "diagnostic_headers_are_not_a_quota_verdict", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseHeaders: http.Header{"X-Codex-Primary-Used-Percent": {"100"}, "X-Codex-Primary-Window-Minutes": {"300"}}}},
		{name: "window_not_exhausted", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseHeaders: http.Header{"X-Codex-Primary-Used-Percent": {"80"}, "X-Codex-Primary-Reset-After-Seconds": {"666"}}}},
		{name: "transient_retry_after", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseHeaders: http.Header{"Retry-After": {"1"}}, ResponseBody: []byte(`{"error":{"type":"rate_limit_exceeded"}}`)}},
		{name: "echoed_quota_message", failure: &service.UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"error":{"type":"rate_limit_exceeded","message":"usage_limit_reached"},"input":{"error":{"type":"usage_limit_reached"}}}`)}},
		{name: "other_status", failure: &service.UpstreamFailoverError{StatusCode: 400, ResponseBody: []byte(`{"error":{"type":"usage_limit_reached"}}`)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.failure.IsAccountQuotaExhausted())
		})
	}
}

func TestOpenAIQuotaExhaustionPreservesPassthroughRule(t *testing.T) {
	rule := newBodyLimitPassthroughRule(http.StatusTooManyRequests, "Configured quota response")
	rule.ErrorCodes = []int{http.StatusTooManyRequests}
	h := newBodyLimitPassthroughHandler(t, rule)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	h.handleFailoverExhausted(c, &service.UpstreamFailoverError{
		StatusCode:   http.StatusTooManyRequests,
		ResponseBody: []byte(`{"error":{"type":"usage_limit_reached"}}`),
	}, false)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Contains(t, rec.Body.String(), "Configured quota response")
}

func TestUpstreamQuotaExhaustionCompatStreamHeartbeat(t *testing.T) {
	for name, send := range map[string]func(*gin.Context, *service.UpstreamFailoverError, bool){
		"responses": (&GatewayHandler{}).handleResponsesFailoverExhausted,
		"chat":      (&GatewayHandler{}).handleCCFailoverExhausted,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/"+name, nil)
			c.Header("Content-Type", "text/event-stream")
			n, err := c.Writer.WriteString(": keepalive\n\n")
			require.NoError(t, err)
			recordGatewayStreamHeartbeat(c, n)
			c.Writer.Flush()
			failure := &service.UpstreamFailoverError{StatusCode: 429, ResponseBody: []byte(`{"error":{"type":"usage_limit_reached"}}`)}
			send(c, failure, true)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Body.String(), service.OpenAICapacityShedClientMessage)
			require.NotContains(t, rec.Body.String(), "rate_limit_error")
			before := rec.Body.String()
			send(c, failure, true)
			require.Equal(t, before, rec.Body.String(), "only one terminal error is allowed")
		})
	}
}
