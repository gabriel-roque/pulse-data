package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/pulse-data/pulse/internal/events"
)

type CircuitState string

const (
	Closed   CircuitState = "closed"
	Open     CircuitState = "open"
	HalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	mu            sync.Mutex
	state         CircuitState
	failures      int
	threshold     int
	openedAt      time.Time
	cooldown      time.Duration
	probeInFlight bool
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold < 1 {
		threshold = 3
	}
	return &CircuitBreaker{state: Closed, threshold: threshold, cooldown: cooldown}
}
func (b *CircuitBreaker) Allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == Open && now.Sub(b.openedAt) >= b.cooldown {
		b.state = HalfOpen
	}
	if b.state == HalfOpen {
		if b.probeInFlight {
			return false
		}
		b.probeInFlight = true
		return true
	}
	return b.state != Open
}
func (b *CircuitBreaker) Success() {
	b.mu.Lock()
	b.state, b.failures, b.probeInFlight = Closed, 0, false
	b.mu.Unlock()
}
func (b *CircuitBreaker) Failure(now time.Time) {
	b.mu.Lock()
	b.failures++
	if b.failures >= b.threshold {
		b.state, b.openedAt = Open, now
	}
	b.probeInFlight = false
	b.mu.Unlock()
}
func (b *CircuitBreaker) State() CircuitState { b.mu.Lock(); defer b.mu.Unlock(); return b.state }

type Bulkhead struct{ slots chan struct{} }

func NewBulkhead(limit int) *Bulkhead {
	if limit < 1 {
		limit = 1
	}
	return &Bulkhead{slots: make(chan struct{}, limit)}
}
func (b *Bulkhead) Acquire(ctx context.Context) error {
	select {
	case b.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (b *Bulkhead) Release() { <-b.slots }

type Delivery struct {
	Endpoint string
	Secret   []byte
	Event    events.Event
}
type Dispatcher struct {
	client        *http.Client
	maxRetries    int
	backoff       []time.Duration
	breakers      sync.Map
	bulkheadLimit int
	bulkheads     sync.Map
	now           func() time.Time
}

func NewDispatcher(concurrency, maxRetries int, timeout time.Duration) *Dispatcher {
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: safeDialer}
	if maxRetries < 0 {
		maxRetries = 0
	}
	if concurrency < 1 {
		concurrency = 1
	}
	return &Dispatcher{client: &http.Client{Transport: transport, Timeout: timeout}, maxRetries: maxRetries, backoff: []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 5 * time.Minute, 30 * time.Minute}, bulkheadLimit: concurrency, now: time.Now}
}

func (d *Dispatcher) endpointCircuitBreaker(endpoint string) *CircuitBreaker {
	if value, ok := d.breakers.Load(endpoint); ok {
		return value.(*CircuitBreaker)
	}
	candidate := NewCircuitBreaker(3, time.Minute)
	actual, _ := d.breakers.LoadOrStore(endpoint, candidate)
	return actual.(*CircuitBreaker)
}

func (d *Dispatcher) endpointBulkhead(endpoint string) *Bulkhead {
	if value, ok := d.bulkheads.Load(endpoint); ok {
		return value.(*Bulkhead)
	}
	candidate := NewBulkhead(d.bulkheadLimit)
	actual, _ := d.bulkheads.LoadOrStore(endpoint, candidate)
	return actual.(*Bulkhead)
}

func (d *Dispatcher) Deliver(ctx context.Context, delivery Delivery) error {
	if err := ValidateEndpoint(ctx, delivery.Endpoint, func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}); err != nil {
		return err
	}
	breaker := d.endpointCircuitBreaker(delivery.Endpoint)
	if !breaker.Allow(d.now()) {
		return fmt.Errorf("circuit breaker is open")
	}
	bulkhead := d.endpointBulkhead(delivery.Endpoint)
	if err := bulkhead.Acquire(ctx); err != nil {
		return err
	}
	defer bulkhead.Release()
	body, err := jsonEvent(delivery.Event)
	if err != nil {
		return err
	}
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		err = d.send(ctx, delivery, body)
		if err == nil {
			breaker.Success()
			return nil
		}
		breaker.Failure(d.now())
		if attempt == d.maxRetries {
			break
		}
		wait := d.backoff[attempt]
		wait += time.Duration(rand.Int64N(int64(wait / 5)))
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	return err
}
func (d *Dispatcher) send(ctx context.Context, delivery Delivery, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	now := d.now()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Pulse-Timestamp", strconv.FormatInt(now.Unix(), 10))
	req.Header.Set("X-Pulse-Event-Id", delivery.Event.EventID)
	req.Header.Set("X-Pulse-Signature", Signature(delivery.Secret, delivery.Event, now, body))
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
func jsonEvent(e events.Event) ([]byte, error) { return jsonMarshal(e) }

// Kept as a variable to make serialization failure testable without network access.
var jsonMarshal = func(e events.Event) ([]byte, error) { return json.Marshal(e) }

func safeDialer(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrBlockedURL
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isPrivateIP(ip) && !allowPrivateEndpoints() {
			continue
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return nil, ErrBlockedURL
}
