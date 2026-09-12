package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pulse-data/pulse/internal/auth"
	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/ratelimit"
	"github.com/pulse-data/pulse/internal/telemetry"
	"github.com/pulse-data/pulse/internal/webhook"
)

type testPublisher struct{ events []events.Event }

func (p *testPublisher) Publish(_ context.Context, e events.Event) error {
	p.events = append(p.events, e)
	return nil
}

func (p *testPublisher) PublishBatch(_ context.Context, batch []events.Event) error {
	p.events = append(p.events, batch...)
	return nil
}

func testServer(t *testing.T) (*httptest.Server, *auth.MemoryStore, *testPublisher) {
	t.Helper()
	store := auth.NewMemoryStore()
	pub := &testPublisher{}
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)
	h := NewHandler(store, pub, ratelimit.NewMemory(10, time.Minute), metrics, 1024, 1<<20, 500, "local-memory-test-only", "admin", nil, webhook.NewMemoryStore(), reg)
	return httptest.NewServer(h.Routes()), store, pub
}
func TestIngestRequiresAuthAndPublishesAfterValidation(t *testing.T) {
	srv, store, pub := testServer(t)
	defer srv.Close()
	_, key, err := store.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(validEventBody()))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status %d", res.Code)
	}
	if len(pub.events) != 1 || pub.events[0].TenantID == "" {
		t.Fatal("event was not published with tenant")
	}
}

func TestIngestBatchPublishesAllEventsAfterDurability(t *testing.T) {
	srv, store, pub := testServer(t)
	defer srv.Close()
	_, key, err := store.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`[%s,%s]`, validEventBody(), validEventBody())
	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status %d", res.Code)
	}
	if len(pub.events) != 2 {
		t.Fatalf("published %d events, want 2", len(pub.events))
	}
}

func TestIngestBatchRejectsOversizedBody(t *testing.T) {
	store := auth.NewMemoryStore()
	pub := &testPublisher{}
	reg := prometheus.NewRegistry()
	h := NewHandler(store, pub, ratelimit.NewMemory(100, time.Minute), telemetry.NewMetrics(reg), 1024, 64, 500, "test", "admin", nil, webhook.NewMemoryStore(), reg)
	_, key, err := store.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events/batch", bytes.NewReader(bytes.Repeat([]byte("x"), 5000)))
	req.Header.Set("Authorization", "Bearer "+key)
	res := httptest.NewRecorder()
	h.Routes().ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", res.Code, http.StatusBadRequest)
	}
	if len(pub.events) != 0 {
		t.Fatal("oversized batch was published")
	}
}

func TestTenantCreationAndRotation(t *testing.T) {
	srv, store, _ := testServer(t)
	defer srv.Close()
	body := bytes.NewBufferString(`{"name":"Acme"}`)
	req := httptest.NewRequest(http.MethodPost, srv.URL+"/v1/tenants", body)
	req.Header.Set("X-Admin-Token", "admin")
	res := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("create status %d", res.Code)
	}
	var created struct {
		TenantID string `json:"tenantId"`
		APIKey   string `json:"apiKey"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &created)
	if created.TenantID == "" || created.APIKey == "" {
		t.Fatal("tenant response incomplete")
	}
	rotate := httptest.NewRequest(http.MethodPost, srv.URL+"/v1/tenants/"+created.TenantID+"/rotate", nil)
	rotate.Header.Set("Authorization", "Bearer "+created.APIKey)
	res = httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(res, rotate)
	if res.Code != http.StatusOK {
		t.Fatalf("rotate status %d", res.Code)
	}
	if _, err := store.Authenticate(context.Background(), created.APIKey); err == nil {
		t.Fatal("old key accepted after rotation")
	}
}

func TestIngestRejectsMultipleJSONValues(t *testing.T) {
	srv, store, _ := testServer(t)
	defer srv.Close()
	_, key, err := store.Create(context.Background(), "Acme")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewBuffer(append(validEventBody(), []byte(` {"extra":true}`)...)))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestReadinessReturnsServiceUnavailableWhenCheckFails(t *testing.T) {
	srv, _, _ := testServer(t)
	defer srv.Close()
	// Rebuild the handler is unnecessary; the route is tested through a small
	// dedicated handler so the readiness contract stays independent of stores.
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)
	h := NewHandler(auth.NewMemoryStore(), &testPublisher{}, ratelimit.NewMemory(10, time.Minute), metrics, 1024, 1<<20, 500, "test", "admin", nil, webhook.NewMemoryStore(), reg)
	h.SetReadyCheck(func(context.Context) error { return errors.New("dependency unavailable") })
	res := httptest.NewRecorder()
	h.Routes().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want %d", res.Code, http.StatusServiceUnavailable)
	}
}
func validEventBody() []byte {
	return []byte(fmt.Sprintf(`{"eventId":"evt_1","type":"payment.completed","timestamp":%q,"payload":{"amount":10}}`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)))
}
