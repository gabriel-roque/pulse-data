# Troubleshooting

## Startup fails with a missing dependency

The normal services require PostgreSQL, Kafka, Redis, and the configured
ClickHouse address. Confirm the environment variables from `.env.example` and
run `make up`. Workers intentionally reject `PULSE_LOCAL_FALLBACK=true`.

Useful checks:

```sh
docker compose ps
docker compose config --quiet
docker compose logs --since 10m ingestion persistence-worker analytics-worker webhook-worker
```

## `make e2e` times out

Check ingestion readiness on port 8080 and query-api readiness on port 8081.
Inspect dependency health checks and use `docker compose logs` before
restarting. `make e2e` runs the tenant, webhook, duplicate, persistence, and
analytics flow; its explicit environment is documented in `tests/e2e/README.md`.

## Ingestion returns `401`

Use `Authorization: Bearer <tenant-api-key>`. Tenant creation requires the
`X-Admin-Token` header and returns the API key once. API keys are hashed in the
store; after rotation the previous key is invalid.

## Ingestion returns `400`

The event body must contain only `eventId`, `type`, `timestamp`, and `payload`.
The event ID and type are limited to 200 characters, type cannot contain
whitespace, timestamp cannot be more than five minutes in the future, payload
must be valid non-null JSON, and the default payload limit is 1 MiB.

## Ingestion returns `429` or `503`

`429` indicates the per-tenant Redis counter exceeded `PULSE_RATE_LIMIT` in
`PULSE_RATE_WINDOW`. `503` can indicate Redis unavailability or Kafka publish
failure. Check Redis connectivity, Kafka health, and
`pulse_kafka_publish_latency_seconds` before changing limits.

## Analytics summary is empty or unavailable

Analytics is eventually consistent. Check analytics-worker logs and lag, then
query a range containing `event_timestamp`. A `501` means ClickHouse was not
configured for that process; a `503` means the query failed. Confirm the
tenant API key and use RFC3339 `from` and `to` values with `from < to`.

## Webhook creation is rejected

Only `http` and `https` endpoints with resolvable public addresses are
accepted. Localhost, private, link-local, unspecified, cloud metadata, and
user-info URLs are blocked. A test receiver must be reachable from the worker
and satisfy the SSRF policy; see `tests/e2e/README.md`.

## No benchmark result is shown

That is intentional until a load profile is executed and captured. Use the
measured profile table in `docs/benchmarks/README.md` and the final validation
report; never substitute configured targets for measurements.
