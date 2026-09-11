# Runbooks

These runbooks assume the Compose environment unless an environment is named
explicitly. Capture timestamps, commands, service versions, request IDs, and
Prometheus evidence for every incident. Do not run `make clean` during an
incident unless data destruction has been approved.

## First response

1. Confirm scope with `/health/live` and `/health/ready` on ingestion and
   query-api.
2. Check `docker compose ps` and `docker compose logs --since 15m`.
3. Inspect `/metrics`, Prometheus targets, Kafka consumer lag, database health,
   Redis availability, and webhook retry/DLQ counters.
4. Preserve evidence before restarting a service.
5. Use the smallest restart that tests the suspected failure. Record recovery
   time and whether accepted events were lost or duplicated.

## Restart a service safely

```sh
docker compose ps
docker compose restart ingestion
curl --fail http://127.0.0.1:${PULSE_INGESTION_PORT:-8080}/health/ready
```

For a worker, verify the consumer returns and lag drains. A healthy process is
not enough evidence that the backlog was recovered.

## Kafka backlog or consumer failure

- Check worker logs and the `pulse_consumer_lag` metric.
- Check broker health and topic/partition availability.
- Restart only the affected worker first.
- Verify the relevant consumer group resumes and lag decreases.
- If a poison message blocks progress, preserve the message and use the DLQ
  procedure; do not delete offsets as a first response.

Expected current behavior: transient fetch and handler errors retry with
bounded backoff. Invalid messages and explicit `DeadLetterError` failures are
written to `events.dlq` before the source offset is committed.

## PostgreSQL unavailable or slow

- Check `docker compose logs postgres` and `pg_isready` from the Compose network.
- Expect ingestion publish to fail with `503` if the failure prevents the
  configured publish path or dependent service startup.
- Do not claim an accepted event was persisted until PostgreSQL and the
  persistence worker have been checked.
- After recovery, verify worker lag drains and compare accepted event IDs with
  PostgreSQL rows.

## Redis unavailable

The current API returns `503` with `rate limiter unavailable`; it does not
fail open. Restore Redis, confirm `PING`, then verify a normal authenticated
ingestion request and the rate-limit metric.

## Webhook failures

- Check `pulse_webhook_delivery_total`, retry, and DLQ counters.
- Confirm endpoint DNS and SSRF policy are still valid.
- Inspect receiver status and the `X-Pulse-Event-Id` header.
- The dispatcher uses a 15-second client timeout, five retries, backoff
  intervals of 1s, 5s, 30s, 5m, and 30m with jitter, a circuit breaker after
  three failures, and a per-endpoint bulkhead of 20.
- A final delivery or DLQ record must be verified; receiver recovery alone is
  not proof of delivery.

## Graceful shutdown

Send SIGTERM, confirm readiness behavior, and observe worker processing and
offset commits. Use the gated script in `tests/integration/shutdown.sh`.
Record the drain duration, in-flight event outcome, and any duplicate or lost
effects. The current release has no recorded full drain measurement.
