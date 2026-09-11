# ADR-009: Autoscaling
## Status
Accepted as a Helm baseline; measurement-based tuning is a production follow-up.
## Contexto
HTTP services and workers have different scaling signals. Kafka consumers are constrained by topic partitions, while CPU alone may not track backlog.
## Drivers
- Horizontal availability for stateless HTTP services.
- Scale workers without exceeding useful partition parallelism.
- Keep disruption bounded with PDB and rolling update settings.
## Alternativas consideradas
- Fixed replica counts.
- CPU-based HPA for all components.
- Component-specific HPA with partition and lag-aware operational review.
## Decisão
Provide Helm HPA defaults for ingestion, query-api, persistence-worker, analytics-worker, and webhook-worker, with component-specific replica bounds and a 70% CPU target. Treat these as starting values; tune from measured throughput, CPU/event, memory, lag, and partition count. Do not scale a consumer group beyond useful partitions without evidence.
## Consequências positivas
- The chart has a repeatable horizontal scaling baseline and PDB/rolling-update controls.
- Tuning is explicitly tied to capacity evidence rather than the 100k/s target.
## Consequências negativas
- CPU-only HPA can react late to Kafka backlog or external webhook latency.
- HPA and PDB behavior must be tested in a real Kubernetes environment.
## Evidências / benchmarks
Implementation evidence: `deployments/helm/pulse/values.yaml` and HPA template. Helm rendering passes; live Kubernetes HPA and lag response were not evaluated.
