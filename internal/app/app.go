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
}

func setup(ctx context.Context, cfg platform.Config) (dependencies, error) {
	var d dependencies
	d.close = func() {}
	if cfg.PostgresDSN != "" {
		p, err := persistence.NewPostgres(ctx, cfg.PostgresDSN)
		if err != nil {
			return d, fmt.Errorf("postgres: %w", err)
		}
		d.postgres, d.store = p, p
		d.close = p.Close
	} else if cfg.LocalFallback {
		d.store = auth.NewMemoryStore()
	} else {
		return d, errors.New("PULSE_POSTGRES_DSN is required; set PULSE_LOCAL_FALLBACK=true only for local tests")
	}
	if len(cfg.KafkaBrokers) > 0 {
		p := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)
		d.publisher = p
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
	}
	return d, nil
}

func RunHTTP(service string) error {
	cfg := platform.Load(service)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
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
		handler = worker.PersistenceHandler{Store: p}.Handle
	case "analytics-worker":
		if cfg.ClickHouseAddr == "" {
			return errors.New("PULSE_CLICKHOUSE_ADDR is required")
		}
		c, err := analytics.NewClickHouse(cfg.ClickHouseAddr, cfg.ClickHouseDatabase)
		if err != nil {
			return err
		}
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
		handler = worker.WebhookHandler{Store: p, Dispatcher: webhook.NewDispatcher(20, 5, 15*time.Second)}.Handle
	default:
		return fmt.Errorf("unknown worker service %q", service)
	}
	consumer := kafka.NewConsumerWithDLQ(cfg.KafkaBrokers, cfg.KafkaTopic, service+"s", "events.dlq", handler)
	defer consumer.Close()
	return consumer.Run(ctx)
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
