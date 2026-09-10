package worker

import (
	"context"
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

type PersistenceHandler struct{ Store persistence.EventStore }

func (h PersistenceHandler) Handle(ctx context.Context, event events.Event) error {
	ctx, span := telemetry.StartSpan(ctx, "pulse.persistence.save_event", trace.WithAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("pulse.event_id", event.EventID),
	))
	_, err := h.Store.SaveEvent(ctx, event)
	telemetry.EndSpan(span, err)
	return err
}

type AnalyticsHandler struct{ Store analytics.Store }

func (h AnalyticsHandler) Handle(ctx context.Context, event events.Event) error {
	ctx, span := telemetry.StartSpan(ctx, "pulse.analytics.record", trace.WithAttributes(
		attribute.String("db.system", "clickhouse"),
		attribute.String("pulse.event_id", event.EventID),
	))
	err := h.Store.Record(ctx, event)
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
				h.Metrics.WebhookRetry.WithLabelValues(event.TenantID).Inc()
				h.Metrics.WebhookDLQ.WithLabelValues(event.TenantID).Inc()
			}
			if releaseErr := h.Store.ReleaseDelivery(ctx, event.TenantID, event.EventID, sub.ID); releaseErr != nil {
				return fmt.Errorf("release delivery claim for subscription %s: %w", sub.ID, releaseErr)
			}
			return &kafka.DeadLetterError{Err: fmt.Errorf("subscription %s: %w", sub.ID, err)}
		}
		if err := h.Store.CompleteDelivery(ctx, event.TenantID, event.EventID, sub.ID); err != nil {
			return fmt.Errorf("complete delivery for subscription %s: %w", sub.ID, err)
		}
	}
	return nil
}
