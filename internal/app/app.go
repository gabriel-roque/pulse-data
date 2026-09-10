package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/pulse-data/pulse/internal/analytics"
	"github.com/pulse-data/pulse/internal/api"
	"github.com/pulse-data/pulse/internal/auth"
	"github.com/pulse-data/pulse/internal/kafka"
	"github.com/pulse-data/pulse/internal/persistence"
	"github.com/pulse-data/pulse/internal/platform"
	"github.com/pulse-data/pulse/internal/ratelimit"
	"github.com/pulse-data/pulse/internal/telemetry"
	"github.com/pulse-data/pulse/internal/webhook"
	"github.com/pulse-data/pulse/internal/worker"
	"github.com/redis/go-redis/v9"
)

type dependencies struct {
	store     auth.Store
	publisher api.Publisher
	limiter   ratelimit.Limiter
	postgres  *persistence.Postgres
	analytics analytics.Store
	close     func()
	ready     func(context.Context) error
}

func setup(ctx context.Context, cfg platform.Config) (dependencies, error) {
	var d dependencies
	d.close = func() {}
	var checks []func(context.Context) error
	d.ready = func(ctx context.Context) error {
		for _, check := range checks {
			if err := check(ctx); err != nil {
				return err
			}
		}
		return nil
	}
	if cfg.PostgresDSN != "" {
		p, err := persistence.NewPostgres(ctx, cfg.PostgresDSN)
		if err != nil {
			return d, fmt.Errorf("postgres: %w", err)
		}
		d.postgres, d.store = p, p
		checks = append(checks, p.Ready)
		d.close = p.Close
	} else if cfg.LocalFallback {
		d.store = auth.NewMemoryStore()
	} else {
		return d, errors.New("PULSE_POSTGRES_DSN is required; set PULSE_LOCAL_FALLBACK=true only for local tests")
	}
	if len(cfg.KafkaBrokers) > 0 {
		p := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
		d.publisher = p
		checks = append(checks, p.Ready)
		oldClose := d.close
		d.close = func() { _ = p.Close(); oldClose() }
	} else if cfg.LocalFallback {
		d.publisher = &platform.MemoryPublisher{}
	} else {
		d.close()
		return d, errors.New("PULSE_KAFKA_BROKERS is required; set PULSE_LOCAL_FALLBACK=true only for local tests")
	}
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			d.close()
			return d, err
		}
		client := redis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			d.close()
			return d, fmt.Errorf("redis: %w", err)
		}
		d.limiter = ratelimit.NewRedis(client, cfg.RateLimit, cfg.RateWindow)
		checks = append(checks, func(ctx context.Context) error { return client.Ping(ctx).Err() })
		oldClose := d.close
		d.close = func() { _ = client.Close(); oldClose() }
	} else if cfg.LocalFallback {
		d.limiter = ratelimit.NewMemory(cfg.RateLimit, cfg.RateWindow)
	} else {
		d.close()
		return d, errors.New("PULSE_REDIS_URL is required; set PULSE_LOCAL_FALLBACK=true only for local tests")
	}
	if cfg.ClickHouseAddr != "" {
		c, err := analytics.NewClickHouse(cfg.ClickHouseAddr, cfg.ClickHouseDatabase)
		if err != nil {
			d.close()
			return d, err
		}
		d.analytics = c
		checks = append(checks, c.Ready)
		oldClose := d.close
		d.close = func() { _ = c.Close(); oldClose() }
	}
	return d, nil
}

func RunHTTP(service string) error {
	cfg := platform.Load(service)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing := telemetry.SetupTracing(ctx, service)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()
	d, err := setup(ctx, cfg)
	if err != nil {
		return err
	}
	defer d.close()
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)
	var webhooks webhook.Store
	if p, ok := d.store.(*persistence.Postgres); ok {
		webhooks = p
	} else if cfg.LocalFallback {
		webhooks = webhook.NewMemoryStore()
	}
	h := api.NewHandler(d.store, d.publisher, d.limiter, metrics, cfg.MaxPayloadBytes, durability(cfg), cfg.AdminToken, d.analytics, webhooks, reg)
	h.SetReadyCheck(d.ready)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: h.Routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func durability(cfg platform.Config) string {
	if len(cfg.KafkaBrokers) > 0 {
		return "kafka-ack"
	}
	return "local-memory-test-only"
}

func RunWorker(service string) error {
	cfg := platform.Load(service)
	if cfg.LocalFallback {
		return errors.New("workers require Kafka and their backing store; local fallback is intentionally ingestion-only")
	}
	if len(cfg.KafkaBrokers) == 0 {
		return errors.New("PULSE_KAFKA_BROKERS is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTracing := telemetry.SetupTracing(ctx, service)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()
	reg := prometheus.NewRegistry()
	metrics := telemetry.NewMetrics(reg)
	var handler kafka.ConsumerHandler
	switch service {
	case "persistence-worker":
		if cfg.PostgresDSN == "" {
			return errors.New("PULSE_POSTGRES_DSN is required")
		}
		p, err := persistence.NewPostgres(ctx, cfg.PostgresDSN)
		if err != nil {
			return err
		}
		defer p.Close()
		handler = worker.PersistenceHandler{Store: p, Metrics: metrics}.Handle
	case "analytics-worker":
		if cfg.ClickHouseAddr == "" {
			return errors.New("PULSE_CLICKHOUSE_ADDR is required")
		}
		c, err := analytics.NewClickHouse(cfg.ClickHouseAddr, cfg.ClickHouseDatabase)
		if err != nil {
			return err
		}
		defer c.Close()
		handler = worker.AnalyticsHandler{Store: c}.Handle
	case "webhook-worker":
		if cfg.PostgresDSN == "" {
			return errors.New("PULSE_POSTGRES_DSN is required")
		}
		p, err := persistence.NewPostgres(ctx, cfg.PostgresDSN)
		if err != nil {
			return err
		}
		defer p.Close()
		handler = worker.WebhookHandler{Store: p, Dispatcher: webhook.NewDispatcher(20, 5, 15*time.Second), Metrics: metrics}.Handle
	default:
		return fmt.Errorf("unknown worker service %q", service)
	}
	consumer := kafka.NewConsumerWithDLQ(cfg.KafkaBrokers, cfg.KafkaTopic, service+"s", "events.dlq", handler)
	consumer.SetMetrics(metrics, service)
	defer consumer.Close()
	metricsServer := &http.Server{Addr: cfg.MetricsAddr, Handler: metricsRoutes(reg), ReadHeaderTimeout: 5 * time.Second}
	metricsErr := make(chan error, 1)
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			metricsErr <- err
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = metricsServer.Shutdown(shutdownCtx)
	}()
	if err := consumer.Run(ctx); err != nil {
		return err
	}
	select {
	case err := <-metricsErr:
		return fmt.Errorf("metrics server: %w", err)
	default:
		return nil
	}
}

func metricsRoutes(reg prometheus.Gatherer) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func Run() error {
	service := os.Getenv("PULSE_SERVICE")
	if service == "" {
		service = "ingestion"
	}
	if service == "ingestion" || service == "query-api" {
		return RunHTTP(service)
	}
	return RunWorker(service)
}
