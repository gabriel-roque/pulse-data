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

func (p *MemoryPublisher) PublishBatch(_ context.Context, batch []events.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Events = append(p.Events, batch...)
	return nil
}
