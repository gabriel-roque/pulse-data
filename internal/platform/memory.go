package platform

import (
	"context"
	"sync"

	"github.com/pulse-data/pulse/internal/events"
)

type MemoryPublisher struct {
	mu     sync.Mutex
	Events []events.Event
}

func (p *MemoryPublisher) Publish(_ context.Context, e events.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = append(p.Events, e)
	return nil
}
