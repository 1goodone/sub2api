package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

const (
	desktopPairingTTL       = 10 * time.Minute
	desktopDeviceSessionTTL = 90 * 24 * time.Hour
	desktopDeviceTokenKey   = "desktop_device_token"
)

var (
	errDesktopPairingExpired  = errors.New("desktop pairing request expired")
	errDesktopPairingPending  = errors.New("desktop pairing request pending")
	errDesktopPairingVerifier = errors.New("desktop pairing verifier mismatch")
	errDesktopPairingConflict = errors.New("desktop pairing request already approved")
)

var desktopPairingStartScript = redis.NewScript(`
redis.call("HSET", KEYS[1], "challenge", ARGV[1], "status", "pending")
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
`)

var desktopPairingApproveScript = redis.NewScript(`
local status = redis.call("HGET", KEYS[1], "status")
if not status then
  return 0
end
if status == "approved" then
  local current_user = redis.call("HGET", KEYS[1], "user_id")
  if current_user == ARGV[1] then
    return 2
  end
  return -1
end
if status ~= "pending" then
  return -1
end
redis.call("HSET", KEYS[1], "status", "approved", "user_id", ARGV[1])
return 1
`)

var desktopPairingExchangeScript = redis.NewScript(`
local challenge = redis.call("HGET", KEYS[1], "challenge")
if not challenge then
  return {"expired"}
end
if challenge ~= ARGV[1] then
  return {"invalid_verifier"}
end
local status = redis.call("HGET", KEYS[1], "status")
if status == "pending" then
  return {"pending"}
end
if status ~= "approved" then
  return {"expired"}
end
local user_id = redis.call("HGET", KEYS[1], "user_id")
redis.call("DEL", KEYS[1])
return {"approved", user_id}
`)

var desktopDeviceSessionIssueScript = redis.NewScript(`
redis.call("HSET", KEYS[1], "user_id", ARGV[1], "token_version", ARGV[2])
redis.call("EXPIRE", KEYS[1], ARGV[3])
return 1
`)

var desktopDeviceSessionVerifyScript = redis.NewScript(`
local user_id = redis.call("HGET", KEYS[1], "user_id")
local token_version = redis.call("HGET", KEYS[1], "token_version")
if not user_id or not token_version then
  return {}
end
redis.call("EXPIRE", KEYS[1], ARGV[1])
return {user_id, token_version}
`)

type desktopPairingStore struct {
	redis *redis.Client
}

func (s desktopPairingStore) key(requestID string) string {
	hash := sha256.Sum256([]byte(requestID))
	return "auth:desktop_pair:" + base64.RawURLEncoding.EncodeToString(hash[:])
}

func (s desktopPairingStore) deviceKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return "auth:desktop_session:" + base64.RawURLEncoding.EncodeToString(hash[:])
}

func (s desktopPairingStore) start(ctx context.Context, requestID, challenge string) error {
	if s.redis == nil {
		return errors.New("desktop pairing store unavailable")
	}
	return desktopPairingStartScript.Run(
		ctx,
		s.redis,
		[]string{s.key(requestID)},
		challenge,
		int(desktopPairingTTL.Seconds()),
	).Err()
}

func (s desktopPairingStore) approve(ctx context.Context, requestID string, userID int64) error {
	if s.redis == nil {
		return errors.New("desktop pairing store unavailable")
	}
	result, err := desktopPairingApproveScript.Run(
		ctx,
		s.redis,
		[]string{s.key(requestID)},
		strconv.FormatInt(userID, 10),
	).Int()
	if err != nil {
		return err
	}
	switch result {
	case 0:
		return errDesktopPairingExpired
	case -1:
		return errDesktopPairingConflict
	default:
		return nil
	}
}

func (s desktopPairingStore) exchange(ctx context.Context, requestID, verifier string) (int64, error) {
	if s.redis == nil {
		return 0, errors.New("desktop pairing store unavailable")
	}
	values, err := desktopPairingExchangeScript.Run(
		ctx,
		s.redis,
		[]string{s.key(requestID)},
		pkceChallenge(verifier),
	).Slice()
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, errDesktopPairingExpired
	}
	status := fmt.Sprint(values[0])
	switch status {
	case "pending":
		return 0, errDesktopPairingPending
	case "invalid_verifier":
		return 0, errDesktopPairingVerifier
	case "approved":
		if len(values) != 2 {
			return 0, errDesktopPairingExpired
		}
		userID, err := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
		if err != nil || userID <= 0 {
			return 0, errDesktopPairingExpired
		}
		return userID, nil
	default:
		return 0, errDesktopPairingExpired
	}
}

type desktopDeviceSession struct {
	UserID       int64
	TokenVersion int64
}

