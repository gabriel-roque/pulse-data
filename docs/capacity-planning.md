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

## Measured boundary

A fresh-volume single-node profile with approximately 10.7 vCPU and 12 GiB of
service limits was tested at the boundary. With 128 tenants and 5-second ramp
stages plus a 30-second hold:

| Offered rate | HTTP result | p95 | p99 | Consumer result |
| ---: | ---: | ---: | ---: | --- |
| 2,300 req/s | 0% errors | 205.6 ms | 276.1 ms | Kafka lag grew downstream |
| 2,400 req/s | 0% errors | 277.3 ms | 337.3 ms | Failed the 250 ms p95 SLO |

The measured durable-ingestion limit under the current SLO is **2,300 req/s**.
It is not an end-to-end processing limit. After the 2,400 req/s run, the
analytics group drained approximately 496 events/s and persistence drained
approximately 658 events/s while backlog remained. The worker handlers perform
one database write per event, so the current topology has no evidence for a
100k/s end-to-end claim.

## Scalable lab profile

The lab changes the workload contract and the worker path together:

- `POST /v1/events/batch` publishes 500 events in one Kafka durability call.
- Kafka uses 48 partitions and each downstream group runs four workers.
- Persistence uses PostgreSQL `COPY` for unique batches and falls back to an
  idempotent multi-row insert on unique conflicts.
- Analytics uses ClickHouse `PrepareBatch`.
- Webhook subscription lookups are shared across events in each batch.
- Redis rate limiting is disabled for the throughput-only run.

The latest formal hold offered `100,000 events/s` and accepted `99,885.8
events/s` at the Kafka durability boundary with zero dropped iterations and
HTTP SLOs passing, but its downstream persistence and analytics lag remained
after load stopped. It is not accepted as sustainable end-to-end evidence. The
default capacity script validates the Kafka ingress contract with a direct
constant-rate hold, zero dropped iterations, and exact batch counts. The
optional `PULSE_CAPACITY_GATE=end-to-end` mode additionally requires consecutive
zero-lag snapshots and store reconciliation. Neither mode changes the fact
that one-event-per-request traffic has a different throughput profile.

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
- `202 Accepted` measures Kafka durability; consumer lag must be zero or
  decreasing at the offered rate before calling a workload sustainable.
- Resource requests, HPA targets, partitioning, and retention require a new
  measured run and an ADR update before production approval.
