#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_INTEGRATION_REAL:-}" = "1" ] || test_die "set PULSE_INTEGRATION_REAL=1 to run the real Compose integration suite"
for command in curl jq docker awk tr; do
    require_cmd "$command"
done

INGESTION_URL=${PULSE_INGESTION_URL:-http://127.0.0.1:8080}
QUERY_URL=${PULSE_QUERY_URL:-http://127.0.0.1:8081}
ADMIN_TOKEN=${PULSE_ADMIN_TOKEN:-}
[ -n "$ADMIN_TOKEN" ] || test_die "PULSE_ADMIN_TOKEN is required; do not guess administrative credentials"
COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.yml}

wait_http "$INGESTION_URL/health/ready" "${PULSE_READY_TIMEOUT:-120}"
wait_http "$QUERY_URL/health/ready" "${PULSE_READY_TIMEOUT:-120}"
for service in postgres kafka redis clickhouse persistence-worker analytics-worker webhook-worker; do
    compose -f "$ROOT_DIR/$COMPOSE_FILE" ps --status running --services | awk -v wanted="$service" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }' || test_die "Compose service is not running: $service"
done

tmp=$(mktemp -d "${TMPDIR:-/tmp}/pulse-integration.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
tenant_name="external-integration-$(date +%s)-$$"

status=$(curl --silent --show-error --output "$tmp/tenant.json" --write-out '%{http_code}' \
    -H "X-Admin-Token: $ADMIN_TOKEN" -H 'Content-Type: application/json' \
    --data "$(jq -nc --arg name "$tenant_name" '{name:$name}')" \
    "$INGESTION_URL/v1/tenants")
[ "$status" = 201 ] || { printf 'tenant creation returned HTTP %s\n' "$status" >&2; cat "$tmp/tenant.json" >&2; exit 1; }
API_KEY=$(jq -er '.apiKey' "$tmp/tenant.json")
TENANT_ID=$(jq -er '.tenantId' "$tmp/tenant.json")
[ -n "$API_KEY" ] && [ -n "$TENANT_ID" ] || test_die "tenant response did not contain credentials"

event_id="evt-integration-$(date +%s)-$$"
event_type='integration.duplicate'
timestamp=$(date -u +%Y-%m-%dT%H:%M:%SZ)
event=$(jq -nc --arg id "$event_id" --arg type "$event_type" --arg timestamp "$timestamp" \
    '{eventId:$id,type:$type,timestamp:$timestamp,payload:{test:"integration",duplicate:true}}')
for attempt in 1 2; do
    status=$(curl --silent --show-error --output "$tmp/event-$attempt.json" --write-out '%{http_code}' \
        -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json' --data "$event" \
        "$INGESTION_URL/v1/events")
    [ "$status" = 202 ] || { printf 'duplicate attempt %s returned HTTP %s\n' "$attempt" "$status" >&2; cat "$tmp/event-$attempt.json" >&2; exit 1; }
done

i=0
while [ "$i" -lt "${PULSE_EVENTUAL_TIMEOUT:-90}" ]; do
    if ! compose -f "$ROOT_DIR/$COMPOSE_FILE" exec -T postgres psql -U "${POSTGRES_USER:-pulse}" -d "${POSTGRES_DB:-pulse}" \
        -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM events WHERE tenant_id = '$TENANT_ID' AND event_id = '$event_id';" >"$tmp/postgres-count"; then
        test_die "PostgreSQL query failed; persistence dependency is unavailable"
    fi
    if [ "$(tr -d '[:space:]' <"$tmp/postgres-count" 2>/dev/null || true)" = 1 ]; then break; fi
    i=$((i + 1))
    sleep 1
done
[ "$i" -lt "${PULSE_EVENTUAL_TIMEOUT:-90}" ] || test_die "PostgreSQL did not collapse the duplicate event"

i=0
while [ "$i" -lt "${PULSE_EVENTUAL_TIMEOUT:-90}" ]; do
    status=$(curl --silent --show-error --output "$tmp/summary.json" --write-out '%{http_code}' \
        -H "Authorization: Bearer $API_KEY" \
        "$QUERY_URL/v1/analytics/summary?type=$event_type")
    if [ "$status" = 200 ] && [ "$(jq -r --arg type "$event_type" '(. // []) | map(select(.type == $type) | .count) | first // 0' "$tmp/summary.json")" = 1 ]; then
        break
    fi
    i=$((i + 1))
     sleep 1
done
[ "$i" -lt "${PULSE_EVENTUAL_TIMEOUT:-90}" ] || { cat "$tmp/summary.json" >&2 2>/dev/null || true; test_die "ClickHouse analytics did not show exactly one event after duplicate ingestion"; }

printf 'integration passed: tenant=%s duplicate persistence=1 analytics=1\n' "$TENANT_ID"
