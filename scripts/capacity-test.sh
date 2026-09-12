#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"
. "$ROOT_DIR/scripts/testlib.sh"

require_cmd docker
require_cmd curl
require_cmd jq
require_cmd k6
require_cmd ps
require_cmd timeout

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

PERSISTENCE_REPLICAS=${PULSE_PERSISTENCE_REPLICAS:-1}
ANALYTICS_REPLICAS=${PULSE_ANALYTICS_REPLICAS:-1}
WEBHOOK_REPLICAS=${PULSE_WEBHOOK_REPLICAS:-1}
if [ "${PULSE_CAPACITY_PROFILE:-compact}" = "lab" ]; then
    export COMPOSE_FILE="$ROOT_DIR/docker-compose.yml:$ROOT_DIR/docker-compose.capacity.yml"
    export PULSE_KAFKA_PARTITIONS=${PULSE_KAFKA_PARTITIONS:-48}
    export PULSE_CONSUMER_BATCH_SIZE=${PULSE_CONSUMER_BATCH_SIZE:-500}
    export PULSE_CONSUMER_BATCH_TIMEOUT=${PULSE_CONSUMER_BATCH_TIMEOUT:-10ms}
    export PULSE_AUTH_CACHE_TTL=${PULSE_AUTH_CACHE_TTL:-0s}
    export PULSE_MAX_BATCH_EVENTS=${PULSE_MAX_BATCH_EVENTS:-500}
    export CAPACITY_BATCH_SIZE=${CAPACITY_BATCH_SIZE:-500}
    export PULSE_POSTGRES_MAX_CONNS=${PULSE_POSTGRES_MAX_CONNS:-8}
    PERSISTENCE_REPLICAS=${PULSE_PERSISTENCE_REPLICAS:-4}
    ANALYTICS_REPLICAS=${PULSE_ANALYTICS_REPLICAS:-4}
    WEBHOOK_REPLICAS=${PULSE_WEBHOOK_REPLICAS:-4}
fi

cleanup_capacity_lab() {
    status=$?
    trap - EXIT
    if [ "${PULSE_CAPACITY_PROFILE:-compact}" = "lab" ] && [ "${PULSE_CAPACITY_CLEANUP:-false}" = "true" ]; then
        docker compose down -v --remove-orphans >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup_capacity_lab EXIT

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
capacity_gate=${PULSE_CAPACITY_GATE:-end-to-end}
case "$capacity_gate" in
    ingress|end-to-end) ;;
    *) test_die 'PULSE_CAPACITY_GATE must be ingress or end-to-end' ;;
esac

if [ "${PULSE_CAPACITY_PROFILE:-compact}" = "lab" ] && [ "${PULSE_CAPACITY_CLEAN:-true}" = "true" ]; then
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
fi

case "$TENANTS" in
    ''|*[!0-9]*) test_die 'PULSE_CAPACITY_TENANTS must be a positive integer' ;;
esac
[ "$TENANTS" -gt 0 ] || test_die 'PULSE_CAPACITY_TENANTS must be greater than zero'

mkdir -p "$RESULT_DIR"

printf '%s\n' "target_rps=${CAPACITY_TARGET_RPS:-100000}" \
    "tenants=$TENANTS" "rate_limit=$RATE_LIMIT" "rate_window=$RATE_WINDOW" \
    "profile=${PULSE_CAPACITY_PROFILE:-compact}" "kafka_partitions=${PULSE_KAFKA_PARTITIONS:-12}" \
    "batch_size=${CAPACITY_BATCH_SIZE:-1}" \
    "capacity_gate=$capacity_gate" \
    "direct=${CAPACITY_DIRECT:-false}" "warmup_duration=${CAPACITY_WARMUP_DURATION:-1m}" \
    "hold_duration=${CAPACITY_HOLD_DURATION:-5m}" "drain_timeout=${PULSE_CAPACITY_DRAIN_TIMEOUT:-300}" \
    "persistence_replicas=$PERSISTENCE_REPLICAS" "analytics_replicas=$ANALYTICS_REPLICAS" \
    "started_at=$RUN_ID" >"$RESULT_DIR/metadata.txt"
