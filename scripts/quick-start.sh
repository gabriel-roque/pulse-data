#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

command -v docker >/dev/null 2>&1 || { printf '%s\n' 'Docker is required.' >&2; exit 1; }
docker compose version >/dev/null 2>&1 || { printf '%s\n' 'Docker Compose v2 is required.' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { printf '%s\n' 'curl is required.' >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { printf '%s\n' 'jq is required.' >&2; exit 1; }

if [ ! -f .env ]; then
    cp .env.example .env
    printf '%s\n' 'Created .env from .env.example. Local-only defaults are in use.'
fi

saved_compose_project=${COMPOSE_PROJECT_NAME-}
saved_admin_token=${PULSE_ADMIN_TOKEN-}
saved_ingestion_port=${PULSE_INGESTION_PORT-}
saved_query_port=${PULSE_QUERY_PORT-}
saved_ui_port=${PULSE_UI_PORT-}
saved_webhook_port=${PULSE_WEBHOOK_MOCK_PORT-}
saved_clickhouse_port=${CLICKHOUSE_HTTP_PORT-}
saved_prometheus_port=${PROMETHEUS_PORT-}
saved_loki_port=${LOKI_PORT-}
saved_tempo_port=${TEMPO_PORT-}
saved_tempo_grpc_port=${TEMPO_OTLP_GRPC_PORT-}
saved_tempo_http_port=${TEMPO_OTLP_HTTP_PORT-}
saved_grafana_port=${GRAFANA_PORT-}

# The local E2E receiver is intentionally enabled only for this isolated profile.
set -a
. ./.env
set +a

[ -z "$saved_compose_project" ] || export COMPOSE_PROJECT_NAME="$saved_compose_project"
[ -z "$saved_admin_token" ] || export PULSE_ADMIN_TOKEN="$saved_admin_token"
[ -z "$saved_ingestion_port" ] || export PULSE_INGESTION_PORT="$saved_ingestion_port"
[ -z "$saved_query_port" ] || export PULSE_QUERY_PORT="$saved_query_port"
[ -z "$saved_ui_port" ] || export PULSE_UI_PORT="$saved_ui_port"
[ -z "$saved_webhook_port" ] || export PULSE_WEBHOOK_MOCK_PORT="$saved_webhook_port"
[ -z "$saved_clickhouse_port" ] || export CLICKHOUSE_HTTP_PORT="$saved_clickhouse_port"
[ -z "$saved_prometheus_port" ] || export PROMETHEUS_PORT="$saved_prometheus_port"
[ -z "$saved_loki_port" ] || export LOKI_PORT="$saved_loki_port"
[ -z "$saved_tempo_port" ] || export TEMPO_PORT="$saved_tempo_port"
[ -z "$saved_tempo_grpc_port" ] || export TEMPO_OTLP_GRPC_PORT="$saved_tempo_grpc_port"
[ -z "$saved_tempo_http_port" ] || export TEMPO_OTLP_HTTP_PORT="$saved_tempo_http_port"
[ -z "$saved_grafana_port" ] || export GRAFANA_PORT="$saved_grafana_port"

PULSE_ALLOW_PRIVATE_WEBHOOKS=true docker compose --profile e2e up -d --build

. "$ROOT_DIR/scripts/testlib.sh"
wait_http "${PULSE_INGESTION_URL:-http://127.0.0.1:${PULSE_INGESTION_PORT:-8080}}/health/ready" "${PULSE_READY_TIMEOUT:-180}"
wait_http "${PULSE_QUERY_URL:-http://127.0.0.1:${PULSE_QUERY_PORT:-8081}}/health/ready" "${PULSE_READY_TIMEOUT:-180}"
wait_http "http://127.0.0.1:${PULSE_UI_PORT:-3001}/" "${PULSE_READY_TIMEOUT:-180}"
wait_http "http://127.0.0.1:${PULSE_WEBHOOK_MOCK_PORT:-8090}/health" "${PULSE_READY_TIMEOUT:-180}"

for service in ingestion query-api analytics-ui persistence-worker analytics-worker webhook-worker postgres kafka redis clickhouse prometheus loki tempo grafana webhook-mock; do
    compose_service_running "$service" || test_die "Compose service is not running: $service"
done

PULSE_E2E_REAL=1 \
PULSE_ADMIN_TOKEN="${PULSE_ADMIN_TOKEN:-change-me-admin}" \
PULSE_E2E_WEBHOOK_URL='http://webhook-mock:8090/receive' \
PULSE_E2E_WEBHOOK_STATUS_URL="http://127.0.0.1:${PULSE_WEBHOOK_MOCK_PORT:-8090}/deliveries" \
PULSE_E2E_WEBHOOK_RESET_URL="http://127.0.0.1:${PULSE_WEBHOOK_MOCK_PORT:-8090}/reset" \
PULSE_E2E_WEBHOOK_CONFIG_URL="http://127.0.0.1:${PULSE_WEBHOOK_MOCK_PORT:-8090}/config" \
tests/e2e/run.sh

printf '%s\n' '' 'Pulse is up and the E2E validation passed.'
printf '%s\n' 'Ingestion: http://127.0.0.1:'"${PULSE_INGESTION_PORT:-8080}" \
    'Query API: http://127.0.0.1:'"${PULSE_QUERY_PORT:-8081}" \
    'Analytics UI: http://127.0.0.1:'"${PULSE_UI_PORT:-3001}" \
    'Grafana: http://127.0.0.1:'"${GRAFANA_PORT:-3000}"
