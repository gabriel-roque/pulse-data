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

## Explicit release boundaries

- The 100k/s target is not achieved by the compact profile.
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
```
