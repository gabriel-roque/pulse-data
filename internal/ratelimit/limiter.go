package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("rate limiter unavailable")

type Limiter interface {
	AllowN(ctx context.Context, tenantID string, amount int) (bool, error)
}

type Memory struct {
	mu    sync.Mutex
	limit int
	rate  float64
	now   func() time.Time
	items map[string]bucket
}

type bucket struct {
	tokens    float64
	timestamp time.Time
}

func NewMemory(limit int, window time.Duration) *Memory {
	return NewMemoryWithClock(limit, window, time.Now)
}

func NewMemoryWithClock(limit int, window time.Duration, now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	rate := 0.0
	if window > 0 {
		rate = float64(limit) / window.Seconds()
	}
	return &Memory{limit: limit, rate: rate, now: now, items: make(map[string]bucket)}
}

func (m *Memory) AllowN(_ context.Context, tenantID string, amount int) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	if m.limit <= 0 {
		return true, nil
	}
	if m.rate <= 0 {
		return false, fmt.Errorf("%w: rate limit window must be positive", ErrUnavailable)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	b, ok := m.items[tenantID]
	if !ok {
		b = bucket{tokens: float64(m.limit), timestamp: now}
	} else if now.After(b.timestamp) {
		b.tokens = min(float64(m.limit), b.tokens+now.Sub(b.timestamp).Seconds()*m.rate)
		b.timestamp = now
	}
	if b.tokens < float64(amount) {
		m.items[tenantID] = b
		return false, nil
	}
	b.tokens -= float64(amount)
	m.items[tenantID] = b
	return true, nil
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
