package platform

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Service            string
	HTTPAddr           string
	MetricsAddr        string
	PostgresDSN        string
	RedisURL           string
	KafkaBrokers       []string
	KafkaTopic         string
	ClickHouseAddr     string
	ClickHouseDatabase string
	AdminToken         string
	RateLimit          int
	RateWindow         time.Duration
	MaxPayloadBytes    int
	LocalFallback      bool
}

func Load(service string) Config {
	return Config{Service: service, HTTPAddr: env("PULSE_HTTP_ADDR", ":8080"), MetricsAddr: env("PULSE_METRICS_ADDR", ":9091"), PostgresDSN: os.Getenv("PULSE_POSTGRES_DSN"), RedisURL: os.Getenv("PULSE_REDIS_URL"), KafkaBrokers: split(os.Getenv("PULSE_KAFKA_BROKERS")), KafkaTopic: env("PULSE_KAFKA_TOPIC", "events.raw"), ClickHouseAddr: os.Getenv("PULSE_CLICKHOUSE_ADDR"), ClickHouseDatabase: env("PULSE_CLICKHOUSE_DATABASE", "default"), AdminToken: os.Getenv("PULSE_ADMIN_TOKEN"), RateLimit: intEnv("PULSE_RATE_LIMIT", 1000), RateWindow: durationEnv("PULSE_RATE_WINDOW", time.Minute), MaxPayloadBytes: intEnv("PULSE_MAX_PAYLOAD_BYTES", 1<<20), LocalFallback: os.Getenv("PULSE_LOCAL_FALLBACK") == "true"}
}
func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func intEnv(k string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(k))
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
func durationEnv(k string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(os.Getenv(k))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
func split(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}
