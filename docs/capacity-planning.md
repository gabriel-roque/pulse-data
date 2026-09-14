# Capacity Planning

Pulse targets 100,000 events/s at the HTTP-to-Kafka durability boundary. The
public ingestion contract is batch-only.

## Validated profile

```text
POST /v1/events/batch
200 requests/s
500 events/request
100,000 events/s
5-minute constant-arrival-rate hold
48 Kafka partitions
128 tenant keys
```

The latest formal run measured 99,996.7 accepted events/s with zero HTTP
failures, zero dropped iterations, p95 43.83 ms, and p99 76.73 ms. All
30,000,500 requested events matched the accepted and Kafka counts. A `202`
response means Kafka acknowledged every event in the batch.

## Sizing model

```text
request_rate = target_events_per_second / batch_size
payload_bandwidth = target_events_per_second * average_event_bytes * protocol_factor
kafka_retention = payload_bandwidth * retention_seconds * replication_factor
required_partitions >= target_events_per_second / measured_partition_rate
```

The benchmark uses a single Kafka broker and replication factor 1 in local
Compose. Production sizing must independently account for replication,
retention, failure headroom, network bandwidth, partition skew, and recovery.
PostgreSQL, ClickHouse, and webhook consumers are asynchronous downstream
effects and require separate capacity plans.
