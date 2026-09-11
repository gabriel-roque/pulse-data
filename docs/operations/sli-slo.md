# SLI and SLO

These are the production objectives and the queries used to evaluate them.
The compact local probe does not qualify as production SLO evidence.

| SLI | Objective | Current release evidence |
| --- | --- | --- |
| Availability | 99.95% monthly | Not measured over a production window. |
| Ingestion latency | p95 < 100 ms | Smoke p95 7.83 ms; compact capacity p95 4.62 s under saturation. |
| Tail latency | p99 < 250 ms | Smoke p99 8.14 ms; compact capacity exceeded the objective. |
| Accepted durability | Zero silent loss | Kafka acknowledgement is explicit; full recovery reconciliation is not measured. |
| Duplicate effects | Zero duplicate effects | PostgreSQL idempotency and E2E duplicate path pass; full downstream reconciliation is not measured. |
| Error rate | < 1% in accepted load | Smoke passed; compact capacity measured 5.63% failures. |
| Backlog | No continuous growth at sustainable load | Baseline and compact capacity showed consumer pressure. |
| Webhook delivery | No silent loss | E2E HMAC/retry path passed; long-running recovery evidence is not measured. |

## PromQL examples

```promql
histogram_quantile(0.95, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events"}[5m])))
histogram_quantile(0.99, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events"}[5m])))
sum(rate(pulse_http_requests_total{route="/v1/events",status=~"5.."}[5m]))
/
sum(rate(pulse_http_requests_total{route="/v1/events"}[5m]))
```

The Compose Prometheus setup scrapes every 15 seconds and retains seven days.
An alert must link to a runbook, include the query window and dependency state,
and identify whether event reconciliation is required.
