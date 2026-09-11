# ADR-001: Messaging Platform
## Status
Accepted for the current local implementation; production HA is outside this release.
## Contexto
Pulse needs durable fan-out from ingestion to independent persistence, analytics, and webhook consumers.
## Drivers
- Independent consumer groups and replay/DLQ handling.
- Tenant-keyed ordering and backpressure visibility.
- Local reproducibility with Docker Compose.
## Alternativas consideradas
- Direct synchronous calls from ingestion to every store.
- PostgreSQL outbox without a broker.
- Kafka-compatible event streaming platform.
## Decisão
Use Kafka with `events.raw` and `events.dlq`. The producer uses synchronous writes and all required acknowledgements. Consumers use independent groups for persistence, analytics, and webhooks.
## Consequências positivas
- Fan-out is decoupled from request handling after the publish durability point.
- Consumer lag and partition assignment provide operational signals.
- Replay and terminal-failure routing have a defined transport.
## Consequências negativas
- Kafka operations, partition sizing, rebalance behavior, and retention must be managed.
- The Compose topology has one broker and replication factor 1, so it is not HA evidence.
## Evidências / benchmarks
Implementation evidence: `internal/kafka/producer.go` and Compose topic creation. The compact capacity probe measured saturation below the offered target; production HA was not evaluated.
