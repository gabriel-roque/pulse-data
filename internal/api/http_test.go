package api

import (
	"bytes"
	"context"
	"encoding/json"
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

func testServer(t *testing.T) (*httptest.Server, *auth.MemoryStore, *testPublisher) {
	t.Helper()
	store := auth.NewMemoryStore()
	pub := &testPublisher{}
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)
	h := NewHandler(store, pub, ratelimit.NewMemory(10, time.Minute), metrics, 1024, "local-memory-test-only", "admin", nil, webhook.NewMemoryStore(), reg)
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
func validEventBody() []byte {
	return []byte(fmt.Sprintf(`{"eventId":"evt_1","type":"payment.completed","timestamp":%q,"payload":{"amount":10}}`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)))
}