func (s desktopPairingStore) issueDeviceSession(ctx context.Context, userID, tokenVersion int64) (string, error) {
	if s.redis == nil {
		return "", errors.New("desktop session store unavailable")
	}
	tokenID, err := randomDesktopPairingID()
	if err != nil {
		return "", err
	}
	token := "dt_" + tokenID
	if err := desktopDeviceSessionIssueScript.Run(
		ctx,
		s.redis,
		[]string{s.deviceKey(token)},
		strconv.FormatInt(userID, 10),
		strconv.FormatInt(tokenVersion, 10),
		int(desktopDeviceSessionTTL.Seconds()),
	).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s desktopPairingStore) verifyDeviceSession(ctx context.Context, token string) (desktopDeviceSession, error) {
	if s.redis == nil {
		return desktopDeviceSession{}, errors.New("desktop session store unavailable")
	}
	values, err := desktopDeviceSessionVerifyScript.Run(
		ctx,
		s.redis,
		[]string{s.deviceKey(token)},
		int(desktopDeviceSessionTTL.Seconds()),
	).Slice()
	if err != nil {
		return desktopDeviceSession{}, err
	}
	if len(values) != 2 {
		return desktopDeviceSession{}, errDesktopPairingExpired
	}
	userID, userErr := strconv.ParseInt(fmt.Sprint(values[0]), 10, 64)
	tokenVersion, versionErr := strconv.ParseInt(fmt.Sprint(values[1]), 10, 64)
	if userErr != nil || versionErr != nil || userID <= 0 {
		return desktopDeviceSession{}, errDesktopPairingExpired
	}
	return desktopDeviceSession{UserID: userID, TokenVersion: tokenVersion}, nil
}

func (s desktopPairingStore) revokeDeviceSession(ctx context.Context, token string) error {
	if s.redis == nil {
		return errors.New("desktop session store unavailable")
	}
	return s.redis.Del(ctx, s.deviceKey(token)).Err()
}

type desktopUserReader interface {
	GetByID(ctx context.Context, id int64) (*service.User, error)
}

type DesktopAuthHandler struct {
	store       desktopPairingStore
	authHandler *AuthHandler
	userService desktopUserReader
}

func NewDesktopAuthHandler(redisClient *redis.Client, authHandler *AuthHandler) *DesktopAuthHandler {
	var userService desktopUserReader
	if authHandler != nil {
		userService = authHandler.userService
	}
	return &DesktopAuthHandler{
		store:       desktopPairingStore{redis: redisClient},
		authHandler: authHandler,
		userService: userService,
	}
}

type desktopPairingStartRequest struct {
	CodeChallenge string `json:"code_challenge" binding:"required"`
}

type desktopPairingStartResponse struct {
	RequestID        string `json:"request_id"`
	UserCode         string `json:"user_code"`
	VerificationPath string `json:"verification_path"`
	ExpiresIn        int    `json:"expires_in"`
}

type desktopPairingRequest struct {
	RequestID string `json:"request_id" binding:"required"`
}

type desktopPairingTokenRequest struct {
	RequestID    string `json:"request_id" binding:"required"`
	CodeVerifier string `json:"code_verifier" binding:"required"`
}

type desktopPairingTokenResponse struct {
	Status      string `json:"status"`
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	UserEmail   string `json:"user_email,omitempty"`
	ExpiresIn   int    `json:"expires_in,omitempty"`
}

func desktopBearerToken(c *gin.Context) (string, bool) {
	parts := strings.SplitN(c.GetHeader("Authorization"), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	token := strings.TrimSpace(parts[1])
	if len(token) != 46 || !strings.HasPrefix(token, "dt_") || !validateDesktopPairingID(strings.TrimPrefix(token, "dt_")) {
		return "", false
	}
	return token, true
}

func (h *DesktopAuthHandler) Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := desktopBearerToken(c)
		if !ok {
			middleware2.AbortWithError(c, http.StatusUnauthorized, "INVALID_DESKTOP_TOKEN", "A valid desktop device token is required")
			return
		}
		session, err := h.store.verifyDeviceSession(c.Request.Context(), token)
		if err != nil || h.userService == nil {
			middleware2.AbortWithError(c, http.StatusUnauthorized, "DESKTOP_SESSION_EXPIRED", "Desktop session has expired")
			return
		}
		user, err := h.userService.GetByID(c.Request.Context(), session.UserID)
		if err != nil || user == nil {
			middleware2.AbortWithError(c, http.StatusUnauthorized, "USER_NOT_FOUND", "User not found")
			return
		}
		if !user.IsActive() {
			middleware2.AbortWithError(c, http.StatusUnauthorized, "USER_INACTIVE", "User account is not active")
			return
		}
		if session.TokenVersion != user.TokenVersion {
			_ = h.store.revokeDeviceSession(c.Request.Context(), token)
			middleware2.AbortWithError(c, http.StatusUnauthorized, "TOKEN_REVOKED", "Desktop session has been revoked")
			return
		}

		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: user.ID, Concurrency: user.Concurrency})
		c.Set(string(middleware2.ContextKeyUserRole), user.Role)
		c.Set(middleware2.ContextKeyAuthEmail, user.Email)
		c.Set(desktopDeviceTokenKey, token)
		c.Next()
	}
}

