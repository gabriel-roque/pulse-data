package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimitIsAtomicAcrossGoroutines(t *testing.T) {
	l := NewMemory(10, time.Minute)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := l.Allow(context.Background(), "tenant")
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
func TestMemoryWindowExpires(t *testing.T) {
	l := NewMemory(1, time.Nanosecond)
	if ok, _ := l.Allow(context.Background(), "t"); !ok {
		t.Fatal("first request rejected")
	}
	time.Sleep(time.Millisecond)
	if ok, _ := l.Allow(context.Background(), "t"); !ok {
		t.Fatal("new window rejected")
	}
}
