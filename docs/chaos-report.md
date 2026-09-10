# Chaos Report

The repository contains gated Compose scenarios in `tests/chaos/run.sh`. The
recovery-only scenarios below were executed on commit `55e9d98`; they preserve
volumes and do not claim business-event reconciliation where it was not run.

## Safety gate

Each scenario requires:

```sh
PULSE_CHAOS_REAL=1 PULSE_CHAOS_CONFIRM=I_UNDERSTAND tests/chaos/run.sh <scenario>
```

The runner uses stop, restart, or SIGTERM and does not remove volumes. The
webhook scenario additionally requires the receiver URLs and admin token
listed in `tests/chaos/README.md`.

## Scenario matrix

| Scenario | Hypothesis | Expected result | Observed | Recovery time | Loss | Duplicate effects | Evidence/correction |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `ingestion-restart` | SIGTERM removes readiness and service recovers | Readiness outage followed by recovery | PASS | 0s reported by runner | no volume loss observed | not reconciled | recovery runner output |
| `worker-restart` | Consumer rebalance recovers backlog | Worker returns and lag drains | PASS restart/recovery | 0s reported by runner | not reconciled | not reconciled | recovery runner output |
| `kafka-restart` | Acknowledged messages survive broker restart | Kafka and readiness recover | PASS restart/recovery | 0s reported by runner | not reconciled | not reconciled | recovery runner output |
| `postgres-unavailable` | Durability failure is explicit | Database recovers | PASS recovery only | 0s reported by runner | request assertion not executed | not applicable | dependency recovery output |
| `redis-unavailable` | Rate limiting fails closed | Redis recovers | PASS recovery only | 0s reported by runner | request assertion not executed | not applicable | dependency recovery output |
| `webhook-failure` | Receiver failures retry before success | Attempts and final delivery are observed | PASS in E2E with deterministic receiver | under retry schedule | no silent loss in test | one final delivery | E2E output |

## Required evidence

Record the tested commit, environment, service versions, start/end UTC times,
fault injection command, readiness and request errors, Kafka lag, worker logs,
database reconciliation, receiver attempts, DLQ contents, and dashboards. A
green health check alone does not prove that accepted events were not lost.

Permanent poison-message/DLQ evidence is collected separately with the gated
integration `tests/integration/dlq.sh`.
