package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pulse-data/pulse/internal/analytics"
	"github.com/pulse-data/pulse/internal/auth"
	"github.com/pulse-data/pulse/internal/events"
	"github.com/pulse-data/pulse/internal/ratelimit"
	"github.com/pulse-data/pulse/internal/telemetry"
	"github.com/pulse-data/pulse/internal/webhook"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Publisher interface {
	Publish(context.Context, events.Event) error
}
type Handler struct {
	auth        auth.Store
	publisher   Publisher
	limiter     ratelimit.Limiter
	metrics     *telemetry.Metrics
	maxPayload  int
	durability  string
	analytics   analytics.Store
	adminToken  string
	webhooks    webhook.Store
	metricsHTTP http.Handler
}

func NewHandler(store auth.Store, publisher Publisher, limiter ratelimit.Limiter, metrics *telemetry.Metrics, maxPayload int, durability, adminToken string, analytical analytics.Store, webhooks webhook.Store, registry prometheus.Gatherer) *Handler {
	return &Handler{auth: store, publisher: publisher, limiter: limiter, metrics: metrics, maxPayload: maxPayload, durability: durability, analytics: analytical, adminToken: adminToken, webhooks: webhooks, metricsHTTP: promhttp.HandlerFor(registry, promhttp.HandlerOpts{})}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.Handle("GET /metrics", h.metricsHTTP)
	mux.HandleFunc("POST /v1/events", h.ingest)
	mux.HandleFunc("POST /v1/tenants", h.createTenant)
	mux.HandleFunc("POST /v1/tenants/", h.rotateTenant)
	mux.HandleFunc("POST /v1/webhooks", h.createWebhook)
	mux.HandleFunc("GET /v1/analytics/summary", h.summary)
	return h.instrument(mux)
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.limiter != nil {
		allowed, err := h.limiter.Allow(r.Context(), tenant.ID)
		if err != nil {
			h.fail(w, http.StatusServiceUnavailable, "rate limiter unavailable")
			return
		}
		if !allowed {
			h.metrics.RateLimitRejected.WithLabelValues(tenant.ID).Inc()
			h.fail(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
	}
	reader := http.MaxBytesReader(w, r.Body, int64(h.maxPayload)+4096)
	defer r.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		h.fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	event, err := events.Decode(raw, tenant.ID, h.maxPayload)
	if err != nil {
		h.metrics.EventsFailed.WithLabelValues("validation").Inc()
		h.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	publishStarted := time.Now()
	if err := h.publisher.Publish(r.Context(), event); err != nil {
		h.metrics.EventsFailed.WithLabelValues("publish").Inc()
		h.fail(w, http.StatusServiceUnavailable, "event was not durable")
		return
	}
	h.metrics.KafkaPublishLatency.WithLabelValues("events.raw").Observe(time.Since(publishStarted).Seconds())
	h.metrics.Published.WithLabelValues("events.raw").Inc()
	h.metrics.EventsReceived.WithLabelValues(tenant.ID).Inc()
	w.Header().Set("X-Pulse-Durability", h.durability)
	writeJSON(w, http.StatusAccepted, map[string]string{"eventId": event.EventID, "status": "accepted"})
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (auth.Tenant, bool) {
	key, ok := auth.Bearer(r.Header.Get("Authorization"))
	if !ok {
		h.fail(w, http.StatusUnauthorized, "missing bearer token")
		return auth.Tenant{}, false
	}
	tenant, err := h.auth.Authenticate(r.Context(), key)
	if err != nil {
		h.fail(w, http.StatusUnauthorized, "invalid credentials")
		return auth.Tenant{}, false
	}
	return tenant, true
}
func (h *Handler) createTenant(w http.ResponseWriter, r *http.Request) {
	if h.adminToken == "" || r.Header.Get("X-Admin-Token") != h.adminToken {
		h.fail(w, http.StatusUnauthorized, "admin authentication required")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		h.fail(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	t, key, err := h.auth.Create(r.Context(), in.Name)
	if err != nil {
		h.fail(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"tenantId": t.ID, "name": t.Name, "apiKey": key})
}
func (h *Handler) rotateTenant(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 || parts[3] != "rotate" {
		h.fail(w, http.StatusNotFound, "not found")
		return
	}
	tenant, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if tenant.ID != parts[2] {
		h.fail(w, http.StatusForbidden, "tenant mismatch")
		return
	}
	key, err := h.auth.RotateKey(r.Context(), tenant.ID)
	if err != nil {
		h.fail(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tenantId": tenant.ID, "apiKey": key})
}
func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	if h.analytics == nil {
		h.fail(w, http.StatusNotImplemented, "analytics is not configured")
		return
	}
	tenant, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	if value := r.URL.Query().Get("from"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			h.fail(w, http.StatusBadRequest, "invalid from")
			return
		} else {
			from = parsed
		}
	}
	if value := r.URL.Query().Get("to"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			h.fail(w, http.StatusBadRequest, "invalid to")
			return
		} else {
			to = parsed
		}
	}
	if !from.Before(to) {
		h.fail(w, http.StatusBadRequest, "from must be before to")
		return
	}
	rows, err := h.analytics.Summary(r.Context(), tenant.ID, r.URL.Query().Get("type"), from, to)
	if err != nil {
		h.fail(w, http.StatusServiceUnavailable, "analytics unavailable")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}
func (h *Handler) createWebhook(w http.ResponseWriter, r *http.Request) {
	if h.webhooks == nil {
		h.fail(w, http.StatusNotImplemented, "webhooks are not configured")
		return
	}
	tenant, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var in struct {
		EventType string `json:"eventType"`
		Endpoint  string `json:"endpoint"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || strings.TrimSpace(in.EventType) == "" {
		h.fail(w, http.StatusBadRequest, "eventType and endpoint are required")
		return
	}
	if err := webhook.ValidateEndpoint(in.Endpoint, func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	}); err != nil {
		h.fail(w, http.StatusBadRequest, "endpoint rejected by SSRF policy")
		return
	}
	id, err := webhook.NewID()
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not create subscription")
		return
	}
	secret, err := webhook.NewSecret()
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not create subscription")
		return
	}
	sub, err := h.webhooks.CreateSubscription(r.Context(), webhook.Subscription{ID: id, TenantID: tenant.ID, EventType: in.EventType, Endpoint: in.Endpoint, Secret: secret, Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	if err != nil {
		h.fail(w, http.StatusInternalServerError, "could not create subscription")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": sub.ID, "eventType": sub.EventType, "endpoint": sub.Endpoint, "secret": base64.RawURLEncoding.EncodeToString(secret)})
}
func (h *Handler) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := telemetry.StartSpan(ctx, r.Method+" "+r.URL.Path, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("url.path", r.URL.Path),
		))
		defer span.End()
		r = r.WithContext(ctx)
		started := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		span.SetAttributes(attribute.Int("http.response.status_code", rw.status))
		if rw.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(rw.status))
		}
		h.metrics.Requests.WithLabelValues(r.Method, r.URL.Path, http.StatusText(rw.status)).Inc()
		h.metrics.RequestDuration.WithLabelValues(r.Method, r.URL.Path).Observe(time.Since(started).Seconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(b []byte) (int, error) { return w.ResponseWriter.Write(b) }
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) fail(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
