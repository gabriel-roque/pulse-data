#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

for command in curl jq python3; do
    require_cmd "$command"
done
require_cmd k6

for file in \
    tests/integration/run.sh \
    tests/integration/rate-limit.sh \
    tests/integration/dlq.sh \
    tests/integration/shutdown.sh \
    tests/e2e/run.sh \
    tests/chaos/run.sh \
    tests/load/common.js \
    tests/load/smoke.js \
    tests/load/baseline.js \
    tests/load/progression.js \
    tests/load/spike.js \
    tests/load/stress.js \
     tests/load/soak.js; do
     [ -f "$ROOT_DIR/$file" ] || test_die "expected external test file is missing: $file"
done

for file in tests/integration/run.sh tests/integration/rate-limit.sh tests/integration/dlq.sh tests/integration/shutdown.sh tests/e2e/run.sh tests/chaos/run.sh; do
     [ -x "$ROOT_DIR/$file" ] || test_die "test script is not executable: $file"
done

python3 -c 'import ast, pathlib, sys; ast.parse(pathlib.Path(sys.argv[1]).read_text())' "$ROOT_DIR/tests/e2e/webhook_mock.py"
printf 'external test syntax validation passed\n'
