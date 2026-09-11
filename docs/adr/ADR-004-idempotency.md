# ADR-004: Idempotency
## Status
Accepted for operational persistence; downstream long-run reconciliation is outside this release.
## Contexto
At-least-once consumers can process the same `(tenantId,eventId)` more than once. Persistence must not create duplicate operational event rows.
## Drivers
- Stable logical identity across retries.
- Safe consumer crash and redelivery behavior.
- No hidden duplicate effect claim.
## Alternativas consideradas
- In-memory deduplication.
- Kafka transactions as the sole mechanism.
- Database uniqueness plus idempotent handlers.
## Decisão
Use `(tenant_id, event_id)` as the logical key. PostgreSQL enforces a unique constraint and uses `ON CONFLICT DO NOTHING`. ClickHouse uses `ReplacingMergeTree` and `uniqExact` summaries, but this path requires explicit duplicate-effect testing before being called proven.
## Consequências positivas
- PostgreSQL persistence is safe against repeated inserts for the same logical event.
- The idempotency key is tenant-scoped and avoids cross-tenant collisions.
## Consequências negativas
- A unique write does not automatically make webhook side effects exactly once.
- ClickHouse merge timing and external delivery need reconciliation tests.
## Evidências / benchmarks
Implementation evidence: `migrations/001_initial.sql`, `internal/persistence/postgres.go`, and `migrations/002_clickhouse.sql`. PostgreSQL duplicate handling and the E2E duplicate path pass; long-run downstream reconciliation was not evaluated.
