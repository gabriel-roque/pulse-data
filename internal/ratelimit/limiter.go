package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrLimited = errors.New("rate limit exceeded")

type Limiter interface {
	Allow(ctx context.Context, tenantID string) (bool, error)
}

type Memory struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	items  map[string]counter
}
type counter struct {
	started time.Time
	count   int
}

func NewMemory(limit int, window time.Duration) *Memory {
	return &Memory{limit: limit, window: window, items: make(map[string]counter)}
}
func (m *Memory) Allow(_ context.Context, tenantID string) (bool, error) {
	if m.limit <= 0 {
		return true, nil
	}
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.items[tenantID]
	if c.started.IsZero() || now.Sub(c.started) >= m.window {
		c = counter{started: now}
	}
	if c.count >= m.limit {
		m.items[tenantID] = c
		return false, nil
	}
	c.count++
	m.items[tenantID] = c
	return true, nil
}
