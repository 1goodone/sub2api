package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func newDesktopPairingStoreTest(t *testing.T) (desktopPairingStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return desktopPairingStore{redis: client}, server
}

func TestDesktopPairingStoreRequiresApprovalAndConsumesOnce(t *testing.T) {
	store, _ := newDesktopPairingStoreTest(t)
	ctx := context.Background()
	requestID := strings.Repeat("r", 43)
	verifier := strings.Repeat("v", 43)

	require.NoError(t, store.start(ctx, requestID, pkceChallenge(verifier)))
	_, err := store.exchange(ctx, requestID, verifier)
	require.ErrorIs(t, err, errDesktopPairingPending)

	_, err = store.exchange(ctx, requestID, strings.Repeat("w", 43))
	require.ErrorIs(t, err, errDesktopPairingVerifier)

	require.NoError(t, store.approve(ctx, requestID, 42))
	userID, err := store.exchange(ctx, requestID, verifier)
	require.NoError(t, err)
	require.Equal(t, int64(42), userID)

	_, err = store.exchange(ctx, requestID, verifier)
	require.ErrorIs(t, err, errDesktopPairingExpired)
}

func TestDesktopPairingStoreApprovalIsIdempotentForSameUser(t *testing.T) {
	store, _ := newDesktopPairingStoreTest(t)
	ctx := context.Background()
	requestID := strings.Repeat("r", 43)

	require.NoError(t, store.start(ctx, requestID, pkceChallenge(strings.Repeat("v", 43))))
	require.NoError(t, store.approve(ctx, requestID, 42))
	require.NoError(t, store.approve(ctx, requestID, 42))
	require.ErrorIs(t, store.approve(ctx, requestID, 43), errDesktopPairingConflict)
}

func TestDesktopPairingStoreExpires(t *testing.T) {
	store, server := newDesktopPairingStoreTest(t)
	ctx := context.Background()
	requestID := strings.Repeat("r", 43)
	verifier := strings.Repeat("v", 43)

	require.NoError(t, store.start(ctx, requestID, pkceChallenge(verifier)))
	server.FastForward(desktopPairingTTL + time.Second)
	require.ErrorIs(t, store.approve(ctx, requestID, 42), errDesktopPairingExpired)
	_, err := store.exchange(ctx, requestID, verifier)
	require.ErrorIs(t, err, errDesktopPairingExpired)
}

func TestDesktopPairingValidation(t *testing.T) {
	requestID, err := randomDesktopPairingID()
	require.NoError(t, err)
	require.True(t, validateDesktopPairingID(requestID))
	require.False(t, validateDesktopPairingID(requestID+"x"))
	require.True(t, validatePKCEVerifier(strings.Repeat("a", 43)))
	require.False(t, validatePKCEVerifier(strings.Repeat("a", 42)))
	require.False(t, validatePKCEVerifier(strings.Repeat("a", 42)+"!"))
	require.Equal(t, strings.ToUpper(requestID[:4]+"-"+requestID[len(requestID)-4:]), desktopPairingUserCode(requestID))
}

type desktopUserReaderStub struct {
	user *service.User
	err  error
}

func (s *desktopUserReaderStub) GetByID(context.Context, int64) (*service.User, error) {
	return s.user, s.err
}

func TestDesktopDeviceSessionStoresOnlyHashAndSlidesExpiry(t *testing.T) {
	store, server := newDesktopPairingStoreTest(t)
	ctx := context.Background()
	token, err := store.issueDeviceSession(ctx, 42, 7)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(token, "dt_"))

	keys := server.Keys()
	require.Len(t, keys, 1)
	require.NotContains(t, keys[0], token)
	require.Equal(t, "42", server.HGet(keys[0], "user_id"))
	require.Equal(t, "7", server.HGet(keys[0], "token_version"))

	server.FastForward(24 * time.Hour)
	session, err := store.verifyDeviceSession(ctx, token)
	require.NoError(t, err)
	require.Equal(t, desktopDeviceSession{UserID: 42, TokenVersion: 7}, session)
	require.Equal(t, desktopDeviceSessionTTL, server.TTL(keys[0]))
}

func TestDesktopDeviceAuthenticationRejectsWrongTokenVersionAndInactiveUser(t *testing.T) {
	store, _ := newDesktopPairingStoreTest(t)
	token, err := store.issueDeviceSession(context.Background(), 42, 7)
	require.NoError(t, err)

	reader := &desktopUserReaderStub{user: &service.User{
		ID: 42, Email: "user@example.com", Role: service.RoleUser,
		Status: service.StatusActive, TokenVersion: 8, Concurrency: 2,
	}}
	handler := &DesktopAuthHandler{store: store, userService: reader}
	router := gin.New()
	router.GET("/desktop", handler.Authenticate(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)

	reader.user.TokenVersion = 7
	reader.user.Status = service.StatusDisabled
	request = httptest.NewRequest(http.MethodGet, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestDesktopDeviceAuthenticationAcceptsDeviceTokenAndRevokesIt(t *testing.T) {
	store, _ := newDesktopPairingStoreTest(t)
	token, err := store.issueDeviceSession(context.Background(), 42, 7)
	require.NoError(t, err)

	handler := &DesktopAuthHandler{
		store: store,
		userService: &desktopUserReaderStub{user: &service.User{
			ID: 42, Email: "user@example.com", Role: service.RoleUser,
			Status: service.StatusActive, TokenVersion: 7, Concurrency: 2,
		}},
	}
	router := gin.New()
	router.GET("/desktop", handler.Authenticate(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.DELETE("/desktop", handler.Authenticate(), handler.Revoke)

	request := httptest.NewRequest(http.MethodGet, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer regular.jwt.token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)

	request = httptest.NewRequest(http.MethodGet, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)

	request = httptest.NewRequest(http.MethodDelete, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)

	request = httptest.NewRequest(http.MethodGet, "/desktop", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}