PULSE_RATE_LIMIT="$RATE_LIMIT" PULSE_RATE_WINDOW="$RATE_WINDOW" docker compose up -d --build \
    --scale persistence-worker="$PERSISTENCE_REPLICAS" \
    --scale analytics-worker="$ANALYTICS_REPLICAS" \
    --scale webhook-worker="$WEBHOOK_REPLICAS"
PULSE_RATE_LIMIT="$RATE_LIMIT" PULSE_RATE_WINDOW="$RATE_WINDOW" docker compose config >"$RESULT_DIR/compose-config.yaml"
wait_http "${API_URL%/}/health/ready" "${PULSE_READY_TIMEOUT:-180}"

expected_partitions=${PULSE_KAFKA_PARTITIONS:-12}
actual_partitions=$(timeout "${PULSE_CAPACITY_ADMIN_TIMEOUT:-30s}" docker compose exec -T kafka /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server localhost:9092 --describe --topic events.raw 2>/dev/null |
    awk -F'PartitionCount: ' 'NF > 1 { split($2, fields, " "); print fields[1]; exit }')
[ "$actual_partitions" = "$expected_partitions" ] || test_die "events.raw has $actual_partitions partitions; expected $expected_partitions"

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
printf 'timestamp,group,topic,partition,current_offset,log_end_offset,lag\n' >"$RESULT_DIR/consumer-lag.csv"
kafka_admin_timeout=${PULSE_CAPACITY_ADMIN_TIMEOUT:-30s}
sample_resources() {
    timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    for service in ingestion query-api analytics-ui persistence-worker analytics-worker webhook-worker kafka postgres redis clickhouse prometheus loki tempo grafana; do
        for container_id in $(docker compose ps -q "$service"); do
            docker stats --no-stream --format "$timestamp,$service,{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}}" "$container_id" 2>/dev/null || true
        done
    done >>"$RESULT_DIR/resources.csv"
}
sample_consumer_lag() {
    timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    timeout "$kafka_admin_timeout" docker compose exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh \
        --bootstrap-server localhost:9092 --all-groups --describe 2>/dev/null |
        awk -v timestamp="$timestamp" 'NR > 1 && $1 != "GROUP" && NF >= 6 { print timestamp "," $1 "," $2 "," $3 "," $4 "," $5 "," $6 }' \
        >>"$RESULT_DIR/consumer-lag.csv" || true
}
consumer_lag_is_zero() {
    timeout "$kafka_admin_timeout" docker compose exec -T kafka /opt/kafka/bin/kafka-consumer-groups.sh \
        --bootstrap-server localhost:9092 --all-groups --describe 2>/dev/null |
        awk 'NR > 1 && $1 != "GROUP" && NF >= 6 { found=1; if ($6 == "-") { if ($5 != 0) bad=1; next } if ($6 !~ /^[0-9]+$/ || $6 != 0) bad=1 } END { exit (!found || bad) }'
}

k6 run \
    --env PULSE_API_URL="$API_URL" --env PULSE_API_KEYS="$api_keys" \
    --env LOAD_CHECK_RESPONSE_BODY=false \
    --env CAPACITY_DIRECT="${CAPACITY_DIRECT:-false}" \
    --env CAPACITY_TARGET_RPS="${CAPACITY_TARGET_RPS:-100000}" \
    --env CAPACITY_START_RPS="${CAPACITY_START_RPS:-1000}" \
    --env CAPACITY_BATCH_SIZE="${CAPACITY_BATCH_SIZE:-1}" \
    --env CAPACITY_STAGE_DURATION="${CAPACITY_STAGE_DURATION:-1m}" \
    --env CAPACITY_WARMUP_DURATION="${CAPACITY_WARMUP_DURATION:-1m}" \
    --env CAPACITY_HOLD_DURATION="${CAPACITY_HOLD_DURATION:-5m}" \
    --env CAPACITY_PREALLOCATED_VUS="${CAPACITY_PREALLOCATED_VUS:-2000}" \
    --env CAPACITY_MAX_VUS="${CAPACITY_MAX_VUS:-10000}" \
    --env LOAD_MAX_ERROR_RATE_PERCENT="${LOAD_MAX_ERROR_RATE_PERCENT:-1}" \
    --env LOAD_P95_MS="${LOAD_P95_MS:-250}" --env LOAD_P99_MS="${LOAD_P99_MS:-1000}" \
    --summary-export "$RESULT_DIR/summary.json" \
    --out "json=$RESULT_DIR/k6.json" "$ROOT_DIR/tests/load/capacity.js" \
    >"$RESULT_DIR/k6.log" 2>&1 &
