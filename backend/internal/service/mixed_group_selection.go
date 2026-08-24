package service

import (
	"context"
	"errors"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

type skipMixedGroupSelectionKey struct{}
type mixedGroupMessagesDispatchModelKey struct{}

func withoutMixedGroupSelection(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipMixedGroupSelectionKey{}, true)
}

func WithMixedGroupMessagesDispatchModel(ctx context.Context, requestedModel string) context.Context {
	requestedModel = strings.TrimSpace(requestedModel)
	if ctx == nil || requestedModel == "" {
		return ctx
	}
	return context.WithValue(ctx, mixedGroupMessagesDispatchModelKey{}, requestedModel)
}

func mixedGroupCandidateRequestedModel(ctx context.Context, state *APIKeyRoutingState, groupID int64, fallback string) string {
	requestedModel, _ := ctx.Value(mixedGroupMessagesDispatchModelKey{}).(string)
	if requestedModel == "" || state == nil {
		return fallback
	}
	candidate, ok := state.Candidate(groupID)
	if !ok || candidate.Group == nil {
		return fallback
	}
	if mapped := strings.TrimSpace(candidate.Group.ResolveMessagesDispatchModel(requestedModel)); mapped != "" {
		return mapped
	}
	return fallback
}

func mixedGroupSelectionIDs(ctx context.Context, groupID *int64) ([]int64, *APIKeyRoutingState, bool) {
	if ctx == nil {
		return nil, nil, false
	}
	if skip, _ := ctx.Value(skipMixedGroupSelectionKey{}).(bool); skip {
		return nil, nil, false
	}
	state, ok := APIKeyRoutingStateFromContext(ctx)
	if !ok {
		return nil, nil, false
	}
	ids, ok := state.CandidateGroupIDs(groupID)
	return ids, state, ok
}

func withMixedGroupCandidateContext(ctx context.Context, state *APIKeyRoutingState, groupID int64) context.Context {
	if ctx == nil || state == nil {
		return ctx
	}
	candidate, ok := state.Candidate(groupID)
	if !ok || !IsGroupContextValid(candidate.Group) {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, candidate.Group)
}

func checkMixedGroupSelectionBilling(ctx context.Context, state *APIKeyRoutingState, groupID int64, billing *BillingCacheService) error {
	if state == nil || state.BillingChecked(groupID) {
		return nil
	}
	apiKey, candidate, ok := state.APIKeyForCandidate(groupID)
	if !ok {
		return ErrNoUsableMixedGroup
	}
	if billing != nil {
		if err := billing.CheckMixedGroupBillingEligibility(
			ctx,
			apiKey.User,
			apiKey,
			candidate.Group,
			candidate.Subscription,
			QuotaPlatform(ctx, apiKey),
			!state.UserRPMCommitted(),
		); err != nil {
			return err
		}
	}
	state.MarkBillingChecked(groupID)
	return nil
}

func shouldTryNextMixedGroupAfterBillingError(err error) bool {
	return errors.Is(err, ErrSubscriptionInvalid) ||
		errors.Is(err, ErrDailyLimitExceeded) ||
		errors.Is(err, ErrWeeklyLimitExceeded) ||
		errors.Is(err, ErrMonthlyLimitExceeded) ||
		errors.Is(err, ErrGroupRPMExceeded)
}

func shouldTryNextMixedGroupAfterSelectionError(err error) bool {
	return errors.Is(err, ErrNoAvailableAccounts) ||
		errors.Is(err, ErrNoAvailableCompactAccounts) ||
		errors.Is(err, ErrClaudeCodeOnly)
}

func CurrentAPIKeyRoutingSubscription(ctx context.Context, fallback *UserSubscription) *UserSubscription {
	state, ok := APIKeyRoutingStateFromContext(ctx)
	if !ok {
		return fallback
	}
	return state.CurrentSubscription()
}
