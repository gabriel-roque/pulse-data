# Technology and Resource Rationale

Pulse is a portfolio implementation of a multi-tenant event pipeline. Each
technology solves a specific boundary problem rather than serving as a
decorative dependency.

| Resource | What it does | Why it is used |
| --- | --- | --- |
| Go | Runs the HTTP APIs and workers | Small binaries, explicit concurrency, low runtime overhead, and strong tooling for services. |
| Kafka | Durable event log and fan-out boundary | Decouples ingestion from effects, preserves keyed ordering, exposes lag, and enables replay/DLQ workflows. |
| Redis | Distributed token bucket | Enforces one tenant rate limit across multiple API replicas with atomic Lua state. |
| PostgreSQL | Operational state | Enforces tenant, event, subscription, and idempotency constraints transactionally. |
| ClickHouse | Analytical event store | Handles columnar aggregation by time, tenant, and event type without competing with control-plane writes. |
| Persistence worker | Operational effect consumer | Converts Kafka events into idempotent PostgreSQL writes. |
| Analytics worker | Analytical effect consumer | Converts Kafka events into ClickHouse rows asynchronously. |
| Webhook worker | External effect consumer | Applies retries, timeout, circuit breaker, bulkhead, HMAC, claims, and SSRF checks before external delivery. |
| Query API | Tenant-scoped read boundary | Keeps ClickHouse credentials and query policy outside clients and the browser UI. |
| Analytics UI | Local portfolio interface | Makes ClickHouse summaries observable through a same-origin, tenant-authenticated workflow. |
| Prometheus | Metrics store | Makes throughput, latency, failures, retries, and resource signals queryable. |
| Grafana | Operational dashboards | Turns Prometheus data into a usable view during load and failure tests. |
| Loki and Tempo | Logs and traces backends | Provide the intended correlation layer for service diagnostics; full collectors/export wiring remain deployment concerns. |
| Docker Compose | Reproducible local topology | Starts the complete system with explicit CPU/memory containment and health dependencies. |
| Helm | Kubernetes packaging | Models replicas, probes, resource requests/limits, autoscaling, and security context. |
| k6 | Load generator | Exercises real HTTP durability behavior and records latency, errors, dropped iterations, and achieved rate. |

## Resource policy

The default Compose profile is a compact local budget of approximately 2 vCPU
and 3 GiB. This is a containment target for a developer machine, not a claim
of 100k events/s capacity. `make capacity-test` raises the tenant limiter,
offers 100k/s, samples every service, and records the first bottleneck.

The current short probe reached the 100k/s offered stage but achieved roughly
910 req/s with 4.62 s p95 and 5.63% HTTP failures. The repository therefore
demonstrates the architecture and the measurement discipline, not a false
claim that the compact profile sustains the target.

## UI design system

The analytics UI uses a small token-based system in `ui/styles.css`: navy and
slate for structure, teal for healthy/primary actions, orange for attention,
and red for errors. Panels, labels, controls, tables, bars, focus states, and
responsive breakpoints share those tokens instead of ad hoc screen styles.
The UI has no frontend build dependency, which keeps the portfolio quick to
run while preserving a clear visual language and mobile layout.
