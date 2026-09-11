# ADR-003: Partition Strategy
## Status
Accepted for v1; hot-tenant alternatives are outside this release.
## Contexto
Events need a stable Kafka key. Ordering by tenant is useful, but a dominant tenant can concentrate traffic on one partition.
## Drivers
- Preserve per-tenant ordering in v1.
- Distribute ordinary multi-tenant traffic.
- Keep the initial topology simple and observable.
## Alternativas consideradas
- Random or event-ID partitioning.
- `tenantId` as the key.
- `tenantId + bucket` as the key, trading ordering for distribution.
## Decisão
Use `tenantId` as the Kafka message key. Reconsider `tenantId + bucket` only after measuring a hot tenant and recording the ordering impact in a follow-up ADR.
## Consequências positivas
- Events for one tenant are routed consistently and preserve partition order.
- The partitioning rule is simple for producers and consumers.
## Consequências negativas
- A single dominant tenant can create a hot partition.
- Scaling consumers beyond the partition count does not increase useful parallelism.
## Evidências / benchmarks
Implementation evidence: `internal/kafka/producer.go:30` and the 12-partition local topic. The capacity probe used multiple tenants; a dedicated hot-tenant comparison was not evaluated.
