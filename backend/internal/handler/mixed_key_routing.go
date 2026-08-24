package handler

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type channelMappingResolver interface {
	ResolveChannelMappingAndRestrict(context.Context, *int64, string) (service.ChannelMappingResult, bool)
}

func checkInitialBillingEligibility(
	ctx context.Context,
	billing *service.BillingCacheService,
	user *service.User,
	apiKey *service.APIKey,
	group *service.Group,
	subscription *service.UserSubscription,
	platform string,
) error {
	if apiKey != nil && apiKey.MixedMode {
		if _, ok := service.APIKeyRoutingStateFromContext(ctx); ok {
			return nil
		}
	}
	return billing.CheckBillingEligibility(ctx, user, apiKey, group, subscription, platform)
}

func syncCurrentAPIKeyRouting(
	c *gin.Context,
	ctx context.Context,
	apiKey *service.APIKey,
	fallback *service.UserSubscription,
) (context.Context, *service.UserSubscription) {
	state, ok := service.APIKeyRoutingStateFromContext(ctx)
	if !ok {
		return ctx, fallback
	}
	if apiKey != nil && service.IsGroupContextValid(apiKey.Group) {
		ctx = context.WithValue(ctx, ctxkey.Group, apiKey.Group)
		if c != nil {
			c.Request = c.Request.WithContext(ctx)
		}
	}
	subscription := state.CurrentSubscription()
	if c != nil {
		c.Set(string(middleware.ContextKeySubscription), subscription)
	}
	return ctx, subscription
}

func refreshMixedKeyRouting(
	c *gin.Context,
	ctx context.Context,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	resolver channelMappingResolver,
	model string,
	mapping service.ChannelMappingResult,
) (context.Context, *service.UserSubscription, service.ChannelMappingResult) {
	if _, ok := service.APIKeyRoutingStateFromContext(ctx); !ok {
		return ctx, subscription, mapping
	}
	ctx, subscription = syncCurrentAPIKeyRouting(c, ctx, apiKey, subscription)
	mapping, _ = resolver.ResolveChannelMappingAndRestrict(ctx, apiKey.GroupID, model)
	return ctx, subscription, mapping
}

func restrictMixedKeyRouting(c *gin.Context, apiKey *service.APIKey, keep func(*service.Group) bool) bool {
	if apiKey == nil || !apiKey.MixedMode {
		return true
	}
	ctx := c.Request.Context()
	if !service.RestrictAPIKeyRoutingCandidates(ctx, keep) {
		return false
	}
	_, _ = syncCurrentAPIKeyRouting(c, ctx, apiKey, nil)
	return true
}
