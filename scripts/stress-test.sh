#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"
. "$ROOT_DIR/scripts/testlib.sh"

require_cmd curl
require_cmd jq
require_cmd k6

if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi

API_URL=${PULSE_API_URL:-http://127.0.0.1:${PULSE_INGESTION_PORT:-8080}}
wait_http "${API_URL%/}/health/ready" "${PULSE_READY_TIMEOUT:-120}"

api_keys=${PULSE_API_KEYS:-${PULSE_API_KEY:-}}
tmp=$(mktemp -d "${TMPDIR:-/tmp}/pulse-stress.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

if [ -z "$api_keys" ]; then
    admin_token=${PULSE_ADMIN_TOKEN:-}
    [ -n "$admin_token" ] || test_die 'set PULSE_API_KEY/PULSE_API_KEYS or PULSE_ADMIN_TOKEN to provision stress tenants'
    tenant_count=${PULSE_STRESS_TENANTS:-16}
    case "$tenant_count" in
        ''|*[!0-9]*) test_die 'PULSE_STRESS_TENANTS must be a positive integer' ;;
    esac
    [ "$tenant_count" -gt 0 ] || test_die 'PULSE_STRESS_TENANTS must be greater than zero'

    i=1
    while [ "$i" -le "$tenant_count" ]; do
        curl --silent --show-error --fail \
            -H "X-Admin-Token: $admin_token" \
            -H 'Content-Type: application/json' \
            --data "$(jq -nc --arg name "stress-$i-$(date +%s)" '{name:$name}')" \
            "${API_URL%/}/v1/tenants" | jq -er '.apiKey' >>"$tmp/api-keys"
        i=$((i + 1))
    done
    api_keys=$(tr '\n' ',' <"$tmp/api-keys" | tr -d '\n' | sed 's/,$//')
fi

printf '%s\n' 'Starting k6 stress profile. The profile ramps to 100,000 requests/s and may fail at saturation.'
PULSE_API_URL="$API_URL" PULSE_API_KEYS="$api_keys" exec "$ROOT_DIR/scripts/run-k6.sh" stress