func (h *DesktopAuthHandler) Revoke(c *gin.Context) {
	token := c.GetString(desktopDeviceTokenKey)
	if token == "" {
		response.Unauthorized(c, "Desktop session not authenticated")
		return
	}
	if err := h.store.revokeDeviceSession(c.Request.Context(), token); err != nil {
		response.InternalError(c, "Desktop session could not be revoked")
		return
	}
	response.Success(c, gin.H{"status": "revoked"})
}

func validateDesktopPairingID(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validatePKCEChallenge(value string) bool {
	return validateDesktopPairingID(value)
}

func validatePKCEVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("-._~", char) {
			continue
		}
		return false
	}
	return true
}

func randomDesktopPairingID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func pkceChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func desktopPairingUserCode(requestID string) string {
	if len(requestID) < 8 {
		return ""
	}
	return strings.ToUpper(requestID[:4] + "-" + requestID[len(requestID)-4:])
}

func (h *DesktopAuthHandler) Start(c *gin.Context) {
	var req desktopPairingStartRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validatePKCEChallenge(req.CodeChallenge) {
		response.BadRequest(c, "Invalid PKCE code challenge")
		return
	}
	requestID, err := randomDesktopPairingID()
	if err != nil {
		response.InternalError(c, "Failed to start desktop authorization")
		return
	}
	if err := h.store.start(c.Request.Context(), requestID, req.CodeChallenge); err != nil {
		response.InternalError(c, "Desktop authorization is temporarily unavailable")
		return
	}
	query := url.Values{"request": []string{requestID}}
	response.Success(c, desktopPairingStartResponse{
		RequestID:        requestID,
		UserCode:         desktopPairingUserCode(requestID),
		VerificationPath: "/desktop-connect?" + query.Encode(),
		ExpiresIn:        int(desktopPairingTTL.Seconds()),
	})
}

func (h *DesktopAuthHandler) Approve(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req desktopPairingRequest
	if err := c.ShouldBindJSON(&req); err != nil || !validateDesktopPairingID(req.RequestID) {
		response.BadRequest(c, "Invalid desktop authorization request")
		return
	}
	if err := h.store.approve(c.Request.Context(), req.RequestID, subject.UserID); err != nil {
		switch {
		case errors.Is(err, errDesktopPairingExpired):
			response.NotFound(c, "Desktop authorization request expired")
		case errors.Is(err, errDesktopPairingConflict):
			response.Error(c, http.StatusConflict, "Desktop authorization request was already approved")
		default:
			response.InternalError(c, "Desktop authorization is temporarily unavailable")
		}
		return
	}
	response.Success(c, gin.H{"status": "approved"})
}

func (h *DesktopAuthHandler) Exchange(c *gin.Context) {
	var req desktopPairingTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil ||
		!validateDesktopPairingID(req.RequestID) ||
		!validatePKCEVerifier(req.CodeVerifier) {
		response.BadRequest(c, "Invalid desktop authorization request")
		return
	}
	userID, err := h.store.exchange(c.Request.Context(), req.RequestID, req.CodeVerifier)
	if err != nil {
		switch {
		case errors.Is(err, errDesktopPairingPending):
			response.Accepted(c, desktopPairingTokenResponse{Status: "pending"})
		case errors.Is(err, errDesktopPairingVerifier):
			response.Unauthorized(c, "Invalid PKCE verifier")
		case errors.Is(err, errDesktopPairingExpired):
			response.NotFound(c, "Desktop authorization request expired")
		default:
			response.InternalError(c, "Desktop authorization is temporarily unavailable")
		}
		return
	}
	if h.authHandler == nil || h.userService == nil {
		response.InternalError(c, "Desktop authorization is temporarily unavailable")
		return
	}
	user, err := h.userService.GetByID(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := ensureLoginUserActive(user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.authHandler.ensureBackendModeAllowsUser(c.Request.Context(), user); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	token, err := h.store.issueDeviceSession(c.Request.Context(), user.ID, user.TokenVersion)
	if err != nil {
		response.InternalError(c, "Failed to complete desktop authorization")
		return
	}
	response.Success(c, desktopPairingTokenResponse{
		Status:      "approved",
		AccessToken: token,
		TokenType:   "Bearer",
		UserEmail:   user.Email,
		ExpiresIn:   int(desktopDeviceSessionTTL.Seconds()),
	})
}
