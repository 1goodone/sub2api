package middleware

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGroupStreamingPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, method, route, path, body string
		status                          int
	}{
		{"streaming", "POST", "/v1/responses", "", `{"stream":true}`, 200},
		{"false", "POST", "/v1/responses", "", `{"stream":false}`, 403},
		{"missing", "POST", "/v1/responses", "", `{}`, 403},
		{"null", "POST", "/v1/responses", "", `{"stream":null}`, 403},
		{"string", "POST", "/v1/responses", "", `{"stream":"true"}`, 403},
		{"duplicate false first", "POST", "/v1/responses", "", `{"stream":false,"stream":true}`, 403},
		{"duplicate false last", "POST", "/v1/responses", "", `{"stream":true,"stream":false}`, 403},
		{"case variant", "POST", "/v1/responses", "", `{"stream":true,"Stream":false}`, 403},
		{"uppercase only", "POST", "/v1/responses", "", `{"Stream":true}`, 403},
		{"consistent duplicate", "POST", "/v1/responses", "", `{"stream":true,"stream":true}`, 200},
		{"nested false", "POST", "/v1/responses", "", `{"stream":true,"metadata":{"stream":false}}`, 200},
		{"invalid json", "POST", "/v1/responses", "", `{"stream":true`, 400},
		{"array", "POST", "/v1/responses", "", `[]`, 400},
		{"messages", "POST", "/v1/messages", "", `{}`, 403},
		{"chat", "POST", "/v1/chat/completions", "", `{}`, 403},
		{"alias", "POST", "/responses", "", `{}`, 403},
		{"codex", "POST", "/backend-api/codex/responses", "", `{}`, 403},
		{"antigravity", "POST", "/antigravity/v1/messages", "", `{}`, 403},
		{"trailing slash", "POST", "/responses/*subpath", "/responses/", `{}`, 403},
		{"unknown subpath", "POST", "/responses/*subpath", "/responses/create", `{}`, 403},
		{"codex compact", "POST", "/responses/*subpath", "/responses/compact", `{}`, 200},
		{"input tokens", "POST", "/responses/*subpath", "/responses/input_tokens", `{}`, 200},
		{"websocket", "GET", "/v1/responses", "", ``, 200},
		{"images", "POST", "/v1/images/generations", "", `{}`, 200},
		{"image edits", "POST", "/v1/images/edits", "", `multipart`, 200},
		{"videos", "POST", "/v1/videos", "", `{}`, 200},
		{"embeddings", "POST", "/v1/embeddings", "", `{}`, 200},
		{"count tokens", "POST", "/v1/messages/count_tokens", "", `{}`, 200},
		{"gemini sync", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro:generateContent", `{}`, 403},
		{"gemini slash sync", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro/generateContent", `{}`, 403},
		{"gemini whitespace sync", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro:generateContent%20", `{}`, 403},
		{"gemini whitespace stream", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro:%20streamGenerateContent", `{}`, 403},
		{"gemini slash stream", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro/streamGenerateContent", `{}`, 200},
		{"gemini stream", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", `{}`, 200},
		{"gemini image", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-3.1-flash-image:generateContent", `{}`, 200},
		{"gemini tokens", "POST", "/v1beta/models/*modelAction", "/v1beta/models/gemini-2.5-pro:countTokens", `{}`, 200},
		{"antigravity gemini", "POST", "/antigravity/v1beta/models/*modelAction", "/antigravity/v1beta/models/gemini-2.5-pro:generateContent", `{}`, 403},
		{"passive image tool", "POST", "/v1/responses", "", `{"tools":[{"type":"image_generation"}],"tool_choice":"none"}`, 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
					Platform: service.PlatformOpenAI, DisableNonStreaming: true,
				}})
			})
			router.Use(GroupStreamingPolicy())
			called := false
			router.Handle(tt.method, tt.route, func(c *gin.Context) {
				called = true
				body, err := io.ReadAll(c.Request.Body)
				require.NoError(t, err)
				require.Equal(t, tt.body, string(body), "body must survive middleware")
				c.Status(200)
			})
			path := tt.path
			if path == "" {
				path = tt.route
			}
			w := doJSON(t, router, tt.method, path, tt.body)
			require.Equal(t, tt.status, w.Code, w.Body.String())
			require.Equal(t, tt.status == 200, called, "denied requests must not reach upstream or billing")
			if tt.status == 403 {
				require.Contains(t, w.Body.String(), "only allows streaming")
			}
		})
	}
}

func TestGroupStreamingPolicyDefaultOffDoesNotReadBody(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{}})
	}, GroupStreamingPolicy())
	router.POST("/v1/responses", func(c *gin.Context) { c.Status(200) })
	body := &readTrackingBody{Reader: strings.NewReader(`{}`)}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/v1/responses", body))
	require.Equal(t, 200, w.Code)
	require.False(t, body.read)
}

func TestGroupStreamingPolicyBodyLimitAndBridgeSubpaths(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyAPIKey), &service.APIKey{Group: &service.Group{
			Platform: service.PlatformAnthropic, DisableNonStreaming: true,
		}})
	}, GroupStreamingPolicy())
	router.POST("/responses/*subpath", func(c *gin.Context) { t.Fatal("must reject before forwarding") })
	for _, suffix := range []string{"/compact", "/input_tokens"} {
		require.Equal(t, 403, doJSON(t, router, "POST", "/responses"+suffix, `{}`).Code)
	}
	req := httptest.NewRequest("POST", "/responses/create", strings.NewReader(`{"stream":true}`))
	req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, 5)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
}

func TestGroupStreamingPolicyMixedRouting(t *testing.T) {
	for _, allDisabled := range []bool{false, true} {
		groups := []*service.Group{
			{ID: 1, Status: service.StatusActive, Platform: service.PlatformOpenAI, DisableNonStreaming: true, RateMultiplier: 1},
			{ID: 2, Status: service.StatusActive, Platform: service.PlatformOpenAI, DisableNonStreaming: allDisabled, RateMultiplier: 2},
		}
		key, state, err := (&service.APIKeyService{}).PrepareAPIKeyRouting(context.Background(), &service.APIKey{
			MixedMode: true, MixedGroupIDs: []int64{1, 2}, MixedGroups: groups, User: &service.User{ID: 1},
		})
		require.NoError(t, err)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(string(ContextKeyAPIKey), key)
			c.Request = c.Request.WithContext(service.WithAPIKeyRoutingState(c.Request.Context(), state))
		}, GroupStreamingPolicy())
		router.POST("/v1/responses", func(c *gin.Context) {
			require.Equal(t, int64(2), key.Group.ID)
			c.Status(200)
		})
		want := 200
		if allDisabled {
			want = 403
		}
		require.Equal(t, want, doJSON(t, router, "POST", "/v1/responses", `{}`).Code)
	}
}
