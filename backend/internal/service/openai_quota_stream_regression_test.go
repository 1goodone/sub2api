//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIQuotaFirstEventPreservesOutputBoundary(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, prefix := range []string{"", ": ping\n\n", "data: {\"type\":\"response.created\",\"response\":{\"id\":\"r\"}}\n\n", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", "data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{}\"}\n\n"} {
				t.Run(fmt.Sprintf("first=%t/pass=%t/prefix=%d", enabled, passthrough, len(prefix)), func(t *testing.T) {
					setStreamFirstResponseForTest(t, enabled)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(prefix + "data: " + `{"type":"response.failed","response":{"error":{"type":"usage_limit_reached","code":"usage_limit_reached","resets_in_seconds":666}}}` + "\n\n"))}
					svc := &OpenAIGatewayService{cfg: &config.Config{}}
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
					var err error
					if passthrough {
						_, err = svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "test", "test")
					} else {
						_, err = svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "test", "test")
					}
					var failure *UpstreamFailoverError
					want := prefix == "" || prefix == ": ping\n\n" || (!enabled && strings.Contains(prefix, "response.created"))
					require.Equal(t, want, errors.As(err, &failure), "%v", err)
					if want {
						require.True(t, failure.IsAccountQuotaExhausted())
						require.False(t, failure.RetryableOnSameAccount)
						require.False(t, failure.RequestRejected, "no new encrypted-history replay permission")
						require.Empty(t, rec.Body.String())
					} else {
						require.Contains(t, rec.Body.String(), "response.failed")
					}
				})
			}
		}
	}
}

func TestOpenAIQuotaEnvelopeSameRetryPolicy(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, detail := range []string{
			`{"type":"usage_limit_reached","resets_in_seconds":666}`,
			`{"code":"insufficient_quota"}`,
			`{"type":"rate_limit_error","message":"usage_limit_reached"}`,
		} {
			t.Run(fmt.Sprintf("nested=%t/%s", nested, detail), func(t *testing.T) {
				payload := `{"error":` + detail + `}`
				if nested {
					payload = `{"type":"response.failed","response":` + payload + `}`
				}
				svc := &OpenAIGatewayService{cfg: &config.Config{}}
				account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
				wantQuota := !strings.Contains(detail, "rate_limit_error")
				failure := svc.newOpenAIAccountFailoverErrorWithClassificationHeaders(account, 429, nil, nil, []byte(payload), "usage limit", false, true)
				require.Equal(t, wantQuota, failure.IsAccountQuotaExhausted())
				require.Equal(t, !wantQuota, failure.RetryableOnSameAccount)
				if wantQuota {
					require.True(t, failure.SameAccountRetryDeadline.IsZero())
				}
				if strings.Contains(detail, "666") {
					svc.markOpenAIOAuth429RateLimited(context.Background(), account, nil, []byte(payload))
					require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
					_, reset := classifyOpenAIOAuth429(nil, []byte(payload))
					require.NotNil(t, reset)
					require.WithinDuration(t, time.Now().Add(666*time.Second), *reset, 2*time.Second)
				}
			})
		}
	}
}
