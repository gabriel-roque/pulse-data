# ADR-005: Operational vs Analytical Storage
## Status
Accepted for the current implementation; production workload sizing is outside this release.
## Contexto
Tenant, event-ingest, and subscription operations have different access patterns from analytical summaries by time and event type.
## Drivers
- Transactional constraints and idempotency for operational state.
- Columnar analytical queries and aggregation.
- Independent scaling and eventual consistency are acceptable for analytics.
## Alternativas consideradas
- PostgreSQL for all reads and writes.
- ClickHouse for all data, including control-plane state.
- PostgreSQL for operational state and ClickHouse for analytics.
## Decisão
Use PostgreSQL for tenants, events, and webhook subscriptions. Use ClickHouse for analytical event rows and time/type summaries. Kafka remains the fan-out boundary between the stores.
## Consequências positivas
- PostgreSQL constraints support tenant and event identity.
- ClickHouse can serve analytical aggregation without coupling it to control-plane transactions.
## Consequências negativas
- Data appears in stores at different times and requires reconciliation.
- Two storage systems increase operations, backup, and schema-management work.
## Evidências / benchmarks
Implementation evidence: migrations `001_initial.sql` and `002_clickhouse.sql`, plus the worker handlers. E2E query/ingest paths pass; compact capacity sizing is documented separately.
