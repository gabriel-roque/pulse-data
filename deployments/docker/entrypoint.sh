#!/bin/sh
set -eu

service="${PULSE_SERVICE:-ingestion}"
case "$service" in
  ingestion|query-api|persistence-worker|analytics-worker|webhook-worker)
    exec "/usr/local/bin/$service" "$@"
    ;;
  *)
    echo "unsupported PULSE_SERVICE: $service" >&2
    exit 64
    ;;
esac
