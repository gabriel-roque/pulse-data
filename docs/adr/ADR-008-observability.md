# ADR-008: Observability
## Status
Accepted as the local baseline; production collectors and trace coverage are outside this release.
## Contexto
The platform has multiple asynchronous boundaries, so request success alone cannot show processing, lag, delivery, or loss behavior.
## Drivers
- Metrics for latency, throughput, failures, lag, workers, and queues.
- Correlatable logs and traces across HTTP, Kafka, stores, and webhooks.
- Dashboards usable during incidents.
## Alternativas consideradas
- Logs only.
- Metrics only.
- Metrics, dashboards, logs, and distributed tracing.
## Decisão
Expose Prometheus metrics from HTTP services, provision Prometheus/Grafana/Loki/Tempo in Compose, and document the HTTP-to-Kafka-to-store/webhook trace path. Required metric families include request latency, event outcomes, publish latency, webhook outcomes, consumer lag, worker count, and queue depth.
## Consequências positivas
- Asynchronous backlog and downstream failures can be observed separately from API health.
- The stack is reproducible locally.
## Consequências negativas
- Labels such as tenant require cardinality and privacy controls.
- Dashboard and trace correctness must be verified against live traffic.
## Evidências / benchmarks
Implementation evidence: `internal/telemetry/metrics.go` and `observability/`. Prometheus and Grafana configuration validate; end-to-end log collector and trace export require deployment-specific wiring.
