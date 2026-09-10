# Code Review

## Scope

Review of the Go services, persistence and messaging boundaries, webhook
delivery, Compose and Helm deployment assets, CI, observability, test harness,
load scripts, and documentation. The repository is a backend event platform;
there is no frontend application or UI design-system package to review.

## Executive assessment

The codebase has a solid distributed-systems baseline: explicit delivery
semantics, tenant-scoped authentication, Kafka consumer groups, idempotent
PostgreSQL persistence, Redis rate limiting, webhook HMAC/SSRF controls,
structured validation, health probes, and executable integration/E2E/load tests.

It is not yet production-ready for the stated 100,000 events/s target. The
local Compose topology is deliberately single-node, the maximum sustainable
throughput is not established, and several production concerns remain outside
the current evidence. The architecture is better described as a well-defined
baseline with an explicit validation backlog than as a finished platform.

## Changes applied in this review

- Added dependency-aware readiness checks for PostgreSQL, Kafka, Redis, and
  ClickHouse, with bounded probe timeouts.
- Added bounded retry/backoff for consumer fetch and processing failures;
  malformed Kafka events are validated and routed to the DLQ.
- Removed tenant IDs and raw request paths from Prometheus labels to prevent
  unbounded cardinality and accidental tenant disclosure.
- Counted duplicate PostgreSQL deliveries in aggregate metrics.
- Extended webhook claims to one hour so the lease covers the configured retry
  schedule, and process all subscriptions before returning an event-level DLQ
  error.
- Propagated request/delivery context through SSRF DNS resolution and closed
  ClickHouse connections during lifecycle shutdown.
- Added tenant-name and analytics-window limits, strict single-document JSON
  handling, restart policies, localhost-only Compose ports, `.gitignore`, and
  `.dockerignore`.
- Pinned the `govulncheck` CI version and enabled the repository's `master`
  push branch in CI.
- Added `scripts/quick-start.sh` for full local startup plus E2E validation and
  `scripts/stress-test.sh` for the standalone k6 stress profile.
- Replaced the architecture image's gradients and shadows with flat RGB color
  fills. `pulse-architecture.png` is now 8-bit RGB, not RGBA.

## Findings that remain

### High

- **Webhook retry durability is still event-level, not delivery-level.** A
  failed subscription is released and the source event is sent to the Kafka
  DLQ after the handler has attempted all subscriptions. A production design
  should model each delivery as an independently retryable state machine or
  publish an outbox/delivery command per subscription. The current claim is a
  useful duplicate guard, but it is not a complete durable retry queue.
- **Claim completion has no fencing token or affected-row contract.** The
  one-hour lease covers the configured schedule, but a worker that outlives a
  lease can still race a new claimant. Add a claim version/token and require
  `RowsAffected() == 1` for completion and release.
- **Observability ingestion is incomplete in Compose.** Tempo is configured,
  but no `PULSE_OTEL_ENDPOINT` is wired by default; Loki has no Promtail,
  Alloy, or Fluent Bit log collector; and there is no Alertmanager. The
  instrumentation exists, but end-to-end traces, logs, and alerts are not
  proven by the stack.
- **Secrets and image provenance are still development-grade.** Compose and
  Helm retain local fallback credentials, image tags are mutable, and CI
  actions are not pinned by immutable commit SHA. Shared or production
  environments need external secrets, image digests/signature verification,
  SBOM/provenance, and TLS/network policy.

### Medium

- **Worker autoscaling uses CPU instead of Kafka lag.** Production consumers
  should scale from lag/arrival rate with a KEDA or external-metrics policy,
  bounded by useful partition parallelism.
- **Worker metrics are not represented as Kubernetes Services or
  ServiceMonitors.** The process exposes `:9091`, but the Helm chart does not
  make those metrics discoverable as a first-class target.
- **CI does not execute every external suite.** DLQ, rate-limit, shutdown,
  chaos, and k6 profiles remain separately gated. Add jobs with explicit
  environment and destructive-test gates, and upload logs/metrics as artifacts.
- **The load profile needs capacity-aware configuration.** The stress wrapper
  provisions multiple tenants, but a local default of 1,000 requests/s per
  tenant cannot demonstrate 100,000 requests/s without a deliberately higher
  rate-limit setting and enough Kafka partitions/consumers.
- **Analytics needs a stronger query budget.** The API now limits the window
  to 31 days and applies a 10-second timeout, but ClickHouse resource quotas,
  query concurrency limits, and a dedicated analytics rate limit are still
  needed for hostile or highly concurrent tenants.

### Low

- The architecture documentation should be updated whenever the delivery
  state machine changes; the Mermaid source and generated SVG/PNG should be
  regenerated together.
- The current project has no frontend, component library, or visual design
  system. If a UI is added, introduce tokens for color, spacing, typography,
  motion, focus states, and data visualization rather than styling screens ad
  hoc.

## Recommended target architecture

1. Keep the current ports-and-adapters direction, but make application use
   cases explicit and keep Kafka, PostgreSQL, Redis, ClickHouse, and HTTP in
   adapters. This makes failure and contract tests independent of vendor
   clients.
2. Use an inbox/outbox model for durable effects. Kafka consumption, inbox
   deduplication, and effect commands should be committed with explicit state
   transitions; external webhooks should be driven by per-subscription
   delivery rows with attempts, leases, fencing, and next-attempt timestamps.
3. Use bounded concurrency and backpressure at every boundary. Scale consumers
   from lag, not only CPU, and document the partition/consumer ceiling.
4. Standardize telemetry around low-cardinality metrics, W3C trace context,
   JSON logs, an OTLP collector, and alert rules tied to the SLOs.
5. Make deployment security explicit: external secrets, immutable image
   references, signed artifacts, network policies, least-privilege service
   accounts, TLS, and admission checks.

## Verification performed

- `go test ./...`
- `go vet ./...`
- `go test -race ./...`
- `docker compose config --quiet`
- `helm lint` and `helm template`
- Shell syntax validation for the operational scripts
- `./scripts/quick-start.sh`, including full Compose startup and real E2E
- PNG inspection: `1600x940`, 8-bit/color RGB, non-interlaced

The stress profile was intentionally not executed as part of this review. Its
five-stage ramp reaches 100,000 requests/s and must be run on a prepared,
instrumented environment; its result must be recorded as evidence rather than
treated as a pass merely because the script starts.
