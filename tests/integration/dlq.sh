#!/bin/sh

set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
. "$ROOT_DIR/scripts/testlib.sh"

[ "${PULSE_INTEGRATION_REAL:-}" = "1" ] || test_die "set PULSE_INTEGRATION_REAL=1 to run Kafka retry/DLQ validation"
require_cmd docker
compose -f "$ROOT_DIR/${COMPOSE_FILE:-docker-compose.yml}" ps kafka >/dev/null
marker="pulse-dlq-marker-$(date +%s)-$$"
topic=${PULSE_KAFKA_TOPIC:-events.raw}
dlq=${PULSE_KAFKA_DLQ_TOPIC:-events.dlq}

printf '%s\n' "$marker" | compose -f "$ROOT_DIR/${COMPOSE_FILE:-docker-compose.yml}" exec -T kafka \
    /opt/kafka/bin/kafka-console-producer.sh --bootstrap-server kafka:9092 --topic "$topic"

if ! output=$(compose -f "$ROOT_DIR/${COMPOSE_FILE:-docker-compose.yml}" exec -T kafka \
    /opt/kafka/bin/kafka-console-consumer.sh --bootstrap-server kafka:9092 --topic "$dlq" \
    --from-beginning --timeout-ms "${PULSE_DLQ_TIMEOUT_MS:-30000}" 2>/dev/null); then
    test_die "Kafka DLQ consumer command failed; Kafka or the DLQ dependency is unavailable"
fi
case "$output" in
    *"$marker"*) printf 'DLQ passed: malformed marker was observed in %s\n' "$dlq" ;;
    *) test_die "malformed marker was not observed in $dlq; consumer/DLQ dependency may be absent" ;;
esac
