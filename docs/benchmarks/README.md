# Benchmarks

No benchmark result is asserted in this repository. The target of 100,000
events/s is a test input and acceptance goal, not a measured throughput.

## Profiles

| Profile | Workload | Purpose | Result |
| --- | --- | --- | --- |
| Smoke | 10 req/s for 1 minute | Fast wiring check | PASS: 600 events, p95 7.83 ms, p99 8.14 ms, 0% errors |
| Baseline | 1,000 req/s for 5 minutes | Reference behavior | PASS ingestion: 300,002 events, p95 12.89 ms, p99 16.86 ms, 0% errors; consumer backlog remained |
| Progression | 1k, 5k, 10k, 25k, 50k, 75k, 100k req/s, 2 minutes each | Find degradation and sustainable ceiling | NOT EXECUTED: bounded host already showed consumer saturation |
| Spike | Abrupt increase to 10k req/s | Recovery and shedding behavior | NOT EXECUTED: bounded host limitation |
| Stress | Up to 100k req/s with a 5-minute final stage | Find saturation | NOT EXECUTED: bounded host limitation |
| Soak | 1k req/s for 1 hour by default | Detect slow leaks and backlog growth | NOT EXECUTED: bounded host limitation |

Profiles live in `tests/load/` and are executed through `scripts/run-k6.sh`.
The default k6 thresholds are error rate below 1%, p95 below 250 ms, p99 below
1 s, and checks above 99%. A saturation profile may fail thresholds; that
failure is evidence for the maximum sustainable throughput.

## Required test record

Copy this table for each profile. Do not fill values from configuration,
targets, or an unrecorded dashboard screenshot.

| Field | Value |
| --- | --- |
| Profile and script | PENDING |
| Commit | PENDING |
| Date/time and timezone | PENDING |
| Client and server hardware | PENDING |
| OS, Go, Docker, Kafka, k6 versions | PENDING |
| Event size distribution | PENDING |
 | Tenants and partition-key distribution | PENDING |
| Offered rate | PENDING |
| Achieved throughput | PENDING |
| p50 / p95 / p99 | PENDING |
| HTTP error rate and k6 checks | PENDING |
| CPU, memory, GC, network | PENDING |
| Kafka lag by group | PENDING |
| PostgreSQL pool and latency | PENDING |
| Redis latency and rejections | PENDING |
| ClickHouse latency | PENDING |
| Backlog growing continuously? | PENDING |
| Maximum sustainable? | PENDING |
| Artifacts/logs/dashboard links | PENDING |
| Interpretation and follow-up | PENDING |

## Run commands

```sh
make up
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh smoke
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh baseline
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh progression
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh stress
PULSE_API_URL=http://127.0.0.1:8080 PULSE_API_KEY=... scripts/run-k6.sh soak
```

For `make load-smoke`, set `LOAD_TEST` if the default smoke profile is not the
desired test. Record why any threshold or soak override was used.
