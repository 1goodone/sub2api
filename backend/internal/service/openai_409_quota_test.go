package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAI409QuotaReplayPermission(t *testing.T) {
	for _, mode := range []string{"safe", "heartbeat", "written", "canceled", "second_http", "ws", "ordinary", "400_recovery", "no_learning", "no_delivery", "output", "pending", "epoch", "tool", "tool_terminal", "invalid_tool", "shadow", "completed_shadow", "unsafe_shadow", "transient", "generic_rejection", "not_rejected"} {
		t.Run(mode, func(t *testing.T) {
			r := &openAIAgentRecoveryRequest{omittedHistory: true, delivery: &openAIAgentRecoveryDelivery{}}
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), openAIAgentRecoveryContextKey{}, r))
			defer cancel()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil).WithContext(ctx)
			failure := &UpstreamFailoverError{StatusCode: 429, RequestRejected: true, UpstreamQuotaExhausted: true}
			attempts, transport := 1, OpenAIUpstreamTransportHTTPSSE
			switch mode {
			case "heartbeat":
				n, err := c.Writer.WriteString(": keepalive\n\n")
				require.NoError(t, err)
				recordOpenAIStreamKeepaliveBytes(c, n)
			case "written":
				_, err := c.Writer.WriteString("data: semantic output\n\n")
				require.NoError(t, err)
			case "canceled":
				cancel()
			case "second_http":
				attempts = 2
			case "ws":
				transport = OpenAIUpstreamTransportResponsesWebsocketV2
			case "ordinary":
				r.omittedHistory = false
			case "400_recovery":
				r.omitted400History = true
			case "no_learning":
				r.disallowLearning = true
			case "no_delivery":
				r.delivery = nil
			case "output":
				r.outputs = [][32]byte{{1}}
			case "pending":
				r.delivery.keys = map[[32]byte]bool{{1}: true}
			case "epoch":
				r.delivery.epochs = map[string]bool{"epoch": true}
			case "tool":
				r.delivery.tools = map[string]openAIToolObservation{"call": {}}
			case "tool_terminal":
				r.delivery.toolTerminal = true
			case "invalid_tool":
				r.delivery.toolInvalid = true
			case "shadow":
				r.shadowCandidate = &openAIReadableShadow{}
			case "completed_shadow":
				r.shadowOutputSafe = true
			case "unsafe_shadow":
				r.shadowUnsafeOutput = true
			case "transient":
				failure.UpstreamQuotaExhausted = false
			case "generic_rejection":
				failure.StatusCode = 403
			case "not_rejected":
				failure.RequestRejected = false
			}
			if r.omittedHistory {
				require.False(t, OpenAIAgentToolResultCanFailover(ctx, failure), "generic RequestRejected must not grant recovery replay")
			}
			failure.recoveryQuotaRejected = openAI409QuotaFailoverSafe(ctx, c, failure, attempts, transport)
			want := mode == "safe" || mode == "heartbeat" || mode == "shadow"
			require.Equal(t, want, failure.recoveryQuotaRejected)
			if r.omittedHistory {
				require.Equal(t, want, OpenAIAgentToolResultCanFailover(ctx, failure))
			}
		})
	}
}
