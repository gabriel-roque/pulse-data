#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_E2E_REAL:-}" = "1" ] || test_die "set PULSE_E2E_REAL=1 to run the real end-to-end suite"
for command in curl jq docker awk tr; do require_cmd "$command"; done
require_env PULSE_ADMIN_TOKEN
require_env PULSE_E2E_WEBHOOK_URL
require_env PULSE_E2E_WEBHOOK_STATUS_URL

INGESTION_URL=${PULSE_INGESTION_URL:-http://127.0.0.1:8080}
QUERY_URL=${PULSE_QUERY_URL:-http://127.0.0.1:8081}
COMPOSE_FILE=${COMPOSE_FILE:-docker-compose.yml}
wait_http "$INGESTION_URL/health/ready" "${PULSE_READY_TIMEOUT:-120}"
wait_http "$QUERY_URL/health/ready" "${PULSE_READY_TIMEOUT:-120}"
curl --silent --show-error --fail "$PULSE_E2E_WEBHOOK_STATUS_URL" >/dev/null || test_die "webhook status dependency is unavailable: $PULSE_E2E_WEBHOOK_STATUS_URL"
for service in postgres kafka redis clickhouse persistence-worker analytics-worker webhook-worker; do
    compose -f "$ROOT_DIR/$COMPOSE_FILE" ps --status running --services | awk -v wanted="$service" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }' || test_die "Compose service is not running: $service"
done

tmp=$(mktemp -d "${TMPDIR:-/tmp}/pulse-e2e.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

if [ -n "${PULSE_E2E_WEBHOOK_RESET_URL:-}" ]; then
    curl --silent --show-error --fail -X POST "$PULSE_E2E_WEBHOOK_RESET_URL" >/dev/null
fi

status=$(curl --silent --show-error --output "$tmp/tenant.json" --write-out '%{http_code}' \
    -H "X-Admin-Token: $PULSE_ADMIN_TOKEN" -H 'Content-Type: application/json' \
    --data "$(jq -nc --arg name "external-e2e-$(date +%s)-$$" '{name:$name}')" \
    "$INGESTION_URL/v1/tenants")
[ "$status" = 201 ] || { cat "$tmp/tenant.json" >&2; test_die "E2E tenant creation failed with HTTP $status"; }
API_KEY=$(jq -er '.apiKey' "$tmp/tenant.json")

status=$(curl --silent --show-error --output "$tmp/webhook.json" --write-out '%{http_code}' \
    -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json' \
    --data "$(jq -nc --arg type 'e2e.webhook' --arg endpoint "$PULSE_E2E_WEBHOOK_URL" '{eventType:$type,endpoint:$endpoint}')" \
    "$INGESTION_URL/v1/webhooks")
[ "$status" = 201 ] || { cat "$tmp/webhook.json" >&2; test_die "webhook creation failed with HTTP $status; endpoint must pass SSRF policy"; }
secret=$(jq -er '.secret' "$tmp/webhook.json")

if [ -n "${PULSE_E2E_WEBHOOK_CONFIG_URL:-}" ]; then
    curl --silent --show-error --fail -X POST "$PULSE_E2E_WEBHOOK_CONFIG_URL" \
        -H 'Content-Type: application/json' \
        --data "$(jq -nc --arg secret "$secret" --argjson fail "${PULSE_E2E_WEBHOOK_FAIL_FIRST:-0}" '{secretB64:$secret,failFirst:$fail}')" >/dev/null
fi

event_id="evt-e2e-$(date +%s)-$$"
event_type='e2e.webhook'
event=$(jq -nc --arg id "$event_id" --arg type "$event_type" '{eventId:$id,type:$type,timestamp:(now|todate),payload:{source:"external-e2e"}}')
for attempt in 1 2; do
    status=$(curl --silent --show-error --output "$tmp/event-$attempt.json" --write-out '%{http_code}' \
        -H "Authorization: Bearer $API_KEY" -H 'Content-Type: application/json' --data "$event" \
        "$INGESTION_URL/v1/events")
    [ "$status" = 202 ] || { cat "$tmp/event-$attempt.json" >&2; test_die "E2E event attempt $attempt returned HTTP $status"; }
done

i=0
while [ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ]; do
    if ! compose -f "$ROOT_DIR/$COMPOSE_FILE" exec -T postgres psql -U "${POSTGRES_USER:-pulse}" -d "${POSTGRES_DB:-pulse}" \
        -v ON_ERROR_STOP=1 -Atqc "SELECT count(*) FROM events WHERE event_id = '$event_id';" >"$tmp/postgres-count"; then
        test_die "PostgreSQL query failed; persistence dependency is unavailable"
    fi
    if [ "$(tr -d '[:space:]' <"$tmp/postgres-count" 2>/dev/null || true)" = 1 ]; then break; fi
    i=$((i + 1))
    sleep 1
done
[ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ] || test_die "event was not persisted exactly once in PostgreSQL"

i=0
while [ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ]; do
    if ! status=$(curl --silent --show-error --output "$tmp/summary.json" --write-out '%{http_code}' \
        -H "Authorization: Bearer $API_KEY" "$QUERY_URL/v1/analytics/summary?type=$event_type"); then
        test_die "analytics query dependency is unavailable"
    fi
    if [ "$status" = 200 ] && [ "$(jq -r --arg type "$event_type" '(. // []) | map(select(.type == $type) | .count) | first // 0' "$tmp/summary.json")" = 1 ]; then break; fi
    i=$((i + 1))
    sleep 1
done
[ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ] || test_die "event was not represented exactly once in ClickHouse analytics"

i=0
while [ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ]; do
    curl --silent --show-error --fail "$PULSE_E2E_WEBHOOK_STATUS_URL" >"$tmp/deliveries.json"
    count=$(jq --arg id "$event_id" '[.deliveries[]? | select(.eventId == $id)] | length' "$tmp/deliveries.json" 2>/dev/null || printf '0')
    if [ "$count" -ge 1 ]; then break; fi
    i=$((i + 1))
    sleep 1
done
[ "$i" -lt "${PULSE_E2E_TIMEOUT:-120}" ] || test_die "webhook receiver did not observe event $event_id"
[ "$count" = 1 ] || test_die "webhook delivered duplicate side effects: expected 1 final delivery, got $count"
if jq -e --arg id "$event_id" '[.deliveries[] | select(.eventId == $id) | .signatureValid] | all' "$tmp/deliveries.json" >/dev/null 2>&1; then :; else
    test_die "webhook receiver reported an invalid X-Pulse-Signature"
fi

for service in postgres kafka redis clickhouse; do
    compose -f "$ROOT_DIR/$COMPOSE_FILE" ps "$service" >/dev/null || test_die "Compose dependency is unavailable: $service"
done
printf 'e2e passed: duplicate persisted once, webhook delivered once with a valid signature, dependencies present\n'
