# Compose Chaos

Every scenario requires `PULSE_CHAOS_REAL=1` and
`PULSE_CHAOS_CONFIRM=I_UNDERSTAND`. The runner uses `stop`, `restart`, or
`SIGTERM` only; it never runs `down -v`, removes volumes, or silently ignores
an unavailable dependency. A trap restores the named services and readiness
is checked after recovery.

Scenarios:

| Scenario | Hypothesis | Expected signal |
| --- | --- | --- |
| `ingestion-restart` | SIGTERM removes readiness and the service recovers | readiness outage and recovery time |
| `worker-restart` | consumer rebalance recovers the backlog | worker returns and lag drains |
| `kafka-restart` | acknowledged messages survive broker restart | Kafka and readiness recover |
| `postgres-unavailable` | durability failure is explicit, not silent loss | no false accepted write; DB recovers |
| `redis-unavailable` | distributed rate limiting fails closed | HTTP 503 while Redis is absent |
| `webhook-failure` | deterministic receiver 500s are retried before success | receiver attempts and one final delivery; permanent DLQ evidence is separate |

The generic runner validates infrastructure recovery. `webhook-failure`
delegates to the E2E receiver and requires `PULSE_CHAOS_WEBHOOK_URL`,
`PULSE_E2E_WEBHOOK_STATUS_URL`, `PULSE_E2E_WEBHOOK_CONFIG_URL`, and
`PULSE_ADMIN_TOKEN`; its failure count defaults to two. Use `dlq.sh` with a
permanent failure/poison message to collect DLQ evidence. A green recovery
check alone is not a claim that no data was lost.
