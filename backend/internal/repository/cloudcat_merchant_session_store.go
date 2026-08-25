package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const cloudcatMerchantSessionPrefix = "cloudcat:merchant:session:"

type cloudcatMerchantSessionStore struct {
	redis *redis.Client
}

func NewCloudcatMerchantSessionStore(redisClient *redis.Client) service.CloudcatMerchantSessionStore {
	return &cloudcatMerchantSessionStore{redis: redisClient}
}

func (s *cloudcatMerchantSessionStore) Load(ctx context.Context, adminID int64) (string, bool, error) {
	value, err := s.redis.Get(ctx, cloudcatMerchantSessionKey(adminID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (s *cloudcatMerchantSessionStore) Save(ctx context.Context, adminID int64, value string, ttl time.Duration) error {
	return s.redis.Set(ctx, cloudcatMerchantSessionKey(adminID), value, ttl).Err()
}

func (s *cloudcatMerchantSessionStore) Delete(ctx context.Context, adminID int64) error {
	return s.redis.Del(ctx, cloudcatMerchantSessionKey(adminID)).Err()
}

func cloudcatMerchantSessionKey(adminID int64) string {
	return cloudcatMerchantSessionPrefix + strconv.FormatInt(adminID, 10)
}
