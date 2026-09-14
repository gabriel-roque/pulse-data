package ratelimit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestMemoryTokenBucketAllowsBurstThenRefills(t *testing.T) {
	clock := &testClock{now: time.Unix(0, 0)}
	l := NewMemoryWithClock(3, 10*time.Second, clock.Now)

	for i := 0; i < 3; i++ {
		if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || !ok {
			t.Fatalf("burst request %d: allowed=%v err=%v", i, ok, err)
		}
	}
	if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || ok {
		t.Fatalf("request beyond burst: allowed=%v err=%v", ok, err)
	}

	clock.Advance(5 * time.Second)
	if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || !ok {
		t.Fatalf("half-window refill: allowed=%v err=%v", ok, err)
	}
	if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || ok {
		t.Fatalf("fractional token consumed too early: allowed=%v err=%v", ok, err)
	}

	clock.Advance(5 * time.Second)
	if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || !ok {
		t.Fatalf("full-window refill: allowed=%v err=%v", ok, err)
	}
}

func TestMemoryTokenBucketIsAtomicAcrossGoroutines(t *testing.T) {
	l := NewMemory(10, time.Minute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := l.AllowN(context.Background(), "tenant", 1)
			if err != nil {
				t.Error(err)
			}
			if ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("allowed %d requests, want 10", allowed)
	}
}

func TestMemoryAllowNReservesBatchAtomically(t *testing.T) {
	l := NewMemory(5, time.Minute)
	if ok, err := l.AllowN(context.Background(), "tenant", 4); err != nil || !ok {
		t.Fatalf("batch was not allowed: allowed=%v err=%v", ok, err)
	}
	if ok, err := l.AllowN(context.Background(), "tenant", 2); err != nil || ok {
		t.Fatalf("partial batch was allowed: allowed=%v err=%v", ok, err)
	}
	if ok, err := l.AllowN(context.Background(), "tenant", 1); err != nil || !ok {
		t.Fatalf("remaining token was not preserved: allowed=%v err=%v", ok, err)
	}
}

func TestMemoryInvalidWindowFailsClosed(t *testing.T) {
	l := NewMemoryWithClock(1, 0, time.Now)
	ok, err := l.AllowN(context.Background(), "tenant", 1)
	if ok {
		t.Fatal("invalid window was allowed")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error=%v, want ErrUnavailable", err)
	}
}
