package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var ErrNoUsableMixedGroup = infraerrors.Forbidden(
	"NO_USABLE_MIXED_GROUP",
	"none of the API key's automatic routing groups are currently available",
)

type ChannelRoutingHealthReader interface {
	UnavailableGroupIDs(ctx context.Context, groupIDs []int64) (map[int64]bool, error)
}

type APIKeyRoutingCandidate struct {
	Group        *Group
	Subscription *UserSubscription
	Rate         float64
}

type APIKeyRoutingState struct {
	mu               sync.RWMutex
	apiKey           *APIKey
	candidates       []APIKeyRoutingCandidate
	current          int
	billingChecked   map[int64]bool
	userRPMCommitted bool
}

type apiKeyRoutingContextKey struct{}

func WithAPIKeyRoutingState(ctx context.Context, state *APIKeyRoutingState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, apiKeyRoutingContextKey{}, state)
}

func APIKeyRoutingStateFromContext(ctx context.Context) (*APIKeyRoutingState, bool) {
	if ctx == nil {
		return nil, false
	}
	state, ok := ctx.Value(apiKeyRoutingContextKey{}).(*APIKeyRoutingState)
	return state, ok && state != nil
}

func (s *APIKeyService) PrepareAPIKeyRouting(ctx context.Context, apiKey *APIKey) (*APIKey, *APIKeyRoutingState, error) {
	return s.prepareAPIKeyRoutingAt(ctx, apiKey, timezone.Now())
}

func (s *APIKeyService) prepareAPIKeyRoutingAt(ctx context.Context, apiKey *APIKey, now time.Time) (*APIKey, *APIKeyRoutingState, error) {
	if apiKey == nil || !apiKey.MixedMode {
		return apiKey, nil, nil
	}
	if apiKey.User == nil {
		return nil, nil, ErrNoUsableMixedGroup
	}

	groups := make(map[int64]*Group, len(apiKey.MixedGroups)+1)
	for _, group := range apiKey.MixedGroups {
		if group != nil {
			groups[group.ID] = group
		}
	}
	if apiKey.Group != nil {
		groups[apiKey.Group.ID] = apiKey.Group
	}

	rates, _ := s.GetUserGroupRates(ctx, apiKey.UserID)
	candidates := make([]APIKeyRoutingCandidate, 0, len(apiKey.MixedGroupIDs))
	platform := ""
	for _, groupID := range apiKey.MixedGroupIDs {
		group := groups[groupID]
		if group == nil {
			loaded, err := s.groupRepo.GetByID(ctx, groupID)
			if err != nil {
				if errors.Is(err, ErrGroupNotFound) {
					continue
				}
				return nil, nil, fmt.Errorf("load automatic routing group %d: %w", groupID, err)
			}
			group = loaded
		}
		if !group.IsActive() {
			continue
		}
		if platform == "" {
			platform = group.Platform
		} else if group.Platform != platform {
			continue
		}

		var subscription *UserSubscription
		if group.IsSubscriptionType() {
			if s.userSubRepo == nil {
				continue
			}
			active, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, apiKey.UserID, group.ID)
			if err != nil {
				if errors.Is(err, ErrSubscriptionNotFound) {
					continue
				}
				return nil, nil, fmt.Errorf("load automatic routing subscription %d: %w", group.ID, err)
			}
			subscription = active
		} else if !apiKey.User.CanBindGroup(group.ID, group.IsExclusive) {
			continue
		}

		rate := group.RateMultiplier
		if override, ok := rates[group.ID]; ok {
			rate = override
		}
		rate *= group.PeakMultiplierAt(now)
		candidates = append(candidates, APIKeyRoutingCandidate{Group: group, Subscription: subscription, Rate: rate})
	}
	if len(candidates) == 0 {
		return nil, nil, ErrNoUsableMixedGroup
	}

	if s.routingHealth != nil {
		groupIDs := make([]int64, 0, len(candidates))
		for _, candidate := range candidates {
			groupIDs = append(groupIDs, candidate.Group.ID)
		}
		if unavailable, err := s.routingHealth.UnavailableGroupIDs(ctx, groupIDs); err == nil {
			filtered := candidates[:0]
			for _, candidate := range candidates {
				if !unavailable[candidate.Group.ID] {
					filtered = append(filtered, candidate)
				}
			}
			candidates = filtered
		}
	}
	if len(candidates) == 0 {
		return nil, nil, ErrNoUsableMixedGroup
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].Rate < candidates[j].Rate
	})

	effective := *apiKey
	if apiKey.User != nil {
		user := *apiKey.User
		user.UserGroupRPMOverride = nil
		effective.User = &user
	}
	state := &APIKeyRoutingState{
		apiKey:         &effective,
		candidates:     candidates,
		billingChecked: make(map[int64]bool, len(candidates)),
	}
	state.applyLocked(0)
	return &effective, state, nil
}

