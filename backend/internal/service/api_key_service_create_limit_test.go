//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type createLimitAPIKeyRepoStub struct {
	*apiKeyRepoStub
	activeCount int64
	countErr    error
	created     []*APIKey
}

func (s *createLimitAPIKeyRepoStub) CountByUserID(context.Context, int64) (int64, error) {
	return s.activeCount, s.countErr
}

func (s *createLimitAPIKeyRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

func (s *createLimitAPIKeyRepoStub) Create(_ context.Context, key *APIKey) error {
	s.created = append(s.created, key)
	return nil
}

type createLimitCacheStub struct {
	*apiKeyCacheStub
	createCounts map[int64]int64
	incrErr      error
	windows      []time.Duration
}

func (s *createLimitCacheStub) IncrementCreateCount(_ context.Context, userID int64, window time.Duration) (int64, error) {
	s.windows = append(s.windows, window)
	if s.incrErr != nil {
		return 0, s.incrErr
	}
	s.createCounts[userID]++
	return s.createCounts[userID], nil
}

func newCreateLimitService(repo *createLimitAPIKeyRepoStub, cache *createLimitCacheStub, maxActive, maxPerHour int) *APIKeyService {
	cfg := &config.Config{APIKeyCreate: config.APIKeyCreateConfig{MaxActivePerUser: maxActive, MaxPerUserPerHour: maxPerHour}}
	return &APIKeyService{apiKeyRepo: repo, userRepo: &userRepoStub{user: &User{ID: 7}}, cache: cache, cfg: cfg}
}

func newCreateLimitStubs() (*createLimitAPIKeyRepoStub, *createLimitCacheStub) {
	return &createLimitAPIKeyRepoStub{apiKeyRepoStub: &apiKeyRepoStub{}},
		&createLimitCacheStub{apiKeyCacheStub: &apiKeyCacheStub{}, createCounts: map[int64]int64{}}
}

func TestAPIKeyServiceCreateLimits(t *testing.T) {
	t.Run("rate limits generated keys", func(t *testing.T) {
		repo, cache := newCreateLimitStubs()
		svc := newCreateLimitService(repo, cache, 0, 3)
		for i := 0; i < 3; i++ {
			_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
			require.NoError(t, err)
		}
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.ErrorIs(t, err, ErrAPIKeyCreateLimited)
		require.Len(t, repo.created, 3)
		require.Equal(t, apiKeyCreateCountWindow, cache.windows[0])
	})

	t.Run("active limit runs before rate counter", func(t *testing.T) {
		repo, cache := newCreateLimitStubs()
		repo.activeCount = 5
		svc := newCreateLimitService(repo, cache, 5, 10)
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.ErrorIs(t, err, ErrAPIKeyCountExceeded)
		require.Zero(t, cache.createCounts[7])
	})

	t.Run("zero limits disable checks", func(t *testing.T) {
		repo, cache := newCreateLimitStubs()
		repo.countErr = errors.New("must not count")
		svc := newCreateLimitService(repo, cache, 0, 0)
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.NoError(t, err)
		require.Empty(t, cache.windows)
	})

	t.Run("redis failure is fail open", func(t *testing.T) {
		repo, cache := newCreateLimitStubs()
		cache.incrErr = errors.New("redis down")
		svc := newCreateLimitService(repo, cache, 0, 1)
		_, err := svc.Create(context.Background(), 7, CreateAPIKeyRequest{Name: "k"})
		require.NoError(t, err)
	})
}
