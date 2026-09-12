# Validation Snapshot

This snapshot describes the current portfolio baseline. It separates passing
checks from capacity limits and does not claim that the compact profile
sustains 100,000 events/s.

## Checks

| Area | Result |
| --- | --- |
| Go build, tests, vet, race | PASS |
| Compose, Helm, Prometheus validation | PASS |
| Quick start and E2E path | PASS in the previous baseline run |
| API, Kafka, PostgreSQL, ClickHouse and webhook wiring | PASS in integration/E2E coverage |
| Analytics UI and same-origin Query API proxy | PASS: page served, invalid key rejected, valid query returned 200 |
| Security dependency scan | PASS in the previous baseline run |
| Compact capacity profile startup | PASS: services healthy within ~2 vCPU/~3 GiB limits |

## Capacity probe

The short probe used `CAPACITY_STAGE_DURATION=3s`,
`CAPACITY_HOLD_DURATION=8s`, 16 tenants, and an offered target of 100,000
req/s. It recorded:

| Signal | Result |
| --- | --- |
| Achieved HTTP rate | ~910 req/s |
| p95 latency | 4.62 s |
| HTTP failures | 5.63% |
| Dropped iterations | 1,178,740 |
| Ingestion memory | 176.2 MiB / 192 MiB |
| Kafka memory | 407.8 MiB / 512 MiB |
| ClickHouse memory | 428.7 MiB / 512 MiB |
| Outcome | Compact profile saturated below the target. |

This is a deliberately short saturation probe. It proves that the test and
resource sampling work; it is not a claim of maximum sustainable throughput.
Raw artifacts are created locally under `artifacts/capacity/` and are ignored
by Git because they can contain generated API keys and large k6 streams.

## Expanded profile boundary

To identify the actual boundary, a fresh-volume single-node profile was tested
with approximately 10.7 vCPU and 12 GiB of service limits, 128 tenants, a
100,000/second limiter, and 5-second ramp stages plus a 30-second hold. The
SLO was less than 1% errors, p95 below 250 ms, p99 below 1 second, and at least
99% successful durability checks.

| Offered rate | Durable HTTP result | p95 | p99 | Outcome |
| ---: | ---: | ---: | ---: | --- |
| 2,300 req/s | 2,300 req/s, 0% errors | 205.6 ms | 276.1 ms | PASS |
| 2,400 req/s | 2,400 req/s, 0% errors | 277.3 ms | 337.3 ms | FAIL: p95 |

Therefore, the measured ingestion boundary for this profile is **2,300
req/s**, not 100,000 req/s. This is the Kafka durability boundary: `202` means
the event was acknowledged by Kafka, not that PostgreSQL and ClickHouse have
finished processing it.

The 2,400 req/s run left 94,488 analytics and 89,968 persistence messages in
Kafka immediately after the load. After 30 seconds, lag was still 79,611 and
70,217 respectively, implying drain rates of approximately 496 and 658
events/s. The current single-worker-per-group topology therefore cannot process
100,000 events/s end to end; batching, horizontal workers, database capacity,
and partition scaling are required before that target is meaningful.

## Scalable lab result

The capacity lab uses a fresh-volume profile with 500 events per HTTP batch,
48 Kafka partitions, four persistence workers, four analytics workers, four
webhook workers, PostgreSQL `COPY` for unique batches, batched ClickHouse
writes, and shared webhook subscription lookups. Redis rate limiting is
disabled for this throughput-only profile and is tested separately.

The previous run offered `100,000 events/s` as `200 batch requests/s` and
accepted every HTTP batch, with batch p95 `699 ms`, p99 `1.05 s`, and `0%` HTTP
failures. Its aggregate rate was lower because it included earlier ramp
stages, and its downstream snapshots still contained persistence lag. It is
therefore not release evidence for a sustainable end-to-end result.

The latest five-minute hold reached `99,885.8 events/s` at the Kafka durability
boundary, with `61,987` batches, `30,993,500` accepted events, zero dropped
iterations, p95 `646.76 ms`, p99 `1.37 s`, and zero HTTP failures. The
downstream drain gate did not pass: persistence and analytics lag remained
material after load stopped. This validates the 100k batched ingress path, not
100k end-to-end processing.

The capacity script now uses a direct constant-rate hold, fresh lab volumes,
exact batch-count checks, and a dropped-iteration threshold. `make capacity-lab`
uses the `ingress` gate and reports downstream lag/reconciliation separately;
the optional `PULSE_CAPACITY_GATE=end-to-end` gate additionally requires
consecutive zero-lag snapshots and matching PostgreSQL/ClickHouse counts. The
single-event endpoint remains bounded by the earlier `2,300 req/s`
durable-ingestion result.

## Explicit release boundaries

- The 100k/s target is not achieved by the compact profile.
- The expanded single-node profile also does not achieve 100k/s end to end;
  its measured durable-ingestion boundary is 2,300 req/s and its observed
  downstream drain rate is below 700 events/s per worker group.
- The scalable batch lab validates 100k events/s at the Kafka durability
  boundary only with the explicit batch endpoint and throughput profile above;
  downstream processing is asynchronous and was not sustainable at that rate
  in this local profile.
- Full-duration progression, spike, stress, and soak runs are not part of this
  release evidence.
- Production TLS, external secrets, network policy, RBAC, image provenance,
  Alertmanager, log collection, and OTLP collector wiring require the target
  deployment.
- Live Kubernetes HPA behavior and consumer-lag scaling were not evaluated.
- Webhook crash fencing and per-subscription durable retry remain architectural
  follow-ups described in [`docs/CODE_REVIEW.md`](CODE_REVIEW.md).

## Reproduce

```sh
make quick-start
make test
make race
make validate
make capacity-test
make capacity-lab
```
