# Capacity Benchmark

The only supported load benchmark validates 100,000 events/s at the Kafka
acknowledgement boundary.

## Workload

| Setting | Default |
| --- | ---: |
| Target | 100,000 events/s |
| Batch size | 500 events |
| HTTP rate | 200 requests/s |
| Duration | 5 minutes |
| Tenants | 128 |
| Kafka partitions | 48 |

`make capacity-test` creates an isolated Compose project with fresh volumes,
starts ingestion and its required dependencies, provisions tenants, executes
k6, reconciles accepted events with Kafka offsets, writes artifacts under
`artifacts/capacity/<run-id>/`, and removes the temporary environment.

## Acceptance

- HTTP error rate: 0%.
- Event acceptance: 100%.
- Dropped iterations: 0.
- Effective accepted rate: at least 99.5% of the target.
- Batch latency: p95 below 1 second and p99 below 2 seconds.
- Requested events = accepted events = Kafka topic offsets.

The most recent formal run offered 100,000 events/s and measured 99,996.7
accepted events/s. All 30,000,500 requested events were accepted and matched
the Kafka offsets, with zero failures, zero dropped iterations, p95 43.83 ms,
and p99 76.73 ms.

PostgreSQL authentication participates in the request path. Asynchronous
PostgreSQL persistence, ClickHouse, webhooks, replication factor, and
multi-broker high availability are outside this ingress benchmark.
