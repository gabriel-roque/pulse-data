# Review Summary

Pulse is an implemented distributed-systems portfolio baseline. It shows
tenant authentication, Kafka fan-out, idempotent persistence, analytical
storage, rate limiting, webhook resilience, observability, a tenant-scoped
analytics UI, Docker Compose, Helm, and executable validation scripts.

## Verified in the repository

- Go tests, `go vet`, formatting, and race detector pass.
- Compose, Helm, and Prometheus configuration validate.
- Quick start runs the local stack and E2E tenant/event/webhook flow.
- Ingestion returns `202` only after Kafka acknowledgement.
- PostgreSQL deduplicates `(tenant_id, event_id)`.
- Webhooks have HMAC, SSRF checks, timeout, retry, circuit breaker, bulkhead,
  and delivery claims.
- The compact Compose budget is explicit and sampled by `make capacity-test`.
- The analytics UI queries ClickHouse only through the authenticated Query API.

## Capacity conclusion

The compact profile is approximately 2 vCPU and 3 GiB. A short 100k/s offered
probe achieved approximately 910 req/s with p95 4.62 s and 5.63% HTTP failures.
The profile is suitable for a bounded demonstration environment, not for the
100k/s target. Scaling the target requires more CPU/memory, Kafka partitions,
worker replicas, and database capacity.

## Production follow-ups

- Model webhook retries per subscription with durable outbox/state-machine
  records and fencing tokens.
- Wire an OTLP collector, log collector, and alert manager in the deployment.
- Scale consumers from Kafka lag, not only CPU, and validate HPA in Kubernetes.
- Replace local credentials with external secrets, TLS, network policy, RBAC,
  immutable images, signatures, and SBOM/provenance.
- Add ClickHouse query quotas and an analytics-specific rate limit.

These are intentionally labelled as production follow-ups rather than hidden
behind empty acceptance fields. The repository's current implementation and
measured limits are summarized in [`FINAL_VALIDATION_REPORT.md`](FINAL_VALIDATION_REPORT.md).
