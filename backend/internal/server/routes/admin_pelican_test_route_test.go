package routes

import (
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminPelicanAccountTestRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{Account: &adminhandler.AccountHandler{}}}
	stepUp := middleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })

	registerAccountRoutes(admin, handlers, stepUp)

	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/v1/admin/accounts/:id/pelican-test" {
			return
		}
	}
	require.Fail(t, "POST /api/v1/admin/accounts/:id/pelican-test should be registered")
}
