#!/bin/sh

set -eu

test_die() {
    printf 'TEST ERROR: %s\n' "$*" >&2
    exit 1
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || test_die "required command is missing: $1"
}

require_env() {
    eval "value=\${$1-}"
    [ -n "$value" ] || test_die "required environment variable is missing: $1"
}

require_flag() {
    eval "actual=\${$1-}"
    [ "$actual" = "$2" ] || test_die "$1=$2 is required; refusing to run an external/destructive test without explicit confirmation"
}

wait_http() {
    url=$1
    timeout=${2:-60}
    i=0
    while [ "$i" -lt "$timeout" ]; do
        if curl --silent --show-error --fail --connect-timeout 2 --max-time 5 "$url" >/dev/null; then
            return 0
        fi
        i=$((i + 1))
        sleep 1
    done
    test_die "timed out waiting for HTTP endpoint: $url"
}

http_status() {
    url=$1
    shift
    curl --silent --show-error --output "$TEST_HTTP_BODY" --write-out '%{http_code}' "$@" "$url"
}

compose() {
    docker compose "$@"
}

compose_service_running() {
    service=$1
    compose ps --status running --services | awk -v wanted="$service" '$0 == wanted { found=1 } END { exit(found ? 0 : 1) }'
}

require_cmd mktemp
TEST_HTTP_BODY=${TEST_HTTP_BODY:-$(mktemp "${TMPDIR:-/tmp}/pulse-test-body.XXXXXX")}
export TEST_HTTP_BODY
