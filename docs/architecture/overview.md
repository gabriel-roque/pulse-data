# Architecture Overview

## Runtime path

1. A tenant sends `POST /v1/events/batch` with a bearer API key.
2. Ingestion authenticates the tenant, reserves all batch events in the Redis
   rate limit, validates every strict JSON envelope, and publishes the batch
   synchronously to Kafka.
3. Kafka uses `tenantId` as the message key. `events.raw` is consumed by three
   independent groups: persistence, analytics, and webhook delivery.
4. The API returns `202` only after Kafka acknowledges every event. Terminal
   malformed or explicitly dead-lettered messages go to `events.dlq`.
5. Consumers commit offsets after successful handling. PostgreSQL, ClickHouse,
   and external webhooks are therefore eventually consistent with Kafka.

## Components

| Component | Responsibility | State or dependency |
| --- | --- | --- |
| `ingestion` | Auth, validation, rate limiting, Kafka publish | Kafka, Redis |
| `persistence-worker` | Idempotent operational writes | PostgreSQL |
| `analytics-worker` | Analytical event writes | ClickHouse |
| `webhook-worker` | Subscription lookup and signed delivery | PostgreSQL, external APIs |
| `query-api` | Tenant-scoped analytics queries | ClickHouse |
| `analytics-ui` | Browser interface for analytics summaries | Query API |
| Kafka | Durable transport, consumer groups, DLQ | Topic storage |
| PostgreSQL | Tenants, events, subscriptions, delivery claims | Operational source |
| Redis | Atomic per-tenant rate limiting | Shared limiter state |
| ClickHouse | Time-series analytics | Analytical store |

Prometheus, Grafana, Loki, and Tempo provide the observability layer. The
current Compose environment is local development: one Kafka broker, 12
partitions, replication factor 1, and single-instance dependencies.

## Guarantees and failure boundaries

- Kafka publish failure returns `503`; no accepted response is emitted.
- Redis failure returns `503`; the API does not fail open.
- Consumer fetch and processing failures retry with bounded backoff.
- Invalid Kafka events are validated at the consumer boundary and sent to the DLQ.
- PostgreSQL enforces unique `(tenant_id, event_id)` persistence.
- Webhooks use claims, HMAC headers, timeout, retry backoff, circuit breaker,
  bulkhead, and SSRF checks. External side effects remain at-least-once.
- HTTP readiness checks configured dependencies; liveness checks only the process.
- Shutdown uses bounded contexts; full consumer-drain evidence is not part of this release.

## Scaling constraints

Consumer groups cannot usefully exceed available partitions. The default
`tenantId` key preserves per-tenant ordering but can create a hot partition for
a dominant tenant. Production deployments need multiple Kafka brokers,
replication, external secrets, network policy, and lag-based consumer scaling.

See [`diagram.mmd`](diagram.mmd) for the editable logical diagram and the ADRs
for decisions about messaging, delivery, partitioning, storage, rate limiting,
webhook resilience, observability, and autoscaling.