k6_pid=$!

while :; do
    k6_state=$(ps -o stat= -p "$k6_pid" 2>/dev/null || true)
    [ -n "$k6_state" ] || break
    case "$k6_state" in
        Z*) break ;;
    esac
    sample_resources
    sample_consumer_lag
    sleep "${PULSE_CAPACITY_SAMPLE_INTERVAL:-5}"
done

set +e
wait "$k6_pid"
k6_exit=$?
set -e
sample_resources
sample_consumer_lag

drain_timeout=${PULSE_CAPACITY_DRAIN_TIMEOUT:-300}
drain_interval=${PULSE_CAPACITY_DRAIN_INTERVAL:-5}
stable_samples=${PULSE_CAPACITY_DRAIN_STABLE_SAMPLES:-3}
drain_elapsed=0
drain_stable=0
drained=0
if [ "$capacity_gate" = "end-to-end" ]; then
    while [ "$drain_elapsed" -lt "$drain_timeout" ]; do
        sample_resources
        sample_consumer_lag
        if consumer_lag_is_zero; then
            drain_stable=$((drain_stable + 1))
            if [ "$drain_stable" -ge "$stable_samples" ]; then
                drained=1
                break
            fi
        else
            drain_stable=0
        fi
        sleep "$drain_interval"
        drain_elapsed=$((drain_elapsed + drain_interval))
    done
fi
printf '%s\n' "drained=$drained" "drain_elapsed_seconds=$drain_elapsed" >>"$RESULT_DIR/metadata.txt"

reconciled=0
expected_events=$(jq -er '.metrics.accepted_events.count' "$RESULT_DIR/summary.json" 2>/dev/null || true)
if [ "$capacity_gate" = "end-to-end" ] && [ "$drained" -eq 1 ] && [ -n "$expected_events" ]; then
    postgres_user=${POSTGRES_USER:-pulse}
    postgres_db=${POSTGRES_DB:-pulse}
    clickhouse_db=${PULSE_CLICKHOUSE_DATABASE:-default}
    postgres_events=$(timeout "${PULSE_CAPACITY_DB_TIMEOUT:-60s}" docker compose exec -T postgres psql -U "$postgres_user" -d "$postgres_db" -Atqc \
        "SELECT count(*) FROM events WHERE event_type = 'load.capacity';" 2>/dev/null || true)
    clickhouse_events=$(timeout "${PULSE_CAPACITY_DB_TIMEOUT:-60s}" docker compose exec -T clickhouse clickhouse-client --query \
        "SELECT uniqExact(event_id) FROM ${clickhouse_db}.events WHERE event_type = 'load.capacity';" 2>/dev/null || true)
    printf '%s\n' "expected_events=$expected_events" "postgres_events=$postgres_events" \
        "clickhouse_events=$clickhouse_events" >>"$RESULT_DIR/metadata.txt"
    if [ "$postgres_events" = "$expected_events" ] && [ "$clickhouse_events" = "$expected_events" ]; then
        reconciled=1
    fi
fi
printf '%s\n' "reconciled=$reconciled" >>"$RESULT_DIR/metadata.txt"

printf '%s\n' "Results: $RESULT_DIR" \
    "Resource samples: $RESULT_DIR/resources.csv" \
    "Consumer lag: $RESULT_DIR/consumer-lag.csv" \
    "k6 summary: $RESULT_DIR/summary.json" \
    "k6 exit code: $k6_exit" \
    "capacity gate: $capacity_gate" \
    "drained: $drained" \
    "reconciled: $reconciled"
if [ "$capacity_gate" = "ingress" ]; then
    [ "$k6_exit" -eq 0 ]
else
    [ "$k6_exit" -eq 0 ] && [ "$drained" -eq 1 ] && [ "$reconciled" -eq 1 ]
fi
