package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newAuthRoutesTestRouter(redisClient *redis.Client) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterAuthRoutes(
		v1,
		&handler.Handlers{
			Auth:           &handler.AuthHandler{},
			User:           &handler.UserHandler{},
			APIKey:         &handler.APIKeyHandler{},
			Usage:          &handler.UsageHandler{},
			Subscription:   &handler.SubscriptionHandler{},
			ChannelMonitor: &handler.ChannelMonitorUserHandler{},
			Setting:        &handler.SettingHandler{},
		},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		servermiddleware.AuditLogMiddleware(func(c *gin.Context) {
			c.Next()
		}),
		redisClient,
		nil,
		nil,
	)

	return router
}

func TestDesktopRoutesExposeOnlyRestrictedPersonalSurface(t *testing.T) {
	router := newAuthRoutesTestRouter(nil)
	actual := make([]string, 0)
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "/api/v1/desktop/") {
			actual = append(actual, route.Method+" "+route.Path)
		}
	}

	expected := []string{
		"GET /api/v1/desktop/profile",
		"GET /api/v1/desktop/keys",
		"GET /api/v1/desktop/keys/:id",
		"PUT /api/v1/desktop/keys/:id/group",
		"GET /api/v1/desktop/groups/available",
		"GET /api/v1/desktop/usage/stats",
		"GET /api/v1/desktop/usage/dashboard/stats",
		"GET /api/v1/desktop/usage/dashboard/snapshot-v2",
		"GET /api/v1/desktop/subscriptions/summary",
		"GET /api/v1/desktop/channel-monitors",
		"DELETE /api/v1/desktop/session",
	}
	require.ElementsMatch(t, expected, actual)
}

func TestAuthRoutesRateLimitFailCloseWhenRedisUnavailable(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() {
		_ = rdb.Close()
	})

	router := newAuthRoutesTestRouter(rdb)
	paths := []string{
		"/api/v1/auth/register",
		"/api/v1/auth/login",
		"/api/v1/auth/login/2fa",
		"/api/v1/auth/desktop/start",
		"/api/v1/auth/desktop/token",
		"/api/v1/auth/send-verify-code",
		"/api/v1/auth/oauth/pending/send-verify-code",
	}

	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.10:12345"

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusTooManyRequests, w.Code, "path=%s", path)
		require.Contains(t, w.Body.String(), "rate limit exceeded", "path=%s", path)
	}
}
