package webhook

import (
	"context"
	"net"
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
	if err := ValidateEndpoint("https://example.test/hook", resolver); err == nil {
		t.Fatal("private target accepted")
	}
	if err := ValidateEndpoint("file:///etc/passwd", resolver); err == nil {
		t.Fatal("invalid scheme accepted")
	}
	if err := ValidateEndpoint("https://example.test/hook", func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("8.8.8.8")}, nil }); err != nil {
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
