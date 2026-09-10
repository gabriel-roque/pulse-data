package worker

import (
	"context"
	"testing"
	"time"

	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/webhook"
)

type recordingDispatcher struct {
	deliveries int
	err        error
}

func (d *recordingDispatcher) Deliver(context.Context, webhook.Delivery) error {
	d.deliveries++
	return d.err
}

func TestWebhookHandlerDeduplicatesDeliveryEffects(t *testing.T) {
	store := webhook.NewMemoryStore()
	_, err := store.CreateSubscription(context.Background(), webhook.Subscription{
		ID:        "sub_1",
		TenantID:  "tenant_1",
		EventType: "invoice.created",
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{}
	handler := WebhookHandler{Store: store, Dispatcher: dispatcher}
	event := events.Event{
		TenantID:  "tenant_1",
		EventID:   "event_1",
		Type:      "invoice.created",
		Timestamp: time.Now(),
		Payload:   []byte(`{"amount":100}`),
	}

	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if dispatcher.deliveries != 1 {
		t.Fatalf("got %d deliveries, want 1", dispatcher.deliveries)
	}
}

func TestWebhookHandlerReleasesFailedDelivery(t *testing.T) {
	store := webhook.NewMemoryStore()
	_, err := store.CreateSubscription(context.Background(), webhook.Subscription{
		ID:        "sub_1",
		TenantID:  "tenant_1",
		EventType: "invoice.created",
		Enabled:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &recordingDispatcher{err: context.DeadlineExceeded}
	handler := WebhookHandler{Store: store, Dispatcher: dispatcher}
	event := events.Event{TenantID: "tenant_1", EventID: "event_1", Type: "invoice.created"}

	if err := handler.Handle(context.Background(), event); err == nil {
		t.Fatal("failed delivery unexpectedly succeeded")
	}
	dispatcher.err = nil
	if err := handler.Handle(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if dispatcher.deliveries != 2 {
		t.Fatalf("got %d deliveries, want retry after release", dispatcher.deliveries)
	}
}
