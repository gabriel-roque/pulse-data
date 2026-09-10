#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"
require_cmd k6
require_cmd curl
require_env PULSE_API_URL
[ -n "${PULSE_API_KEY:-}${PULSE_API_KEYS:-}" ] || test_die "required environment variable is missing: PULSE_API_KEY or PULSE_API_KEYS"

profile=${1:-}
case "$profile" in
    smoke|baseline|progression|spike|stress|soak) ;;
    *) test_die "usage: scripts/run-k6.sh smoke|baseline|progression|spike|stress|soak" ;;
esac

wait_http "${PULSE_API_URL%/}/health/ready" "${PULSE_READY_TIMEOUT:-120}"
exec k6 run "$ROOT_DIR/tests/load/$profile.js"
