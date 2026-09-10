# SLI and SLO

The values below are service objectives from the project plan, not observed
measurements. Each measured value remains `PENDING` until it is captured from
the stated query and test window.

## Objectives

| SLI | SLO / target | Measurement | Current evidence |
| --- | --- | --- | --- |
| Availability | 99.95% monthly for the production API | Successful ready/serving requests over eligible time | PENDING |
| Ingestion latency | p95 < 100 ms | `pulse_http_request_duration_seconds` for `POST /v1/events`, excluding rejected requests only if documented | PENDING |
| Ingestion tail latency | p99 < 250 ms | Same histogram and window as p95 | PENDING |
| Accepted event durability | 0 silent loss | Reconcile `202` event IDs with Kafka and downstream stores after recovery tests | PENDING |
| Duplicate effects | 0 | Duplicate delivery and persistence reconciliation using `(tenant_id,event_id)` | PENDING |
| Error rate | < 1% in load profiles | k6 failed checks and HTTP failures | PENDING |
| Backlog | No continuous growth at sustainable load | Kafka consumer lag and queue depth time series | PENDING |
| Webhook delivery | Scenario-specific; no silent loss | Delivery, retry, and DLQ counters plus receiver log | PENDING |

## Query examples

Metric names are exposed by the Go telemetry package. Adapt labels to the
Prometheus deployment before recording results:

```promql
histogram_quantile(0.95, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events"}[5m])))
histogram_quantile(0.99, sum by (le) (rate(pulse_http_request_duration_seconds_bucket{route="/v1/events"}[5m])))
sum(rate(pulse_http_requests_total{route="/v1/events",status=~"5.."}[5m]))
/
sum(rate(pulse_http_requests_total{route="/v1/events"}[5m]))
```

The exact label values and retention must be recorded with each result. The
Compose Prometheus configuration uses a 15-second scrape interval and 7-day
storage retention.

## Error budget

For a 30-day month, a 99.95% availability objective permits approximately
21.6 minutes of unavailable time. This is a planning conversion, not an
observed outage budget. Production policy, alert windows, exclusions, and
ownership are `PENDING`.

## Alert/runbook linkage

An alert should link to the relevant runbook and include the query window,
affected tenant or route, dependency state, and whether data reconciliation
is required. Use [`docs/runbooks/README.md`](../runbooks/README.md) for the
initial response sequence.
