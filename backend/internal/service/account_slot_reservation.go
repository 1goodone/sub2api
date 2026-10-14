package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/gin-gonic/gin"
)

const SlotReservationEnabled = "slot_reservation_enabled"
const SlotReservationSeconds = "slot_reservation_seconds"

// Reservations use the ordinary account slot set, so every admission path
// (including Live) counts them. Only idle reservations can be reclaimed.
type AccountSlotReservationCache interface {
	AcquireReservedAccountSlot(context.Context, int64, int, string, string) (bool, error)
	FinishReservedAccountSlot(context.Context, int64, string, string, int) error
	CancelAccountSlotReservation(context.Context, int64, string) error
	GetAccountSlotReservationsBatch(context.Context, []int64) (map[int64][]int64, error)
	GetReservedSessionAccount(context.Context, string) (int64, error)
}

func ValidateSlotReservationExtra(extra map[string]any) error {
	if v, ok := extra[SlotReservationEnabled]; ok {
		if _, valid := v.(bool); !valid {
			return infraerrors.BadRequest("INVALID_SLOT_RESERVATION", "slot_reservation_enabled must be a boolean")
		}
	}
	if v, ok := extra[SlotReservationSeconds]; ok {
		n, valid := accountCostMultiplierNumber(v)
		if !valid || n < 1 || n > 3600 || math.Trunc(n) != n {
			return infraerrors.BadRequest("INVALID_SLOT_RESERVATION", "slot_reservation_seconds must be an integer between 1 and 3600")
		}
	}
	return nil
}

func (a *Account) GetSlotReservationSeconds() int {
	if a == nil || a.Type != AccountTypeOAuth || a.Extra[SlotReservationEnabled] != true {
		return 0
	}
	if n, ok := accountCostMultiplierNumber(a.Extra[SlotReservationSeconds]); ok && n >= 1 && n <= 3600 && math.Trunc(n) == n {
		return int(n)
	}
	return 60
}

type slotReservationContextKey struct{}
type slotReservationState struct {
	mu      sync.Mutex
	userID  int64
	session string
	leases  map[int64]*reservedAccountLease
}
type reservedAccountLease struct {
	mu                       sync.Mutex
	cache                    AccountSlotReservationCache
	accountID                int64
	seconds, depth           int
	started, completed, keep bool
	invalidated              bool
}

func WithAccountSlotReservation(ctx context.Context, userID int64, session string) context.Context {
	return context.WithValue(ctx, slotReservationContextKey{}, &slotReservationState{userID: userID, session: session, leases: make(map[int64]*reservedAccountLease)})
}

// Use the authenticated user and existing routing session when no explicit
// client session header is present. Never store raw client identifiers in Redis.
func SetAccountSlotReservationSession(ctx context.Context, session string) {
	if s, _ := ctx.Value(slotReservationContextKey{}).(*slotReservationState); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.session == "" {
			s.session = session
		}
	}
}

func reservationState(ctx context.Context) (*slotReservationState, string) {
	s, _ := ctx.Value(slotReservationContextKey{}).(*slotReservationState)
	if s == nil {
		return nil, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userID <= 0 || s.session == "" {
		return s, ""
	}
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", s.userID, s.session)))
	return s, hex.EncodeToString(hash[:])
}

// BeginAccountSlotReservation pairs forwarding with its exact acquired lease.
// Nested protocol adapters only finish the lease at their outermost return.
func BeginAccountSlotReservation(ctx context.Context, account *Account, request ...*gin.Context) func(error, bool) {
	s, _ := reservationState(ctx)
	if s == nil || account == nil {
		return func(error, bool) {}
	}
	s.mu.Lock()
	l := s.leases[account.ID]
	s.mu.Unlock()
	if l == nil {
		return func(error, bool) {}
	}
	l.mu.Lock()
	l.started = true
	l.seconds = account.GetSlotReservationSeconds()
	l.depth++
	l.mu.Unlock()
	return func(err error, success bool) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.depth--
		if l.depth > 0 {
			return
		}
		l.completed = true
		// Explicit user-input errors and caller cancellation retain capacity;
		// upstream failures invalidate reservations across the account.
		l.keep = !l.invalidated && ((err == nil && success) || reservationUserError(ctx, err, request))
		if !l.keep {
			l.invalidateLocked()
		}
	}
}

func (l *reservedAccountLease) invalidateLocked() {
	if l.invalidated {
		return
	}
	l.invalidated = true
	l.keep = false
	bg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := l.cache.CancelAccountSlotReservation(bg, l.accountID, ""); err != nil {
		logger.LegacyPrintf("service.concurrency", "cancel reserved account slot %d: %v", l.accountID, err)
	}
}

// An upstream error cancels reservations even when an internal retry later
// succeeds. Only the currently forwarding attempt is affected.
func invalidateActiveSlotReservations(ctx context.Context) {
	s, _ := reservationState(ctx)
	if s == nil {
		return
	}
	s.mu.Lock()
	leases := make([]*reservedAccountLease, 0, len(s.leases))
	for _, lease := range s.leases {
		leases = append(leases, lease)
	}
	s.mu.Unlock()
	for _, lease := range leases {
		lease.mu.Lock()
		if lease.depth > 0 {
			lease.invalidateLocked()
		}
		lease.mu.Unlock()
	}
}

func (s *ConcurrencyService) GetAccountSlotReservationsBatch(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	if s != nil {
		if cache, ok := s.cache.(AccountSlotReservationCache); ok {
			return cache.GetAccountSlotReservationsBatch(ctx, ids)
		}
	}
	return nil, nil
}

func (s *ConcurrencyService) reservedSessionAccount(ctx context.Context) int64 {
	_, session := reservationState(ctx)
	if s != nil && session != "" {
		if cache, ok := s.cache.(AccountSlotReservationCache); ok {
			id, err := cache.GetReservedSessionAccount(ctx, session)
			if err == nil {
				return id
			}
		}
	}
	return 0
}

// Use structured status supplied by the gateway, never error-string matching.
// 401/403/429 and upstream failures take precedence over client cancellation.
func reservationUserError(ctx context.Context, err error, request []*gin.Context) bool {
	var failover *UpstreamFailoverError
	if errors.As(err, &failover) {
		return false
	}
	if len(request) > 0 && request[0] != nil {
		c := request[0]
		upstream := c.GetInt(OpsUpstreamStatusCodeKey)
		if upstream == 401 || upstream == 402 || upstream == 403 || upstream == 429 || upstream >= 500 {
			return false
		}
		if isOpenAIContextWindowError(c.GetString(OpsUpstreamErrorMessageKey), []byte(c.GetString(OpsUpstreamErrorDetailKey))) {
			return true
		}
		if c.Writer != nil && c.Writer.Status() == 400 && (upstream == 0 || upstream == 400) {
			return true
		}
	}
	return (err == nil || errors.Is(err, context.Canceled) || IsOpenAIWSClientGoneError(err)) && errors.Is(ctx.Err(), context.Canceled)
}
