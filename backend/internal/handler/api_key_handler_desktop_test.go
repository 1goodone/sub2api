package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyHandlerUpdateGroupRejectsInvalidDesktopInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Next()
	})
	router.PUT("/desktop/keys/:id/group", (&APIKeyHandler{}).UpdateGroup)

	for _, test := range []struct {
		name string
		id   string
		body string
	}{
		{name: "invalid key id", id: "invalid", body: `{"group_id":1}`},
		{name: "missing group", id: "1", body: `{}`},
		{name: "zero group", id: "1", body: `{"group_id":0}`},
		{name: "negative group", id: "1", body: `{"group_id":-1}`},
		{name: "non numeric group", id: "1", body: `{"group_id":"1"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/desktop/keys/"+test.id+"/group", strings.NewReader(test.body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, req)

			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestAPIKeyHandlerUpdateGroupRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/desktop/keys/:id/group", (&APIKeyHandler{}).UpdateGroup)

	req := httptest.NewRequest(http.MethodPut, "/desktop/keys/1/group", strings.NewReader(`{"group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	require.Equal(t, http.StatusUnauthorized, response.Code)
}
