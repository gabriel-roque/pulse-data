#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"
. "$ROOT_DIR/scripts/testlib.sh"

require_cmd docker
require_cmd curl
require_cmd jq
require_cmd k6

saved_compose_project=${COMPOSE_PROJECT_NAME-}
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

if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi

[ -z "$saved_compose_project" ] || export COMPOSE_PROJECT_NAME="$saved_compose_project"
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

API_URL=${PULSE_API_URL:-http://127.0.0.1:${PULSE_INGESTION_PORT:-8080}}
ADMIN_TOKEN=${PULSE_ADMIN_TOKEN:-}
TENANTS=${PULSE_CAPACITY_TENANTS:-128}
RATE_LIMIT=${PULSE_CAPACITY_RATE_LIMIT:-100000}
RATE_WINDOW=${PULSE_CAPACITY_RATE_WINDOW:-1s}
RESULT_ROOT=${PULSE_CAPACITY_RESULTS_DIR:-artifacts/capacity}
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
RESULT_DIR="$RESULT_ROOT/$RUN_ID"

case "$TENANTS" in
    ''|*[!0-9]*) test_die 'PULSE_CAPACITY_TENANTS must be a positive integer' ;;
esac
[ "$TENANTS" -gt 0 ] || test_die 'PULSE_CAPACITY_TENANTS must be greater than zero'

mkdir -p "$RESULT_DIR"

printf '%s\n' "target_rps=${CAPACITY_TARGET_RPS:-100000}" \
    "tenants=$TENANTS" "rate_limit=$RATE_LIMIT" "rate_window=$RATE_WINDOW" \
    "started_at=$RUN_ID" >"$RESULT_DIR/metadata.txt"
PULSE_RATE_LIMIT="$RATE_LIMIT" PULSE_RATE_WINDOW="$RATE_WINDOW" docker compose up -d --build
PULSE_RATE_LIMIT="$RATE_LIMIT" PULSE_RATE_WINDOW="$RATE_WINDOW" docker compose config >"$RESULT_DIR/compose-config.yaml"
wait_http "${API_URL%/}/health/ready" "${PULSE_READY_TIMEOUT:-180}"

api_keys=${PULSE_API_KEYS:-${PULSE_API_KEY:-}}
if [ -z "$api_keys" ]; then
    [ -n "$ADMIN_TOKEN" ] || test_die 'set PULSE_ADMIN_TOKEN or PULSE_API_KEY(S) before capacity test'
    i=1
    while [ "$i" -le "$TENANTS" ]; do
        curl --silent --show-error --fail \
            -H "X-Admin-Token: $ADMIN_TOKEN" \
            -H 'Content-Type: application/json' \
            --data "$(jq -nc --arg name "capacity-$RUN_ID-$i" '{name:$name}')" \
            "${API_URL%/}/v1/tenants" | jq -er '.apiKey' >>"$RESULT_DIR/api-keys"
        i=$((i + 1))
    done
    api_keys=$(tr '\n' ',' <"$RESULT_DIR/api-keys" | tr -d '\n' | sed 's/,$//')
fi

printf 'timestamp,service,cpu,memory,memory_percent\n' >"$RESULT_DIR/resources.csv"
sample_resources() {
    timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    for service in ingestion query-api analytics-ui persistence-worker analytics-worker webhook-worker kafka postgres redis clickhouse prometheus loki tempo grafana; do
        container_id=$(docker compose ps -q "$service")
        if [ -n "$container_id" ]; then
            docker stats --no-stream --format "$timestamp,$service,{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}}" "$container_id" 2>/dev/null || true
        fi
    done >>"$RESULT_DIR/resources.csv"
}

PULSE_API_URL="$API_URL" PULSE_API_KEYS="$api_keys" LOAD_CHECK_RESPONSE_BODY=false \
    k6 run --summary-export "$RESULT_DIR/summary.json" \
    --out "json=$RESULT_DIR/k6.json" "$ROOT_DIR/tests/load/capacity.js" \
    >"$RESULT_DIR/k6.log" 2>&1 &
k6_pid=$!

while kill -0 "$k6_pid" 2>/dev/null; do
    sample_resources
    sleep "${PULSE_CAPACITY_SAMPLE_INTERVAL:-5}"
done

set +e
wait "$k6_pid"
k6_exit=$?
set -e
sample_resources

printf '%s\n' "Results: $RESULT_DIR" \
    "Resource samples: $RESULT_DIR/resources.csv" \
    "k6 summary: $RESULT_DIR/summary.json" \
    "k6 exit code: $k6_exit"
exit "$k6_exit"
