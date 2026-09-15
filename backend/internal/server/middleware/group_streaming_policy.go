package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// GroupStreamingPolicy runs before protocol rewrites and account selection.
func GroupStreamingPolicy() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, ok := GetAPIKeyFromContext(c)
		if !ok || apiKey == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		group := apiKey.Group
		if !apiKey.MixedMode && (group == nil || (!group.DisableNonStreaming && group.FallbackGroupID == nil && group.FallbackGroupIDOnInvalidRequest == nil)) {
			c.Next()
			return
		}

		var nonStreaming bool
		switch c.FullPath() {
		case "/v1/messages", "/messages", "/antigravity/v1/messages",
			"/v1/chat/completions", "/chat/completions",
			"/v1/responses", "/responses", "/backend-api/codex/responses",
			"/v1/responses/*subpath", "/responses/*subpath", "/backend-api/codex/responses/*subpath":
			// Native Codex helpers do not create a text response. Other platforms
			// may bridge these paths to generation, so they still need the check.
			if group != nil && group.Platform == service.PlatformOpenAI &&
				(c.Param("subpath") == "/compact" || c.Param("subpath") == "/input_tokens") {
				c.Next()
				return
			}
			body, err := httputil.ReadRequestBodyWithPrealloc(c.Request)
			if err != nil {
				status := http.StatusBadRequest
				var maxErr *http.MaxBytesError
				if errors.As(err, &maxErr) {
					status = http.StatusRequestEntityTooLarge
				}
				writeGroupStreamingError(c, status, "INVALID_REQUEST", "Failed to read request body")
				return
			}
			requestmodel.ResetRequestBody(c.Request, body)
			if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
				writeGroupStreamingError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body")
				return
			}
			// Every parser must agree: duplicate/case-variant fields cannot hide false.
			stream := gjson.GetBytes(body, "stream").Type == gjson.True
			gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
				if strings.EqualFold(key.String(), "stream") && value.Type != gjson.True {
					stream = false
				}
				return true
			})
			nonStreaming = !stream
		case "/v1beta/models/*modelAction", "/antigravity/v1beta/models/*modelAction":
			modelAction := strings.TrimSpace(strings.TrimPrefix(c.Param("modelAction"), "/"))
			model, action, found := strings.Cut(modelAction, ":")
			if !found {
				model, action, _ = strings.Cut(modelAction, "/")
			}
			nonStreaming = action != "streamGenerateContent" && action != "countTokens" &&
				!service.IsGeminiImageGenerationModel(model)
		default:
			c.Next()
			return
		}
		if !nonStreaming {
			c.Next()
			return
		}

		ctx := service.WithNonStreamingTextRequest(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		allowed := service.RestrictAPIKeyRoutingCandidates(ctx, func(candidate *service.Group) bool {
			return service.CheckGroupStreamingPolicy(ctx, candidate) == nil
		})
		if !allowed || service.CheckGroupStreamingPolicy(ctx, apiKey.Group) != nil {
			RejectNonStreamingRequest(c)
			return
		}
		if state, ok := service.APIKeyRoutingStateFromContext(ctx); ok {
			setGroupContext(c, apiKey.Group)
			c.Set(string(ContextKeySubscription), state.CurrentSubscription())
		}
		c.Next()
	}
}

func RejectNonStreamingRequest(c *gin.Context) {
	service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
	MarkIngressRejected(c, IngressRejectNonStreamingDisabled)
	writeGroupStreamingError(c, http.StatusForbidden, "NON_STREAMING_DISABLED", service.ErrNonStreamingDisabled.Message)
}

func writeGroupStreamingError(c *gin.Context, status int, code, message string) {
	path := c.Request.URL.Path
	switch {
	case strings.HasPrefix(path, "/v1beta/"), strings.HasPrefix(path, "/antigravity/v1beta/"):
		GoogleErrorWriter(c, status, message)
	case strings.HasSuffix(path, "/messages"):
		AnthropicErrorWriter(c, status, message)
	default:
		c.JSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "code": code, "message": message}})
	}
	c.Abort()
}
