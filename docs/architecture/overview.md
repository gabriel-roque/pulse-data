# Architecture Overview

## Scope

Pulse accepts tenant-scoped events over HTTP, publishes them to Kafka, and
fans them out through independent consumer groups. The current Compose stack
contains one Kafka broker, PostgreSQL, Redis, ClickHouse, the two HTTP
services, three workers, and the observability services. The Helm chart models
multiple replicas and autoscaling for the HTTP services and workers.

## Components

| Component | Responsibility | Durable dependency |
| --- | --- | --- |
| `ingestion` | Authenticate, rate limit, validate, and publish events. | Kafka; Redis for rate limiting |
| `persistence-worker` | Consume events and insert them into PostgreSQL. | PostgreSQL |
| `analytics-worker` | Consume events and record analytical rows. | ClickHouse |
| `webhook-worker` | Resolve subscriptions and deliver signed webhooks with retry controls. | PostgreSQL; external endpoint |
| `query-api` | Authenticate tenants and query ClickHouse summaries. | ClickHouse |
| Kafka | Event transport, consumer groups, and `events.dlq`. | Compose volume in local setup |
| PostgreSQL | Tenant, event, and webhook subscription state. | PostgreSQL volume or managed service |
| Redis | Atomic per-tenant rate-limit counter. | Redis volume or managed service |
| ClickHouse | Analytical event storage and summaries. | ClickHouse volume or managed service |
| Prometheus/Grafana/Loki/Tempo | Metrics, dashboards, logs, and traces infrastructure. | Local volumes in Compose |

## Event path

1. The client sends `POST /v1/events` with a tenant API key.
2. The API derives the tenant from the bearer credential; client-supplied
   `tenantId` is not accepted in the request schema.
3. Redis atomically checks the tenant's configured counter and window.
4. Strict JSON decoding and event validation are applied, including payload
   size and future timestamp limits.
5. Kafka receives the event with the tenant ID as the message key and an
   `event-id` header. The producer uses synchronous writes and all required
   acknowledgements.
6. The API returns `202` and `X-Pulse-Durability: kafka-ack` only after publish
   succeeds.
7. The persistence, analytics, and webhook consumer groups process the same
   topic independently.
8. Consumers commit offsets after successful handling. Terminal failures may
   be written to `events.dlq` before the source offset is committed.

## Data and consistency

PostgreSQL is the operational source for tenant and subscription state and
has a unique `(tenant_id, event_id)` constraint for persistence idempotency.
ClickHouse is the analytical store and uses `ReplacingMergeTree`; the
analytics path must still be validated for duplicate effects under redelivery.
The system is intentionally eventually consistent between Kafka, PostgreSQL,
ClickHouse, and webhooks.

## Failure boundaries

- Kafka publish failure produces `503` and no accepted response.
- Redis failure produces `503`; the current API does not fail open.
- A worker error stops its consumer loop unless the error is explicitly
  terminal and routed to the DLQ.
- Webhook delivery has a per-endpoint bulkhead, retry backoff, circuit breaker,
  timeout, HMAC headers, and SSRF checks.
- HTTP services shut down on SIGINT/SIGTERM with a bounded 10-second HTTP
  shutdown context. Full consumer drain behavior requires the shutdown tests
  and is currently evidence-pending.

## Scaling constraints

Kafka consumer groups can usefully scale only up to the number of partitions
with work. The Compose topic is created with 12 partitions and replication
factor 1, which is a local development topology, not a production HA claim.
The default partition key is `tenantId`; this preserves per-tenant ordering but
can create a hot partition for a dominant tenant. The hot-partition benchmark
is required before changing this policy.

## Observability

HTTP services expose `/metrics`. The current metric families include request
counts and duration, received/processed/failed/duplicate events, rate-limit
rejections, Kafka publish latency, webhook delivery/retry/DLQ, consumer lag,
workers, and queue depth. Prometheus scrapes ingestion and query-api in the
Compose configuration. End-to-end tracing and production dashboard coverage
remain validation items, not measured claims.
