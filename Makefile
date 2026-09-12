SHELL := /bin/sh

GO := go
COMPOSE := docker compose
LOAD_TEST ?= tests/load/smoke.js

.PHONY: up down clean quick-start stress-test capacity-test capacity-lab test race integration e2e load-smoke validate lint build fmt vet scan

up:
	$(COMPOSE) up -d --build

quick-start:
	./scripts/quick-start.sh

stress-test:
	./scripts/stress-test.sh

capacity-test:
	./scripts/capacity-test.sh

capacity-lab:
	COMPOSE_PROJECT_NAME=$${COMPOSE_PROJECT_NAME:-pulse-capacity-lab} \
	PULSE_CAPACITY_PROFILE=lab PULSE_CAPACITY_RATE_LIMIT=$${PULSE_CAPACITY_RATE_LIMIT:-0} \
	PULSE_CAPACITY_CLEANUP=$${PULSE_CAPACITY_CLEANUP:-true} \
	PULSE_CAPACITY_GATE=$${PULSE_CAPACITY_GATE:-ingress} \
	PULSE_API_URL=$${PULSE_API_URL:-http://127.0.0.1:8280} \
	PULSE_INGESTION_PORT=$${PULSE_INGESTION_PORT:-8280} PULSE_QUERY_PORT=$${PULSE_QUERY_PORT:-8281} \
	PULSE_UI_PORT=$${PULSE_UI_PORT:-3021} GRAFANA_PORT=$${GRAFANA_PORT:-3022} \
	PROMETHEUS_PORT=$${PROMETHEUS_PORT:-9200} LOKI_PORT=$${LOKI_PORT:-3120} \
	TEMPO_PORT=$${TEMPO_PORT:-3220} TEMPO_OTLP_GRPC_PORT=$${TEMPO_OTLP_GRPC_PORT:-4340} \
	TEMPO_OTLP_HTTP_PORT=$${TEMPO_OTLP_HTTP_PORT:-4341} CLICKHOUSE_HTTP_PORT=$${CLICKHOUSE_HTTP_PORT:-8140} \
	PULSE_WEBHOOK_MOCK_PORT=$${PULSE_WEBHOOK_MOCK_PORT:-8105} \
	CAPACITY_TARGET_RPS=$${CAPACITY_TARGET_RPS:-100000} CAPACITY_DIRECT=true \
	CAPACITY_WARMUP_DURATION=$${CAPACITY_WARMUP_DURATION:-1m} CAPACITY_HOLD_DURATION=$${CAPACITY_HOLD_DURATION:-5m} \
	LOAD_P95_MS=$${LOAD_P95_MS:-1000} LOAD_P99_MS=$${LOAD_P99_MS:-2000} \
	./scripts/capacity-test.sh

down:
	$(COMPOSE) down

clean:
	$(COMPOSE) down -v --remove-orphans

fmt:
	test -z "$$($(GO)fmt -l .)"

vet:
	$(GO) vet ./...

lint: fmt vet

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

build:
	$(GO) build ./cmd/...

integration: up
	PULSE_INTEGRATION_REAL=1 PULSE_ADMIN_TOKEN=$${PULSE_ADMIN_TOKEN:-change-me-admin} tests/integration/run.sh

e2e: up
	PULSE_ALLOW_PRIVATE_WEBHOOKS=true $(COMPOSE) --profile e2e up -d --build
	PULSE_E2E_REAL=1 PULSE_ADMIN_TOKEN=$${PULSE_ADMIN_TOKEN:-change-me-admin} \
	PULSE_E2E_WEBHOOK_URL=http://webhook-mock:8090/receive \
	PULSE_E2E_WEBHOOK_STATUS_URL=http://127.0.0.1:$${PULSE_WEBHOOK_MOCK_PORT:-8090}/deliveries \
	PULSE_E2E_WEBHOOK_RESET_URL=http://127.0.0.1:$${PULSE_WEBHOOK_MOCK_PORT:-8090}/reset \
	PULSE_E2E_WEBHOOK_CONFIG_URL=http://127.0.0.1:$${PULSE_WEBHOOK_MOCK_PORT:-8090}/config \
	tests/e2e/run.sh

load-smoke: up
	@test -f "$(LOAD_TEST)" || { echo "load test not found: $(LOAD_TEST)" >&2; exit 1; }
	command -v k6 >/dev/null || { echo "k6 is required for load-smoke" >&2; exit 1; }
	api_key=$${PULSE_API_KEY:-$${PULSE_LOAD_API_KEY:-}}; \
	$(COMPOSE) ps --status running >/dev/null; \
	for i in $$(seq 1 120); do curl --silent --show-error --fail "http://127.0.0.1:$${PULSE_INGESTION_PORT:-8080}/health/ready" >/dev/null && break; sleep 1; done; \
	if [ -z "$$api_key" ]; then \
		command -v curl >/dev/null && command -v jq >/dev/null || { echo "curl and jq are required to provision a smoke tenant" >&2; exit 1; }; \
		api_key=$$(curl --fail --silent --show-error -H "X-Admin-Token: $${PULSE_ADMIN_TOKEN:-change-me-admin}" -H 'Content-Type: application/json' --data '{"name":"make-load-smoke"}' "http://127.0.0.1:$${PULSE_INGESTION_PORT:-8080}/v1/tenants" | jq -er .apiKey); \
	fi; \
	PULSE_API_URL=$${PULSE_API_URL:-http://127.0.0.1:$${PULSE_INGESTION_PORT:-8080}} PULSE_API_KEY="$$api_key" k6 run "$(LOAD_TEST)"

validate:
	$(COMPOSE) config --quiet
	$(COMPOSE) -f docker-compose.yml -f docker-compose.capacity.yml config --quiet
	@if command -v helm >/dev/null; then helm lint deployments/helm/pulse; helm template pulse deployments/helm/pulse >/dev/null; else docker run --rm -v "$(CURDIR):/work:ro" alpine/helm:3.17 lint /work/deployments/helm/pulse; docker run --rm -v "$(CURDIR):/work:ro" alpine/helm:3.17 template pulse /work/deployments/helm/pulse >/dev/null; fi
	@if command -v promtool >/dev/null; then promtool check config observability/prometheus/prometheus.yml; else docker run --rm --entrypoint promtool -v "$(CURDIR):/work:ro" prom/prometheus:v3.5.0 check config /work/observability/prometheus/prometheus.yml; fi

scan:
	command -v govulncheck >/dev/null || { echo "govulncheck is required for scan" >&2; exit 1; }
	govulncheck ./...