func (s *APIKeyRoutingState) CandidateGroupIDs(groupID *int64) ([]int64, bool) {
	if s == nil || groupID == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	start := -1
	for i := s.current; i < len(s.candidates); i++ {
		if s.candidates[i].Group != nil && s.candidates[i].Group.ID == *groupID {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	ids := make([]int64, 0, len(s.candidates)-start)
	for i := start; i < len(s.candidates); i++ {
		if s.candidates[i].Group != nil {
			ids = append(ids, s.candidates[i].Group.ID)
		}
	}
	return ids, len(ids) > 0
}

func (s *APIKeyRoutingState) ApplyGroup(groupID int64) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := s.current; i < len(s.candidates); i++ {
		if s.candidates[i].Group != nil && s.candidates[i].Group.ID == groupID {
			s.applyLocked(i)
			return true
		}
	}
	return false
}

func (s *APIKeyRoutingState) restrictCandidates(keep func(*Group) bool) bool {
	if s == nil || keep == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	filtered := make([]APIKeyRoutingCandidate, 0, len(s.candidates))
	for _, candidate := range s.candidates {
		if candidate.Group != nil && keep(candidate.Group) {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return false
	}
	s.candidates = filtered
	s.current = 0
	s.applyLocked(0)
	return true
}

func RestrictAPIKeyRoutingCandidates(ctx context.Context, keep func(*Group) bool) bool {
	state, ok := APIKeyRoutingStateFromContext(ctx)
	if !ok {
		return true
	}
	return state.restrictCandidates(keep)
}

func (s *APIKeyRoutingState) applyLocked(index int) {
	if index < 0 || index >= len(s.candidates) || s.apiKey == nil {
		return
	}
	candidate := s.candidates[index]
	groupID := candidate.Group.ID
	s.current = index
	s.apiKey.GroupID = &groupID
	s.apiKey.Group = candidate.Group
	if s.apiKey.User != nil {
		s.apiKey.User.UserGroupRPMOverride = nil
	}
}

func (s *APIKeyRoutingState) Candidate(groupID int64) (APIKeyRoutingCandidate, bool) {
	if s == nil {
		return APIKeyRoutingCandidate{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := s.current; i < len(s.candidates); i++ {
		candidate := s.candidates[i]
		if candidate.Group != nil && candidate.Group.ID == groupID {
			return candidate, true
		}
	}
	return APIKeyRoutingCandidate{}, false
}

func (s *APIKeyRoutingState) APIKeyForCandidate(groupID int64) (*APIKey, APIKeyRoutingCandidate, bool) {
	if s == nil {
		return nil, APIKeyRoutingCandidate{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := s.current; i < len(s.candidates); i++ {
		candidate := s.candidates[i]
		if candidate.Group == nil || candidate.Group.ID != groupID || s.apiKey == nil {
			continue
		}
		key := *s.apiKey
		id := groupID
		key.GroupID = &id
		key.Group = candidate.Group
		if s.apiKey.User != nil {
			user := *s.apiKey.User
			user.UserGroupRPMOverride = nil
			key.User = &user
		}
		return &key, candidate, true
	}
	return nil, APIKeyRoutingCandidate{}, false
}

func (s *APIKeyRoutingState) CurrentSubscription() *UserSubscription {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.current < 0 || s.current >= len(s.candidates) {
		return nil
	}
	return s.candidates[s.current].Subscription
}

func (s *APIKeyRoutingState) BillingChecked(groupID int64) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.billingChecked[groupID]
}

func (s *APIKeyRoutingState) MarkBillingChecked(groupID int64) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.billingChecked[groupID] = true
	s.userRPMCommitted = true
	s.mu.Unlock()
}

func (s *APIKeyRoutingState) UserRPMCommitted() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userRPMCommitted
}
