package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulse-data/pulse/internal/analytics"
	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/kafka"
	"github.com/pulse-data/pulse/internal/persistence"
	"github.com/pulse-data/pulse/internal/telemetry"
	"github.com/pulse-data/pulse/internal/webhook"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type PersistenceHandler struct {
	Store   persistence.EventStore
	Metrics *telemetry.Metrics
}

type persistenceBatchStore interface {
	SaveEvents(context.Context, []events.Event) error
}

func (h PersistenceHandler) Handle(ctx context.Context, event events.Event) error {
	ctx, span := telemetry.StartSpan(ctx, "pulse.persistence.save_event", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("pulse.event_id", event.EventID),
	))
	inserted, err := h.Store.SaveEvent(ctx, event)
	if err == nil && !inserted && h.Metrics != nil {
		// Duplicate deliveries are expected with at-least-once Kafka processing.
		// Keep the signal aggregate-only to avoid tenant cardinality in metrics.
		h.Metrics.EventsDuplicate.WithLabelValues().Inc()
	}
	telemetry.EndSpan(span, err)
	return err
}

func (h PersistenceHandler) HandleBatch(ctx context.Context, batch []events.Event) error {
	store, ok := h.Store.(persistenceBatchStore)
	if !ok {
		for _, event := range batch {
			if err := h.Handle(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
	ctx, span := telemetry.StartSpan(ctx, "pulse.persistence.save_batch", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.Int("pulse.batch_size", len(batch)),
	))
	err := store.SaveEvents(ctx, batch)
	telemetry.EndSpan(span, err)
	return err
}

type AnalyticsHandler struct{ Store analytics.Store }

type analyticsBatchStore interface {
	RecordBatch(context.Context, []events.Event) error
}

func (h AnalyticsHandler) Handle(ctx context.Context, event events.Event) error {
	ctx, span := telemetry.StartSpan(ctx, "pulse.analytics.record", trace.WithAttributes(
		attribute.String("db.system", "clickhouse"),
		attribute.String("pulse.event_id", event.EventID),
	))
	err := h.Store.Record(ctx, event)
	telemetry.EndSpan(span, err)
	return err
}

func (h AnalyticsHandler) HandleBatch(ctx context.Context, batch []events.Event) error {
	store, ok := h.Store.(analyticsBatchStore)
	if !ok {
		for _, event := range batch {
			if err := h.Handle(ctx, event); err != nil {
				return err
			}
		}
		return nil
	}
	ctx, span := telemetry.StartSpan(ctx, "pulse.analytics.record_batch", trace.WithAttributes(
		attribute.String("db.system", "clickhouse"),
		attribute.Int("pulse.batch_size", len(batch)),
	))
	err := store.RecordBatch(ctx, batch)
	telemetry.EndSpan(span, err)
	return err
}

type WebhookHandler struct {
	Store      webhook.Store
	Dispatcher interface {
		Deliver(context.Context, webhook.Delivery) error
	}
	Metrics *telemetry.Metrics
}

func (h WebhookHandler) Handle(ctx context.Context, event events.Event) error {
	listCtx, listSpan := telemetry.StartSpan(ctx, "pulse.webhook.list_subscriptions", trace.WithAttributes(
		attribute.String("pulse.tenant_id", event.TenantID),
		attribute.String("pulse.event_type", event.Type),
	))
	subs, err := h.Store.ListSubscriptions(listCtx, event.TenantID, event.Type)
	telemetry.EndSpan(listSpan, err)
	if err != nil {
		return err
	}
	return h.deliver(ctx, event, subs)
}

func (h WebhookHandler) HandleBatch(ctx context.Context, batch []events.Event) error {
	subscriptions := make(map[string][]webhook.Subscription)
	var failedEvents []events.Event
	var failures []error
	for _, event := range batch {
		key := event.TenantID + "\x00" + event.Type
		subs, ok := subscriptions[key]
		if !ok {
			listCtx, listSpan := telemetry.StartSpan(ctx, "pulse.webhook.list_subscriptions", trace.WithAttributes(
				attribute.String("pulse.tenant_id", event.TenantID),
				attribute.String("pulse.event_type", event.Type),
			))
			var err error
			subs, err = h.Store.ListSubscriptions(listCtx, event.TenantID, event.Type)
			telemetry.EndSpan(listSpan, err)
			if err != nil {
				return err
			}
			subscriptions[key] = subs
		}
		if err := h.deliver(ctx, event, subs); err != nil {
			var terminal *kafka.DeadLetterError
			if errors.As(err, &terminal) {
				failedEvents = append(failedEvents, event)
				failures = append(failures, err)
				continue
			}
			return err
		}
	}
	if len(failures) > 0 {
		return &kafka.BatchDeadLetterError{Events: failedEvents, Err: errors.Join(failures...)}
	}
	return nil
}

func (h WebhookHandler) deliver(ctx context.Context, event events.Event, subs []webhook.Subscription) error {
	var failures []error
	for _, sub := range subs {
		claimCtx, claimSpan := telemetry.StartSpan(ctx, "pulse.webhook.claim_delivery", trace.WithAttributes(
			attribute.String("pulse.subscription_id", sub.ID),
			attribute.String("pulse.event_id", event.EventID),
		))
		claimed, err := h.Store.ClaimDelivery(claimCtx, event.TenantID, event.EventID, sub.ID)
		telemetry.EndSpan(claimSpan, err)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		deliveryCtx, deliverySpan := telemetry.StartSpan(ctx, "pulse.webhook.deliver", trace.WithAttributes(
			attribute.String("server.address", sub.Endpoint),
			attribute.String("pulse.subscription_id", sub.ID),
			attribute.String("pulse.event_id", event.EventID),
		))
		err = h.Dispatcher.Deliver(deliveryCtx, webhook.Delivery{Endpoint: sub.Endpoint, Secret: sub.Secret, Event: event})
		telemetry.EndSpan(deliverySpan, err)
		if h.Metrics != nil {
			result := "success"
			if err != nil {
				result = "failure"
			}
			h.Metrics.WebhookDelivery.WithLabelValues(result).Inc()
		}
		if err != nil {
			if h.Metrics != nil {
				h.Metrics.WebhookRetry.WithLabelValues().Inc()
				h.Metrics.WebhookDLQ.WithLabelValues().Inc()
			}
			if releaseErr := h.Store.ReleaseDelivery(ctx, event.TenantID, event.EventID, sub.ID); releaseErr != nil {
				failures = append(failures, fmt.Errorf("release delivery claim for subscription %s: %w", sub.ID, releaseErr))
				continue
			}
			failures = append(failures, fmt.Errorf("subscription %s: %w", sub.ID, err))
			continue
		}
		if err := h.Store.CompleteDelivery(ctx, event.TenantID, event.EventID, sub.ID); err != nil {
			failures = append(failures, fmt.Errorf("complete delivery for subscription %s: %w", sub.ID, err))
		}
	}
	if len(failures) > 0 {
		return &kafka.DeadLetterError{Err: errors.Join(failures...)}
	}
	return nil
}
