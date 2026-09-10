# Capacity Planning

This is a calculation template. No capacity number is presented as measured
until the benchmark fields are populated.

## Inputs to measure

| Input | Value | Source/status |
| --- | --- | --- |
| Average event bytes | PENDING | k6 payload capture |
| p95 event bytes | PENDING | k6 payload capture |
| Sustainable events/s per ingestion pod | 1,000 accepted/s measured; full-effect rate not established | baseline/stress gap |
| CPU per 1,000 events/s | ingestion near 0%; persistence ~13%; ClickHouse ~174-203% in baseline snapshot | docker stats |
| Memory per pod | ingestion ~35 MiB; persistence ~13 MiB; analytics ~14 MiB | docker stats |
| Network bytes/event | PENDING | interface metrics |
| Kafka partitions and bytes/s/partition | 12 partitions; single tenant concentrated all traffic in partition 11 | kafka groups describe |
| Persistence worker events/s | PENDING | worker benchmark |
| Analytics worker events/s | PENDING | worker benchmark |
| Webhook worker deliveries/s | PENDING | webhook benchmark |
| PostgreSQL write capacity | observed ~583 events/s drain with four workers during measured interval | lag reconciliation |
| Redis command latency/capacity | no rejection in 1k/s distributed baseline; exact ceiling not isolated | k6/Redis |
| ClickHouse ingest/query capacity | limiting consumer; 169,202 lag remained after scale recovery interval | lag reconciliation |

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
| Current baseline | 1000 | not established for full effects | 1 ingestion + 1 worker each | 12 | measured above | measured above | not calculated | not calculated | consumer-bound |
| 100k events/s | 100000 | not measured | requires capacity test | >=12, benchmark required | not measured | not measured | production replication required | production storage sizing required | not approved |
