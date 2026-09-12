package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const tokenBucketLua = `
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local amount = tonumber(ARGV[4])
local redis_time = redis.call('TIME')
local now = tonumber(redis_time[1]) * 1000000 + tonumber(redis_time[2])
local state = redis.call('HMGET', KEYS[1], 'tokens', 'timestamp')
local tokens = tonumber(state[1])
local timestamp = tonumber(state[2])

if tokens == nil or timestamp == nil then
    tokens = capacity
    timestamp = now
else
    local elapsed = now - timestamp
    if elapsed > 0 then
        tokens = math.min(capacity, tokens + elapsed * refill_rate)
        timestamp = now
    end
end

local allowed = 0
if tokens >= amount then
    tokens = tokens - amount
    allowed = 1
end
redis.call('HSET', KEYS[1], 'tokens', tokens, 'timestamp', timestamp)
redis.call('PEXPIRE', KEYS[1], ttl)
return allowed
`

var tokenScript = redis.NewScript(tokenBucketLua)

type Redis struct {
	client redis.Scripter
	limit  int
	window time.Duration
	prefix string
}

func NewRedis(client redis.UniversalClient, limit int, window time.Duration) *Redis {
	return &Redis{client: client, limit: limit, window: window, prefix: "pulse:rate:"}
}
func (r *Redis) Allow(ctx context.Context, tenantID string) (bool, error) {
	return r.AllowN(ctx, tenantID, 1)
}

func (r *Redis) AllowN(ctx context.Context, tenantID string, amount int) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	if r.limit <= 0 {
		return true, nil
	}
	if r.window <= 0 {
		return false, fmt.Errorf("%w: rate limit window must be positive", ErrUnavailable)
	}

	windowMicros := r.window.Microseconds()
	if windowMicros <= 0 {
		windowMicros = 1
	}
	ttl := int64(r.window / time.Millisecond)
	if r.window%time.Millisecond != 0 {
		ttl++
	}
	if ttl < 1 {
		ttl = 1
	}
	refillRate := float64(r.limit) / float64(windowMicros)
	n, err := tokenScript.Run(ctx, r.client, []string{r.prefix + tenantID}, r.limit, refillRate, ttl, amount).Int()
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	if n != 0 && n != 1 {
		return false, fmt.Errorf("%w: unexpected token bucket result %d", ErrUnavailable, n)
	}
	return n == 1, nil
}
