# Chaos Report

The repository contains gated Compose scenarios in `tests/chaos/run.sh`.
This report is a result template, not evidence that the scenarios have run.
Every row is `PENDING` until the command output, timestamps, and reconciliation
checks are attached.

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
| `ingestion-restart` | SIGTERM removes readiness and service recovers | Readiness outage followed by recovery | PENDING | PENDING | PENDING | PENDING | PENDING |
| `worker-restart` | Consumer rebalance recovers backlog | Worker returns and lag drains | PENDING | PENDING | PENDING | PENDING | PENDING |
| `kafka-restart` | Acknowledged messages survive broker restart | Kafka and readiness recover | PENDING | PENDING | PENDING | PENDING | PENDING |
| `postgres-unavailable` | Durability failure is explicit | No false accepted write; database recovers | PENDING | PENDING | PENDING | PENDING | PENDING |
| `redis-unavailable` | Rate limiting fails closed | HTTP 503 while Redis is absent | PENDING | PENDING | PENDING | PENDING | PENDING |
| `webhook-failure` | Receiver failures retry before success | Attempts and final delivery are observed | PENDING | PENDING | PENDING | PENDING | PENDING |

## Required evidence

Record the tested commit, environment, service versions, start/end UTC times,
fault injection command, readiness and request errors, Kafka lag, worker logs,
database reconciliation, receiver attempts, DLQ contents, and dashboards. A
green health check alone does not prove that accepted events were not lost.

Permanent poison-message/DLQ evidence is collected separately with the gated
integration `tests/integration/dlq.sh`.
