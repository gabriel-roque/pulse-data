package ratelimit

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeScripter struct {
	result int64
	err    error
	args   []interface{}
}

func (f *fakeScripter) command(ctx context.Context, args ...interface{}) *redis.Cmd {
	f.args = args
	return redis.NewCmdResult(f.result, f.err)
}

func (f *fakeScripter) Eval(ctx context.Context, _ string, _ []string, args ...interface{}) *redis.Cmd {
	return f.command(ctx, args...)
}

func (f *fakeScripter) EvalSha(ctx context.Context, _ string, _ []string, args ...interface{}) *redis.Cmd {
	return f.command(ctx, args...)
}

func (f *fakeScripter) EvalRO(ctx context.Context, _ string, _ []string, args ...interface{}) *redis.Cmd {
	return f.command(ctx, args...)
}

func (f *fakeScripter) EvalShaRO(ctx context.Context, _ string, _ []string, args ...interface{}) *redis.Cmd {
	return f.command(ctx, args...)
}

func (f *fakeScripter) ScriptExists(ctx context.Context, _ ...string) *redis.BoolSliceCmd {
	cmd := redis.NewBoolSliceCmd(ctx)
	cmd.SetErr(f.err)
	return cmd
}

func (f *fakeScripter) ScriptLoad(ctx context.Context, _ string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx)
	cmd.SetErr(f.err)
	return cmd
}

func TestRedisTokenBucketPassesCapacityRateAndTTL(t *testing.T) {
	fake := &fakeScripter{result: 1}
	l := &Redis{client: fake, limit: 10, window: 1500 * time.Millisecond, prefix: "pulse:rate:"}

	ok, err := l.Allow(context.Background(), "tenant")
	if err != nil || !ok {
		t.Fatalf("allowed=%v err=%v", ok, err)
	}
	if len(fake.args) != 4 {
		t.Fatalf("script args=%v, want capacity, refill rate, ttl, amount", fake.args)
	}
	if got, want := fake.args[0], 10; got != want {
		t.Fatalf("capacity=%v, want %v", got, want)
	}
	if got, want := fake.args[1], float64(10)/1500000; got != want {
		t.Fatalf("refill rate=%v, want %v", got, want)
	}
	if got, want := fake.args[2], int64(1500); got != want {
		t.Fatalf("ttl=%v, want %v milliseconds", got, want)
	}
	if got, want := fake.args[3], 1; got != want {
		t.Fatalf("amount=%v, want %v", got, want)
	}
}

func TestRedisFailureFailsClosed(t *testing.T) {
	fake := &fakeScripter{result: 1, err: errors.New("redis down")}
	l := &Redis{client: fake, limit: 1, window: time.Minute, prefix: "pulse:rate:"}

	ok, err := l.Allow(context.Background(), "tenant")
	if ok {
		t.Fatal("Redis failure was allowed")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error=%v, want ErrUnavailable", err)
	}
}

func TestTokenBucketLuaUsesAtomicStateAndRefill(t *testing.T) {
	for _, fragment := range []string{
		"redis.call('TIME')",
		"redis.call('HMGET', KEYS[1], 'tokens', 'timestamp')",
		"math.min(capacity, tokens + elapsed * refill_rate)",
		"redis.call('HSET', KEYS[1], 'tokens', tokens, 'timestamp', timestamp)",
		"redis.call('PEXPIRE', KEYS[1], ttl)",
	} {
		if !strings.Contains(tokenBucketLua, fragment) {
			t.Fatalf("Lua script does not contain %q", fragment)
		}
	}
	if strings.Contains(tokenBucketLua, "INCR") {
		t.Fatal("Lua script still uses fixed-window INCR")
	}
}
