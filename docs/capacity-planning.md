# Capacity Planning

This is a calculation template. No capacity number is presented as measured
until the benchmark fields are populated.

## Inputs to measure

| Input | Value | Source/status |
| --- | --- | --- |
| Average event bytes | PENDING | k6 payload capture |
| p95 event bytes | PENDING | k6 payload capture |
| Sustainable events/s per ingestion pod | PENDING | progression/stress |
| CPU per 1,000 events/s | PENDING | container metrics |
| Memory per pod | PENDING | container metrics |
| Network bytes/event | PENDING | interface metrics |
| Kafka partitions and bytes/s/partition | PENDING | broker metrics |
| Persistence worker events/s | PENDING | worker benchmark |
| Analytics worker events/s | PENDING | worker benchmark |
| Webhook worker deliveries/s | PENDING | webhook benchmark |
| PostgreSQL write capacity | PENDING | pool and database metrics |
| Redis command latency/capacity | PENDING | Redis metrics |
| ClickHouse ingest/query capacity | PENDING | ClickHouse metrics |

## Planning equations

Use measured values and state assumptions explicitly.

```text
ingestion_pods = ceil(target_events_per_second / sustainable_events_per_pod)
worker_replicas = ceil(target_events_per_second / sustainable_events_per_worker)
required_partitions >= max(worker_replicas, target_events_per_second / events_per_second_per_partition)
payload_bandwidth = target_events_per_second * average_event_bytes * protocol_factor
kafka_retention_bytes = payload_bandwidth * retention_seconds * replication_factor
daily_storage = target_events_per_day * average_event_bytes * storage_factor
```

For 100k events/s, substitute the measured event size, replication factor,
retention, and overhead. Do not assume the Compose replication factor of 1 is
appropriate for production. Include headroom, failure capacity, rebalance
capacity, and the hot-tenant partition distribution.

## Scaling notes

- Ingestion and query-api are HTTP services and can scale horizontally.
- Kafka consumers cannot usefully exceed the number of partitions in their
  consumer group.
- The current partition key is tenant ID, preserving per-tenant ordering but
  requiring a hot-partition test for dominant tenants.
- Helm defaults contain resource requests/limits and HPA bounds, but those
  values are configuration defaults, not benchmark-derived recommendations.
- Any change to partitioning, resource requests, HPA targets, or storage
  retention must reference a new measurement and the applicable ADR.

## Capacity result template

| Scenario | Target rate | Sustainable rate | Pods/replicas | Partitions | CPU | Memory | Kafka retention | Storage/day | Decision |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| Current baseline | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING |
| 100k events/s | 100000 | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING | PENDING |
