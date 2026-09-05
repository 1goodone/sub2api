package repository

import (
	"context"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const paymentVerificationCooldownKeyPrefix = "payment:verification:cooldown:"

type paymentVerificationCooldown struct {
	rdb *redis.Client
}

func NewPaymentVerificationCooldown(rdb *redis.Client) service.PaymentVerificationCooldown {
	return &paymentVerificationCooldown{rdb: rdb}
}

func (c *paymentVerificationCooldown) TryAcquire(ctx context.Context, orderID int64, ttl time.Duration) (bool, error) {
	if c == nil || c.rdb == nil || orderID <= 0 || ttl <= 0 {
		return true, nil
	}
	key := paymentVerificationCooldownKeyPrefix + strconv.FormatInt(orderID, 10)
	return c.rdb.SetNX(ctx, key, "1", ttl).Result()
}
