# Benchmarks

The benchmark target is 100,000 offered events/s. It is an input to the test,
not a measured guarantee. Every run must record the offered rate, achieved
rate, p50/p95/p99, error rate, dropped iterations, consumer lag, backlog
behavior, CPU, memory, and the tested resource profile.

## Profiles

| Profile | Workload | Current status |
| --- | --- | --- |
| Smoke | 10 req/s for 1 minute | Measured: 600 requests, p95 7.83 ms, p99 8.14 ms, 0% errors. |
| Baseline | 1,000 req/s for 5 minutes | Measured ingestion path; downstream backlog remained. |
| Progression | 1k to 100k req/s | Not run in the current compact release. |
| Spike | Abrupt rise to 10k req/s | Not run in the current compact release. |
| Stress | Up to 100k req/s, 5-minute hold | Not run as a full-duration test. |
| Soak | 1k req/s for 1 hour | Not run because backlog was already growing. |
| Capacity | 100k req/s with compact Compose limits | Short probe measured ~910 req/s, p95 4.62 s, 5.63% failures. |

The short capacity probe is saturation evidence, not a sustainable-rate or
100k/s acceptance. Its resource samples and k6 summary are written to the
ignored `artifacts/capacity/<run-id>/` directory.

## Run

```sh
make quick-start
make load-smoke
make capacity-test
```

For a full default run, `make capacity-test` uses 128 tenants, a `100000/1s`
tenant limiter, a five-stage ramp, a five-minute hold, and the Compose compact
budget. Short probes can override `CAPACITY_STAGE_DURATION` and
`CAPACITY_HOLD_DURATION`.

Thresholds are intentionally strict: error rate below 1%, p95 below 250 ms,
p99 below 1 s, and successful checks above 99%. A saturation failure is a
valid result when the evidence records where the system stopped scaling.
