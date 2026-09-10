# ADR-006: Rate Limiting
## Status
Accepted for the current deployment; distributed multi-replica validation pending.
## Contexto
Per-tenant admission control must not multiply when ingestion has multiple replicas.
## Drivers
- Shared state across API instances.
- Atomic decision under concurrency.
- Explicit behavior when the limiter dependency is unavailable.
## Alternativas consideradas
- Per-process in-memory counters.
- Redis atomic counter.
- Gateway-only rate limiting.
## Decisão
Use Redis and an atomic Lua counter keyed by `pulse:rate:<tenantId>`, with configurable limit and duration. If Redis is unavailable, return `503` rather than fail open.
## Consequências positivas
- Multiple ingestion replicas share the same counter.
- The failure mode avoids uncontrolled admission when the control dependency is unavailable.
## Consequências negativas
- Redis is on the request critical path.
- The current script implements a fixed window counter despite the plan's token-bucket wording; algorithm and fairness should be revisited if requirements demand token-bucket semantics.
## Evidências / benchmarks
Implementation evidence: `internal/ratelimit/redis.go` and `internal/api/http.go`. Two-replica rate-limit test and latency/capacity measurements: **PENDING**.
