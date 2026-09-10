SHELL := /bin/sh

GO := go
COMPOSE := docker compose
IMAGE ?= pulse:local
LOAD_TEST ?= tests/load/smoke.js

.PHONY: up down clean test race integration e2e load-smoke validate lint build fmt vet scan

up:
	$(COMPOSE) up -d --build

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
	$(GO) test ./...

e2e: up
	@i=0; while ! curl --fail --silent http://127.0.0.1:$${PULSE_INGESTION_PORT:-8080}/health/ready >/dev/null; do i=$$((i + 1)); test $$i -lt 60; sleep 2; done
	curl --fail --silent http://127.0.0.1:$${PULSE_INGESTION_PORT:-8080}/health/live >/dev/null
	curl --fail --silent http://127.0.0.1:$${PULSE_QUERY_PORT:-8081}/health/ready >/dev/null

load-smoke: up
	@test -f "$(LOAD_TEST)" || { echo "load test not found: $(LOAD_TEST)" >&2; exit 1; }
	command -v k6 >/dev/null || { echo "k6 is required for load-smoke" >&2; exit 1; }
	k6 run "$(LOAD_TEST)"

validate:
	$(COMPOSE) config --quiet
	@if command -v helm >/dev/null; then helm lint deployments/helm/pulse; helm template pulse deployments/helm/pulse >/dev/null; else echo "helm not installed; skipped Helm validation"; fi
	@if command -v promtool >/dev/null; then promtool check config observability/prometheus/prometheus.yml; else echo "promtool not installed; skipped Prometheus validation"; fi

scan:
	command -v govulncheck >/dev/null || { echo "govulncheck is required for scan" >&2; exit 1; }
	govulncheck ./...
