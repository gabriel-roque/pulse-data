# Integration Tests

These scripts exercise the running Compose deployment from outside the
processes. They never call `docker compose down -v` and fail when a required
tool, endpoint, credential, or service is unavailable.

## Commands

```sh
PULSE_INTEGRATION_REAL=1 PULSE_ADMIN_TOKEN=change-me-admin tests/integration/run.sh
PULSE_INTEGRATION_REAL=1 PULSE_ADMIN_TOKEN=change-me-admin \
  PULSE_RATE_TEST_LIMIT=10 PULSE_RATE_TEST_URLS=http://127.0.0.1:8080,http://127.0.0.1:8082 \
  tests/integration/rate-limit.sh
PULSE_INTEGRATION_REAL=1 tests/integration/dlq.sh
PULSE_INTEGRATION_REAL=1 PULSE_SHUTDOWN_CONFIRM=I_UNDERSTAND tests/integration/shutdown.sh
```

The duplicate test expects one PostgreSQL row and one distinct ClickHouse
summary row. `rate-limit.sh` requires two externally reachable ingestion
replicas and proves that the Redis counter is shared. `dlq.sh` publishes an
invalid Kafka value and requires that exact marker in `events.dlq`.
