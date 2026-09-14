SHELL := /bin/sh

GO := go
COMPOSE := docker compose

.PHONY: up down clean quick-start capacity-test test race integration e2e validate lint build fmt vet scan

up:
	$(COMPOSE) up -d --build

quick-start:
	./scripts/quick-start.sh

capacity-test:
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

validate:
	$(COMPOSE) config --quiet
	$(COMPOSE) -f docker-compose.yml -f docker-compose.capacity.yml config --quiet
	@if command -v helm >/dev/null; then helm lint deployments/helm/pulse; helm template pulse deployments/helm/pulse >/dev/null; else docker run --rm -v "$(CURDIR):/work:ro" alpine/helm:3.17 lint /work/deployments/helm/pulse; docker run --rm -v "$(CURDIR):/work:ro" alpine/helm:3.17 template pulse /work/deployments/helm/pulse >/dev/null; fi
	@if command -v promtool >/dev/null; then promtool check config observability/prometheus/prometheus.yml; else docker run --rm --entrypoint promtool -v "$(CURDIR):/work:ro" prom/prometheus:v3.5.0 check config /work/observability/prometheus/prometheus.yml; fi

scan:
	command -v govulncheck >/dev/null || { echo "govulncheck is required for scan" >&2; exit 1; }
	govulncheck ./...
