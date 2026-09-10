package worker

import (
	"context"
	"fmt"

	"github.com/pulse-data/pulse/internal/analytics"
	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/kafka"
	"github.com/pulse-data/pulse/internal/persistence"
	"github.com/pulse-data/pulse/internal/webhook"
)

type PersistenceHandler struct{ Store persistence.EventStore }

func (h PersistenceHandler) Handle(ctx context.Context, event events.Event) error {
	_, err := h.Store.SaveEvent(ctx, event)
	return err
}

type AnalyticsHandler struct{ Store analytics.Store }

func (h AnalyticsHandler) Handle(ctx context.Context, event events.Event) error {
	return h.Store.Record(ctx, event)
}

type WebhookHandler struct {
	Store      webhook.Store
	Dispatcher *webhook.Dispatcher
}

func (h WebhookHandler) Handle(ctx context.Context, event events.Event) error {
	subs, err := h.Store.ListSubscriptions(ctx, event.TenantID, event.Type)
	if err != nil {
		return err
	}
	for _, sub := range subs {
		if err := h.Dispatcher.Deliver(ctx, webhook.Delivery{Endpoint: sub.Endpoint, Secret: sub.Secret, Event: event}); err != nil {
			return &kafka.DeadLetterError{Err: fmt.Errorf("subscription %s: %w", sub.ID, err)}
		}
	}
	return nil
}
