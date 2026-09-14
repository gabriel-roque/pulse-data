# Validation Snapshot

## Scope

The capacity claim is 100,000 events/s from HTTP batch ingestion through the
Kafka acknowledgement boundary. It does not include asynchronous PostgreSQL,
ClickHouse, or webhook completion.

## Quality gates

| Area | Result |
| --- | --- |
| Go tests, formatting, vet, and race detector | PASS |
| Compose, Helm, and Prometheus validation | PASS |
| Integration and E2E batch flows | PASS |
| Kafka batch publish and consumer DLQ semantics | PASS |
| Batch body and event-count limits | PASS |

## Capacity evidence

The formal run used 500 events per request, 200 requests/s, 128 tenants, 48
Kafka partitions, and a constant five-minute target window.

| Signal | Result |
| --- | ---: |
| Offered target | 100,000 events/s |
| Effective accepted rate | 99,996.7 events/s |
| Requested/accepted/Kafka events | 30,000,500 / 30,000,500 / 30,000,500 |
| HTTP failures | 0% |
| Dropped iterations | 0 |
| Batch p95 | 43.83 ms |
| Batch p99 | 76.73 ms |

The current test additionally requires requested, accepted, and Kafka event
counts to match exactly. Generated credentials remain in a temporary directory
and are removed after the run. Artifacts contain configuration, resource
samples, k6 logs, summary metrics, and aggregate counts only.

## Reproduce

```sh
make test
make race
make validate
make capacity-test
```

The capacity environment is isolated from the default Compose project and is
removed automatically after success or failure.
