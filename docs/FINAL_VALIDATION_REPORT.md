# Final Validation Report

This report records the local validation evidence. Results below are not a
claim that the 100,000 events/s target was achieved.

| Field | Result/evidence |
| --- | --- |
| Commit tested | `55e9d98` (dependency/security fix; final code rebuild) |
| Date and timezone | 2026-09-09/10, America/Sao_Paulo (UTC-03) |
| Environment/hardware | Linux amd64, 12 vCPU, 31 GiB RAM, Docker Engine 29.8.0 |
| Versions | Go 1.25.13, Compose 5.5.1, Kafka 3.9.0, PostgreSQL 17, Redis 7.4, ClickHouse 25.3, k6 2.2.0 |
| Build | PASS: `go build ./cmd/...`; clean Compose image build PASS |
| Lint | PASS: `make lint` |
| Unit | PASS: `make test` |
| Race | PASS: `make race` |
| Integration | PASS: real API -> Kafka -> PostgreSQL/ClickHouse duplicate test |
| E2E | PASS: tenant -> webhook -> duplicate event -> persistence/analytics/HMAC receiver |
| Security | PASS: `govulncheck ./...`; Trivy image HIGH/CRITICAL total 0 after grpc 1.83.2 |
| Load | PASS smoke: 600 requests, 10 req/s, 0% errors, p95 7.83 ms, p99 8.14 ms |
| Stress | NOT EXECUTED: default profile would add sustained backlog after measured consumer saturation |
| Spike | NOT EXECUTED: same bounded local environment limitation; profile exists |
| Soak | NOT EXECUTED: one-hour run is not evidence-compatible while backlog is growing |
| Chaos | PASS recovery-only scenarios: ingestion, worker, Kafka, PostgreSQL and Redis; business reconciliation gap documented |
| Maximum sustainable throughput | NOT ESTABLISHED; 1,000 req/s ingestion accepted, but full-effect consumers lagged. Observed four-worker drain interval was about 583 events/s |
| p50/p95/p99 | Smoke: 7.22/7.83/8.14 ms. Distributed baseline: 5.82/12.89/16.86 ms |
| Error rate | Smoke and distributed baseline: 0%; default-limit baseline: 98% intentional 429s |
| Consumer lag | Single-tenant baseline: persistence 143,874 and analytics 218,759; distributed baseline immediate: 186,198 and 231,221; after 4 replicas/2 min: 88,124 and 169,202 |
| Problems found | Kafka default 1 s batching; default benchmark rate-limit mismatch; hot tenant partition; DB/ClickHouse consumer saturation; missing webhook crash-safe claim lifecycle; missing worker metrics endpoint; vulnerable dependencies |
| Corrections made | Explicit 5 ms Kafka batching; token bucket; leased webhook claims; per-endpoint breakers; W3C/OTLP tracing; worker metrics; real E2E profile; dependency/toolchain upgrades |
| Limitations | Full-effect sustainable rate, 100k/s, stress/spike/soak and Kubernetes/HPA runtime were not proven on this host |
| Criteria not met | 100k/s target; maximum sustainable throughput; stress/spike/soak evidence; full chaos reconciliation; live Kubernetes/HPA validation |
| Conclusion | IMPLEMENTED BASELINE, NOT FULLY VALIDATED against every global acceptance criterion |

## Validation commands

```sh
make clean
make up
make lint
make test
make race
make integration
make e2e
make load-smoke
make validate
```

The smoke and baseline commands were executed. Record the separate load profile commands from
[`docs/benchmarks/README.md`](benchmarks/README.md) and the gated chaos
commands from [`docs/chaos-report.md`](chaos-report.md). Failures and limits
are recorded above; no criterion was hidden.
