# ADR-002: Delivery Semantics
## Status
Accepted for the current implementation; full recovery reconciliation is outside this release.
## Contexto
Distributed retries and consumer restarts can redeliver an event. Exactly-once effects across PostgreSQL, ClickHouse, and external webhooks require stronger coordination than the current system provides.
## Drivers
- Do not acknowledge an event before Kafka publish succeeds.
- Tolerate consumer crashes and redelivery.
- Make failure visible through retries, lag, and DLQ.
## Alternativas consideradas
- At-most-once processing.
- Exactly-once end-to-end across external systems.
- At-least-once transport with idempotent effect handlers.
## Decisão
Use synchronous Kafka publish for the ingestion acceptance point, then at-least-once consumers that commit offsets after successful handling. Terminal failures may be written to the DLQ before committing the source offset. Do not promise exactly-once end-to-end.
## Consequências positivas
- Accepted requests have an explicit Kafka durability header and failed publishes return `503`.
- Consumer restart can retry uncommitted work.
## Consequências negativas
- Redelivery is possible and every effect must be reconciled for duplicates.
- External webhook delivery cannot be made exactly once by Pulse alone.
## Evidências / benchmarks
Implementation evidence: `internal/api/http.go` and `internal/kafka/producer.go`. Duplicate persistence and E2E paths are covered; full crash/recovery reconciliation was not evaluated.
