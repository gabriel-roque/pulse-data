package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

var tokenScript = redis.NewScript(`local current = redis.call('INCR', KEYS[1]); if current == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[2]) end; if current <= tonumber(ARGV[1]) then return 1 else return 0 end`)

type Redis struct {
	client redis.UniversalClient
	limit  int
	window time.Duration
	prefix string
}

func NewRedis(client redis.UniversalClient, limit int, window time.Duration) *Redis {
	return &Redis{client: client, limit: limit, window: window, prefix: "pulse:rate:"}
}
func (r *Redis) Allow(ctx context.Context, tenantID string) (bool, error) {
	if r.limit <= 0 {
		return true, nil
	}
	n, err := tokenScript.Run(ctx, r.client, []string{r.prefix + tenantID}, r.limit, r.window.Milliseconds()).Int()
	return n == 1, err
}
