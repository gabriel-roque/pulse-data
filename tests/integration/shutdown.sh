#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_INTEGRATION_REAL:-}" = "1" ] || test_die "set PULSE_INTEGRATION_REAL=1 to run shutdown validation"
[ "${PULSE_SHUTDOWN_CONFIRM:-}" = "I_UNDERSTAND" ] || test_die "set PULSE_SHUTDOWN_CONFIRM=I_UNDERSTAND; this test sends SIGTERM to a Compose service"
for command in docker curl awk; do require_cmd "$command"; done
service=${PULSE_SHUTDOWN_SERVICE:-ingestion}
compose_file=${COMPOSE_FILE:-docker-compose.yml}
compose -f "$ROOT_DIR/$compose_file" ps "$service" >/dev/null
wait_http "${PULSE_INGESTION_URL:-http://127.0.0.1:8080}/health/ready" "${PULSE_READY_TIMEOUT:-120}"

compose -f "$ROOT_DIR/$compose_file" kill -s SIGTERM "$service"
i=0
while compose -f "$ROOT_DIR/$compose_file" ps --status running --services | awk -v wanted="$service" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }'; do
    i=$((i + 1))
    [ "$i" -lt 30 ] || test_die "$service did not stop after SIGTERM"
    sleep 1
done

compose -f "$ROOT_DIR/$compose_file" up -d "$service"
wait_http "${PULSE_INGESTION_URL:-http://127.0.0.1:8080}/health/ready" "${PULSE_READY_TIMEOUT:-120}"
printf 'shutdown passed: %s stopped with SIGTERM and recovered; volumes were not removed\n' "$service"
