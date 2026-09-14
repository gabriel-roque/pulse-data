# SLI and SLO

These are the production objectives and the queries used to evaluate them.
The capacity test validates the Kafka ingress boundary, not downstream effects.

| SLI | Objective | Current release evidence |
| --- | --- | --- |
| Availability | 99.95% monthly | Not measured over a production window. |
| Ingestion latency | p95 < 1 s at 100k events/s | Formal batch run p95 43.83 ms. |
| Tail latency | p99 < 2 s at 100k events/s | Formal batch run p99 76.73 ms. |
| Accepted durability | Zero silent loss at ingress | Accepted event count is reconciled with Kafka offsets. |
| Duplicate effects | Zero duplicate effects | PostgreSQL idempotency and E2E duplicate path pass; full downstream reconciliation is not measured. |
| Error rate | 0% in the capacity test | Formal batch run had zero HTTP failures. |
| Backlog | Diagnostic only for ingress target | Downstream capacity is outside the 100k ingress claim. |
| Webhook delivery | No silent loss | E2E HMAC/retry path passed; long-running recovery evidence is not measured. |

## PromQL examples

```promql
histogram_quantile(0.95, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events/batch"}[5m])))
histogram_quantile(0.99, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events/batch"}[5m])))
sum(rate(pulse_http_requests_total{route="/v1/events/batch",status=~"5.."}[5m]))
/
sum(rate(pulse_http_requests_total{route="/v1/events/batch"}[5m]))
```

The Compose Prometheus setup scrapes every 15 seconds and retains seven days.
An alert must link to a runbook, include the query window and dependency state,
and identify whether event reconciliation is required.
