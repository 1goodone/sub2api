package repository

import (
	"context"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const reservedSlotPrefix = "reserved:"

var acquireReservedAccountSlotScript = redis.NewScript(`
redis.replicate_commands()
local now = tonumber(redis.call('TIME')[1])
local ttl = tonumber(ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - ttl)
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', now - 60)
local reused = redis.call('ZREM', KEYS[1], 'reserved:' .. ARGV[4])
if reused == 0 and redis.call('ZCARD', KEYS[1]) + redis.call('ZCARD', KEYS[2]) >= tonumber(ARGV[1]) then
    return {0, now}
end
redis.call('ZADD', KEYS[1], now, ARGV[3])
redis.call('HSET', KEYS[3], ARGV[3], redis.call('HGET', KEYS[3], '@generation') or '0')
redis.call('EXPIRE', KEYS[3], ttl)
local last = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
redis.call('EXPIRE', KEYS[1], math.max(ttl, math.ceil(tonumber(last[2]) + ttl - now)))
return {1, now}
`)

var finishReservedAccountSlotScript = redis.NewScript(`
redis.replicate_commands()
local now = tonumber(redis.call('TIME')[1])
local ttl = tonumber(ARGV[3])
local seconds = tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - ttl)
local owned = redis.call('ZREM', KEYS[1], ARGV[1])
local generation = redis.call('HGET', KEYS[3], ARGV[1])
local currentGeneration = redis.call('HGET', KEYS[3], '@generation') or '0'
redis.call('HDEL', KEYS[3], ARGV[1])
local member = 'reserved:' .. ARGV[2]
if seconds > 0 and owned == 1 and generation == currentGeneration then
    -- Encode the idle deadline using the existing timestamp + slot TTL rule.
    -- One idle slot per user/session/account, even after parallel requests.
    redis.call('ZADD', KEYS[1], now + seconds - ttl, member)
    redis.call('SET', KEYS[2], ARGV[5], 'EX', seconds)
    local last = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
    redis.call('EXPIRE', KEYS[1], math.max(ttl, math.ceil(tonumber(last[2]) + ttl - now)))
end
return 1
`)

func (c *concurrencyCache) AcquireReservedAccountSlot(ctx context.Context, id int64, max int, request, session string) (bool, error) {
	result, now, err := runScriptInt64Pair(ctx, c.rdb, acquireReservedAccountSlotScript, []string{accountSlotKey(id), liveAccountSlotKey(id), "concurrency:reservation_generation:" + strconv.FormatInt(id, 10)}, max, c.slotTTLSeconds, request, session)
	if err == nil && result == 1 {
		c.touchActiveIndexAt(ctx, accountActiveIndexKey, id, now+int64(c.slotTTLSeconds))
	}
	return result == 1, err
}

func (c *concurrencyCache) FinishReservedAccountSlot(ctx context.Context, id int64, request, session string, seconds int) error {
	if err := finishReservedAccountSlotScript.Run(ctx, c.rdb, []string{accountSlotKey(id), "concurrency:reserved_session:" + session, "concurrency:reservation_generation:" + strconv.FormatInt(id, 10)}, request, session, c.slotTTLSeconds, seconds, id).Err(); err != nil {
		return err
	}
	c.refreshAccountActiveIndex(ctx, id)
	return nil
}

func (c *concurrencyCache) CancelAccountSlotReservation(ctx context.Context, id int64, session string) error {
	if session != "" {
		return c.rdb.ZRem(ctx, accountSlotKey(id), reservedSlotPrefix+session).Err()
	}
	return cancelAllAccountSlotReservationsScript.Run(ctx, c.rdb, []string{accountSlotKey(id), "concurrency:reservation_generation:" + strconv.FormatInt(id, 10)}, c.slotTTLSeconds).Err()
}

func (c *concurrencyCache) GetAccountSlotReservationsBatch(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	now, err := c.rdb.Time(ctx).Result()
	if err != nil {
		return nil, err
	}
	pipe := c.rdb.Pipeline()
	cmds := make(map[int64]*redis.ZSliceCmd, len(ids))
	for _, id := range ids {
		cmds[id] = pipe.ZRangeByScoreWithScores(ctx, accountSlotKey(id), &redis.ZRangeBy{Min: "(" + strconv.FormatInt(now.Unix()-int64(c.slotTTLSeconds), 10), Max: "+inf"})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}
	for id, cmd := range cmds {
		for _, z := range cmd.Val() {
			if member, ok := z.Member.(string); ok && strings.HasPrefix(member, reservedSlotPrefix) {
				result[id] = append(result[id], int64(z.Score)+int64(c.slotTTLSeconds))
			}
		}
	}
	return result, nil
}

var getReservedSessionAccountScript = redis.NewScript(`
local id = redis.call('GET', KEYS[1])
if not id then return 0 end
local score = redis.call('ZSCORE', 'concurrency:account:' .. id, 'reserved:' .. ARGV[1])
if not score or tonumber(score) + tonumber(ARGV[2]) <= tonumber(redis.call('TIME')[1]) then return 0 end
return tonumber(id)
`)

func (c *concurrencyCache) GetReservedSessionAccount(ctx context.Context, session string) (int64, error) {
	return getReservedSessionAccountScript.Run(ctx, c.rdb, []string{"concurrency:reserved_session:" + session}, session, c.slotTTLSeconds).Int64()
}

var cancelAllAccountSlotReservationsScript = redis.NewScript(`
redis.call('HINCRBY', KEYS[2], '@generation', 1)
redis.call('EXPIRE', KEYS[2], tonumber(ARGV[1]))
for _, member in ipairs(redis.call('ZRANGE', KEYS[1], 0, -1)) do
 if string.sub(member, 1, 9) == 'reserved:' then redis.call('ZREM', KEYS[1], member) end
end
return 1
`)
