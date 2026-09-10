# ADR-007: Webhook Resilience
## Status
Accepted for the current implementation; failure-matrix validation pending.
## Contexto
External endpoints are slow, unreliable, and outside Pulse's transaction boundary. One bad endpoint must not block every endpoint.
## Drivers
- Bounded latency and concurrency.
- Retry transient failures with visible terminal failure.
- Protect against SSRF and replay of unsigned requests.
## Alternativas consideradas
- One unbounded goroutine per delivery.
- Immediate single attempt.
- Bounded per-endpoint dispatch with timeout, retries, circuit breaker, bulkhead, and DLQ.
## Decisão
Use a 15-second HTTP timeout, per-endpoint bulkhead limit 20, circuit breaker after three failures with one-minute cooldown, five retries with 1s/5s/30s/5m/30m backoff and jitter, HMAC headers, and endpoint validation. Terminal delivery errors are represented as `DeadLetterError` for Kafka DLQ handling.
## Consequências positivas
- Slow or failing endpoints are isolated and retried.
- Receivers can verify timestamped HMAC signatures and event IDs.
- Permanent failures have a DLQ path.
## Consequências negativas
- Retries can produce repeated external attempts and do not guarantee exactly-once effects.
- Per-endpoint state can grow with the number of subscriptions and needs lifecycle controls.
## Evidências / benchmarks
Implementation evidence: `internal/webhook/resilience.go`, `hmac.go`, and `ssrf.go`. Bulkhead, breaker, retry, timeout, SSRF, and DLQ scenario results: **PENDING**.
