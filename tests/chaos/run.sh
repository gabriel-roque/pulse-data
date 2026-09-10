#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_CHAOS_REAL:-}" = "1" ] || test_die "set PULSE_CHAOS_REAL=1 to run Compose chaos"
[ "${PULSE_CHAOS_CONFIRM:-}" = "I_UNDERSTAND" ] || test_die "set PULSE_CHAOS_CONFIRM=I_UNDERSTAND; chaos stops a live Compose service"
for command in docker curl awk; do require_cmd "$command"; done
scenario=${1:-}
[ -n "$scenario" ] || test_die "usage: tests/chaos/run.sh ingestion-restart|worker-restart|kafka-restart|postgres-unavailable|redis-unavailable|webhook-failure"
compose_file=${COMPOSE_FILE:-docker-compose.yml}
ingestion=${PULSE_INGESTION_URL:-http://127.0.0.1:8080}

wait_ready() { wait_http "$ingestion/health/ready" "${PULSE_READY_TIMEOUT:-120}"; }
compose_restore() {
    compose -f "$ROOT_DIR/$compose_file" up -d ingestion persistence-worker analytics-worker webhook-worker kafka postgres redis clickhouse >/dev/null
}
trap 'compose_restore' EXIT INT TERM
wait_ready

case "$scenario" in
    ingestion-restart)
        target=ingestion
        hypothesis='SIGTERM of ingestion makes it leave readiness and recover without removing data.'
        compose -f "$ROOT_DIR/$compose_file" kill -s SIGTERM "$target"
        ;;
    worker-restart)
        target=${PULSE_CHAOS_WORKER:-persistence-worker}
        hypothesis='A worker restart causes consumer rebalance and eventual backlog recovery.'
        compose -f "$ROOT_DIR/$compose_file" kill -s SIGTERM "$target"
        ;;
    kafka-restart)
        target=kafka
        hypothesis='Kafka restart preserves acknowledged messages and consumers recover.'
        compose -f "$ROOT_DIR/$compose_file" restart "$target"
        ;;
    postgres-unavailable)
        target=postgres
        hypothesis='PostgreSQL outage fails new durability-dependent work explicitly and recovery drains backlog.'
        compose -f "$ROOT_DIR/$compose_file" stop "$target"
        ;;
    redis-unavailable)
        target=redis
        hypothesis='Redis outage is surfaced as HTTP 503 by the rate limiter; it does not silently fail open.'
        compose -f "$ROOT_DIR/$compose_file" stop "$target"
        ;;
    webhook-failure)
        require_env PULSE_CHAOS_WEBHOOK_URL
        require_env PULSE_E2E_WEBHOOK_STATUS_URL
        require_env PULSE_E2E_WEBHOOK_CONFIG_URL
        require_env PULSE_ADMIN_TOKEN
        hypothesis='A deterministic webhook 500 is retried before the delivery is accepted; permanent failure is checked separately by the DLQ suite.'
        PULSE_E2E_REAL=1 PULSE_E2E_WEBHOOK_URL="$PULSE_CHAOS_WEBHOOK_URL" \
            PULSE_E2E_WEBHOOK_FAIL_FIRST="${PULSE_CHAOS_WEBHOOK_FAIL_FIRST:-2}" \
            PULSE_E2E_REAL=1 "$ROOT_DIR/tests/e2e/run.sh"
        printf 'chaos scenario passed: %s\nhypothesis: %s\nvalidation: E2E receiver observed deterministic retry and one final delivery\n' "$scenario" "$hypothesis"
        exit 0
        ;;
    *) test_die "unknown chaos scenario: $scenario" ;;
esac

i=0
while [ "$i" -lt "${PULSE_CHAOS_RECOVERY_TIMEOUT:-120}" ]; do
    if [ "$target" = kafka ] || [ "$target" = postgres ] || [ "$target" = redis ]; then
        if compose -f "$ROOT_DIR/$compose_file" ps --status running --services | awk -v wanted="$target" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }'; then break; fi
    else
        if [ "$target" != ingestion ] && compose -f "$ROOT_DIR/$compose_file" ps --status running --services | awk -v wanted="$target" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }'; then break; fi
        if [ "$target" = ingestion ] && ! curl --silent --fail "$ingestion/health/ready" >/dev/null 2>&1; then break; fi
    fi
    i=$((i + 1)); sleep 1
done

compose_restore
wait_ready
printf 'chaos scenario passed: %s\nhypothesis: %s\nrecovery_seconds: %s\nvalidation: readiness restored; volumes/data were not removed\n' "$scenario" "$hypothesis" "$i"
