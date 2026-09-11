# Capacity Planning

This document distinguishes measured local behavior from production sizing.
The default Compose profile contains steady-state services within
approximately 2 vCPU and 3 GiB. It is a compact developer profile, not a
100k events/s guarantee.

## Current evidence

The short compact probe offered 100,000 req/s with 16 tenants and measured:

| Signal | Observation |
| --- | --- |
| Achieved HTTP rate | ~910 req/s |
| p95 latency | 4.62 s |
| HTTP failures | 5.63% |
| Dropped k6 iterations | 1,178,740 |
| Ingestion memory | 176.2 MiB / 192 MiB |
| Kafka memory | 407.8 MiB / 512 MiB |
| ClickHouse memory | 428.7 MiB / 512 MiB |
| Interpretation | Compact profile saturated far below the offered target. |

This is a short probe, not a maximum-sustainable-throughput claim. The full
run stores raw evidence under `artifacts/capacity/<run-id>/`.

## Sizing model

Use measured values rather than the target to size a deployment:

```text
ingestion_replicas = ceil(target_events_per_second / measured_events_per_pod)
worker_replicas = ceil(target_events_per_second / measured_events_per_worker)
required_partitions >= max(worker_replicas, target_events_per_second / partition_rate)
payload_bandwidth = target_events_per_second * average_event_bytes * protocol_factor
kafka_retention = payload_bandwidth * retention_seconds * replication_factor
```

For a real 100k/s deployment, measure event size, partition distribution,
consumer throughput, database write capacity, ClickHouse ingest capacity,
network bandwidth, retention, failure headroom, and recovery backlog. The
single-broker Compose topology is not production HA evidence.

## Constraints

- Kafka consumers cannot usefully exceed available partitions.
- `tenantId` preserves ordering but can create a hot partition.
- PostgreSQL and ClickHouse effects are asynchronous and must be reconciled.
- Resource requests, HPA targets, partitioning, and retention require a new
  measured run and an ADR update before production approval.
