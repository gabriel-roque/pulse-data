#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"
. "$ROOT_DIR/scripts/testlib.sh"

for command in docker curl env jq k6 ps timeout; do
    require_cmd "$command"
done

if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi

export COMPOSE_PROJECT_NAME=${PULSE_CAPACITY_PROJECT:-pulse-capacity}
export COMPOSE_FILE="$ROOT_DIR/docker-compose.yml:$ROOT_DIR/docker-compose.capacity.yml"
export PULSE_INGESTION_PORT=${PULSE_CAPACITY_PORT:-8280}
export CLICKHOUSE_HTTP_PORT=${PULSE_CAPACITY_CLICKHOUSE_PORT:-8140}
export PULSE_AUTH_CACHE_TTL=0s
export PULSE_RATE_LIMIT=0
export PULSE_KAFKA_PARTITIONS=48
export PULSE_MAX_BATCH_EVENTS=500

TARGET_EPS=100000
BATCH_SIZE=500
TENANTS=128
DURATION=5m
API_URL=${PULSE_CAPACITY_API_URL:-http://127.0.0.1:$PULSE_INGESTION_PORT}
ADMIN_TOKEN=${PULSE_ADMIN_TOKEN:-change-me-admin}
RESULT_ROOT=${PULSE_CAPACITY_RESULTS_DIR:-artifacts/capacity}
RUN_ID=$(date -u +%Y%m%dT%H%M%SZ)
RESULT_DIR="$RESULT_ROOT/$RUN_ID"
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/pulse-capacity.XXXXXX")
umask 077
printf 'header = "X-Admin-Token: %s"\n' "$ADMIN_TOKEN" >"$TMP_DIR/admin-curl.conf"
printf '{}\n' >"$TMP_DIR/k6-config.json"

cleanup() {
    status=$?
    trap - EXIT INT TERM
    rm -rf "$TMP_DIR"
    if [ "${PULSE_CAPACITY_CLEANUP:-true}" = "true" ]; then
        docker compose down -v --remove-orphans >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup EXIT INT TERM

docker compose down -v --remove-orphans >/dev/null 2>&1 || true
mkdir -p "$RESULT_DIR"

printf '%s\n' \
    "target_events_per_second=$TARGET_EPS" \
    "batch_size=$BATCH_SIZE" \
    "target_requests_per_second=$((TARGET_EPS / BATCH_SIZE))" \
    "duration=$DURATION" \
    "tenants=$TENANTS" \
    "kafka_partitions=$PULSE_KAFKA_PARTITIONS" \
    "started_at=$RUN_ID" >"$RESULT_DIR/metadata.txt"

docker compose up -d --build ingestion
wait_http "${API_URL%/}/health/ready" "${PULSE_READY_TIMEOUT:-180}"

actual_partitions=$(timeout 30s docker compose exec -T kafka /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server localhost:9092 --describe --topic events.raw 2>/dev/null |
    awk -F'PartitionCount: ' 'NF > 1 { split($2, fields, " "); print fields[1]; exit }')
[ "$actual_partitions" = "$PULSE_KAFKA_PARTITIONS" ] || test_die "events.raw has $actual_partitions partitions; expected $PULSE_KAFKA_PARTITIONS"

i=1
while [ "$i" -le "$TENANTS" ]; do
    tenant_response="$TMP_DIR/tenant-$i.json"
    curl --silent --show-error --fail \
        --config "$TMP_DIR/admin-curl.conf" \
        -H 'Content-Type: application/json' \
        --output "$tenant_response" \
        --data "$(jq -nc --arg name "capacity-$RUN_ID-$i" '{name:$name}')" \
        "${API_URL%/}/v1/tenants"
    jq -er '.apiKey' "$tenant_response" >>"$TMP_DIR/api-keys"
    i=$((i + 1))
done
printf 'timestamp,container,cpu,memory,memory_percent\n' >"$RESULT_DIR/resources.csv"
sample_resources() {
    container_ids=$(docker compose ps -q ingestion kafka postgres redis clickhouse)
    [ -z "$container_ids" ] || docker stats --no-stream \
        --format "$(date -u +%Y-%m-%dT%H:%M:%SZ),{{.Name}},{{.CPUPerc}},{{.MemUsage}},{{.MemPerc}}" \
        $container_ids >>"$RESULT_DIR/resources.csv" 2>/dev/null || true
}

env -i PATH="$PATH" HOME="${HOME:-/tmp}" k6 run \
    --config "$TMP_DIR/k6-config.json" \
    --no-thresholds=false \
    --env PULSE_API_URL="$API_URL" \
    --env PULSE_API_KEYS_FILE="$TMP_DIR/api-keys" \
    --summary-export "$RESULT_DIR/summary.json" \
    "$ROOT_DIR/tests/load/capacity.js" >"$RESULT_DIR/k6.log" 2>&1 &
k6_pid=$!

while :; do
    k6_state=$(ps -o stat= -p "$k6_pid" 2>/dev/null || true)
    [ -n "$k6_state" ] || break
    case "$k6_state" in Z*) break ;; esac
    sample_resources
    sleep "${PULSE_CAPACITY_SAMPLE_INTERVAL:-10}"
done

set +e
wait "$k6_pid"
k6_exit=$?
set -e
sample_resources

requested_events=$(jq -er '.metrics.requested_events.count' "$RESULT_DIR/summary.json" 2>/dev/null || printf '0')
accepted_events=$(jq -er '.metrics.accepted_events.count' "$RESULT_DIR/summary.json" 2>/dev/null || printf '0')
kafka_events=$(timeout 30s docker compose exec -T kafka /opt/kafka/bin/kafka-get-offsets.sh \
    --bootstrap-server localhost:9092 --topic events.raw 2>/dev/null |
    awk -F: '{ total += $3 } END { print total + 0 }')

printf '%s\n' \
    "requested_events=$requested_events" \
    "accepted_events=$accepted_events" \
    "kafka_events=$kafka_events" \
    "k6_exit_code=$k6_exit" >>"$RESULT_DIR/metadata.txt"

printf '%s\n' \
    "Results: $RESULT_DIR" \
    "Requested events: $requested_events" \
    "Accepted events: $accepted_events" \
    "Kafka events: $kafka_events" \
    "k6 exit code: $k6_exit"

[ "$k6_exit" -eq 0 ] && \
    [ "$accepted_events" -ge 30000000 ] && \
    [ "$accepted_events" = "$requested_events" ] && \
    [ "$kafka_events" = "$accepted_events" ]
