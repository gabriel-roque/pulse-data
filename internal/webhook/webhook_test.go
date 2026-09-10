package webhook

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pulse-data/pulse/internal/events"
)

func TestSignatureAndReplayProtection(t *testing.T) {
	now := time.Now()
	body := []byte(`{"eventId":"e"}`)
	sig := Signature([]byte("secret"), events.Event{}, now, body)
	if !VerifySignature([]byte("secret"), sig, now, body, time.Minute, now) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature([]byte("secret"), sig, now.Add(-2*time.Minute), body, time.Minute, now) {
		t.Fatal("replayed signature accepted")
	}
	if VerifySignature([]byte("other"), sig, now, body, time.Minute, now) {
		t.Fatal("wrong secret accepted")
	}
}
func TestSSRFPolicy(t *testing.T) {
	resolver := func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.0.0.1")}, nil }
	if err := ValidateEndpoint(context.Background(), "https://example.test/hook", resolver); err == nil {
		t.Fatal("private target accepted")
	}
	if err := ValidateEndpoint(context.Background(), "file:///etc/passwd", resolver); err == nil {
		t.Fatal("invalid scheme accepted")
	}
	if err := ValidateEndpoint(context.Background(), "https://example.test/hook", func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }); err != nil {
		t.Fatal(err)
	}
}
func TestCircuitBreakerTransitions(t *testing.T) {
	now := time.Now()
	b := NewCircuitBreaker(2, time.Second)
	b.Failure(now)
	if !b.Allow(now) {
		t.Fatal("closed breaker blocked")
	}
	b.Failure(now)
	if b.Allow(now) {
		t.Fatal("open breaker allowed")
	}
	if !b.Allow(now.Add(time.Second)) {
		t.Fatal("half-open breaker blocked")
	}
	b.Success()
	if b.State() != Closed {
		t.Fatal("breaker did not close")
	}
}
func TestBulkheadCancellation(t *testing.T) {
	b := NewBulkhead(1)
	if err := b.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := b.Acquire(ctx); err == nil {
		t.Fatal("second acquire unexpectedly succeeded")
	}
	b.Release()
}

func TestDispatcherCircuitBreakerIsPerEndpoint(t *testing.T) {
	t.Setenv("PULSE_ALLOW_PRIVATE_WEBHOOKS", "true")

	const (
		endpointA = "http://127.0.0.1/a"
		endpointB = "http://127.0.0.1/b"
	)
	var attemptsA, attemptsB int
	dispatcher := NewDispatcher(1, 2, time.Second)
	dispatcher.backoff = []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond}
	dispatcher.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if req.URL.Path == "/a" {
			attemptsA++
			status = http.StatusBadGateway
		} else {
			attemptsB++
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	delivery := Delivery{Endpoint: endpointA, Secret: []byte("secret"), Event: events.Event{EventID: "event"}}

	if err := dispatcher.Deliver(context.Background(), delivery); err == nil {
		t.Fatal("failing endpoint unexpectedly succeeded")
	}
	if attemptsA != 3 {
		t.Fatalf("failing endpoint attempts = %d, want 3", attemptsA)
	}
	if err := dispatcher.Deliver(context.Background(), delivery); err == nil {
		t.Fatal("open circuit unexpectedly allowed endpoint A")
	}
	if attemptsA != 3 {
		t.Fatalf("open circuit sent endpoint A request, got %d attempts", attemptsA)
	}

	delivery.Endpoint = endpointB
	if err := dispatcher.Deliver(context.Background(), delivery); err != nil {
		t.Fatalf("healthy endpoint blocked by endpoint A circuit: %v", err)
	}
	if attemptsB != 1 {
		t.Fatalf("healthy endpoint attempts = %d, want 1", attemptsB)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestMemoryStoreClaimDeliveryIsAtomic(t *testing.T) {
	store := NewMemoryStore()
	const attempts = 32
	claims := make(chan bool, attempts)
	for range attempts {
		go func() {
			claimed, err := store.ClaimDelivery(context.Background(), "tenant", "event", "subscription")
			if err != nil {
				t.Errorf("claim delivery: %v", err)
				return
			}
			claims <- claimed
		}()
	}

	claimedCount := 0
	for range attempts {
		if <-claims {
			claimedCount++
		}
	}
	if claimedCount != 1 {
		t.Fatalf("got %d claims, want 1", claimedCount)
	}
}

func TestMemoryStoreDeliveryLifecycle(t *testing.T) {
	store := NewMemoryStore()
	now := time.Now()
	store.now = func() time.Time { return now }

	claimed, err := store.ClaimDelivery(context.Background(), "tenant", "event", "subscription")
	if err != nil || !claimed {
		t.Fatalf("initial claim = %v, %v; want true, nil", claimed, err)
	}
	claimed, err = store.ClaimDelivery(context.Background(), "tenant", "event", "subscription")
	if err != nil || claimed {
		t.Fatalf("active lease claim = %v, %v; want false, nil", claimed, err)
	}
	if err := store.ReleaseDelivery(context.Background(), "tenant", "event", "subscription"); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimDelivery(context.Background(), "tenant", "event", "subscription")
	if err != nil || !claimed {
		t.Fatalf("released claim = %v, %v; want true, nil", claimed, err)
	}
	if err := store.CompleteDelivery(context.Background(), "tenant", "event", "subscription"); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimDelivery(context.Background(), "tenant", "event", "subscription")
	if err != nil || claimed {
		t.Fatalf("completed claim = %v, %v; want false, nil", claimed, err)
	}

	claimed, err = store.ClaimDelivery(context.Background(), "tenant", "expired", "subscription")
	if err != nil || !claimed {
		t.Fatalf("expiry setup claim = %v, %v; want true, nil", claimed, err)
	}
	now = now.Add(deliveryLease + time.Second)
	claimed, err = store.ClaimDelivery(context.Background(), "tenant", "expired", "subscription")
	if err != nil || !claimed {
		t.Fatalf("expired lease claim = %v, %v; want true, nil", claimed, err)
	}
}
