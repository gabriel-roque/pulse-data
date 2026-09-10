#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_INTEGRATION_REAL:-}" = "1" ] || test_die "set PULSE_INTEGRATION_REAL=1 to run distributed rate-limit validation"
for command in curl jq sed; do require_cmd "$command"; done
require_env PULSE_ADMIN_TOKEN
require_env PULSE_RATE_TEST_URLS
limit=${PULSE_RATE_TEST_LIMIT:-}
[ -n "$limit" ] || test_die "PULSE_RATE_TEST_LIMIT is required and must match the configured distributed limit"
case "$limit" in *[!0-9]*|'') test_die "PULSE_RATE_TEST_LIMIT must be a positive integer" ;; esac
[ "$limit" -gt 0 ] || test_die "PULSE_RATE_TEST_LIMIT must be positive"

old_ifs=$IFS
IFS=,
set -- $PULSE_RATE_TEST_URLS
IFS=$old_ifs
[ "$#" -ge 2 ] || test_die "PULSE_RATE_TEST_URLS must contain at least two ingestion URLs to prove replicas share Redis state"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/pulse-rate.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM
status=$(curl --silent --show-error --output "$tmp/tenant.json" --write-out '%{http_code}' \
    -H "X-Admin-Token: $PULSE_ADMIN_TOKEN" -H 'Content-Type: application/json' \
    --data '{"name":"external-rate-limit-test"}' "${1%/}/v1/tenants")
[ "$status" = 201 ] || { cat "$tmp/tenant.json" >&2; test_die "could not create rate-limit test tenant (HTTP $status)"; }
key=$(jq -er '.apiKey' "$tmp/tenant.json")

accepted=0
rejected=0
i=0
while [ "$i" -lt "$((limit + 1))" ]; do
    endpoint=$(printf '%s\n' "$@" | sed -n "$((i % $# + 1))p")
    body=$(jq -nc --arg id "evt-rate-$i-$(date +%s)" '{eventId:$id,type:"integration.rate",timestamp:(now|todate),payload:{n:1}}')
    code=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
        -H "Authorization: Bearer $key" -H 'Content-Type: application/json' --data "$body" "${endpoint%/}/v1/events")
    case "$code" in 202) accepted=$((accepted + 1)) ;; 429) rejected=$((rejected + 1)) ;; *) test_die "unexpected rate-limit response HTTP $code" ;; esac
    i=$((i + 1))
done
[ "$accepted" = "$limit" ] || test_die "distributed rate limit accepted $accepted requests; expected exactly $limit"
[ "$rejected" = 1 ] || test_die "distributed rate limit rejected $rejected requests; expected one"
