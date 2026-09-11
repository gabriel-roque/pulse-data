# Pulse

Pulse is a multi-tenant event platform written in Go. It accepts HTTP events,
publishes them to Kafka, and applies independent effects to PostgreSQL,
ClickHouse, and signed webhooks.

![Pulse event platform architecture](docs/architecture/pulse-architecture.png)

The editable diagram sources are
[`pulse-architecture.svg`](docs/architecture/pulse-architecture.svg) and
[`diagram.mmd`](docs/architecture/diagram.mmd). The detailed component and
failure model is in [`docs/architecture/overview.md`](docs/architecture/overview.md).

## Status

This repository contains an implemented baseline, not proof of the 100,000
events/s target. The current evidence and known limits are in
[`docs/FINAL_VALIDATION_REPORT.md`](docs/FINAL_VALIDATION_REPORT.md).

Core semantics:

- `202 Accepted` is returned only after a synchronous Kafka publish with all required acknowledgements.
- Consumers are at-least-once and commit offsets after successful handling.
- PostgreSQL deduplicates `(tenant_id, event_id)`; webhook claims deduplicate each subscription delivery.
- The platform is eventually consistent and does not promise end-to-end exactly-once effects.

## Quick start

Requirements: Docker Compose v2, `curl`, and `jq`.

```sh
./scripts/quick-start.sh
```

The script creates `.env` when absent, builds the image, starts the complete
local Compose profile, waits for readiness, and runs the real E2E flow. It
leaves the stack running.

After startup, open the analytics UI at
[`http://127.0.0.1:3001`](http://127.0.0.1:3001). Enter a tenant API key,
choose a time range, and run a tenant-scoped ClickHouse summary query.

```sh
make down   # stop containers
make clean  # stop and remove containers and volumes
```

Local ports bind to `127.0.0.1`. Compose credentials are development defaults;
replace them before using a shared environment.

## Commands

| Command | Purpose |
| --- | --- |
| `make quick-start` | Start the local stack and run E2E validation. |
| `make up` / `make down` | Start or stop Compose without removing volumes. |
| `make test` | Run Go tests. |
| `make lint` | Run formatting and `go vet`. |
| `make race` | Run tests with the race detector. |
| `make integration` | Run integration coverage against Compose. |
| `make validate` | Validate Compose, Helm, and Prometheus configuration. |
| `make scan` | Run `govulncheck ./...`. |
| `./scripts/stress-test.sh` | Run the k6 stress profile. |
| `make capacity-test` | Run the 100k/s scenario with resource sampling. |

The stress script requires k6, waits for ingestion readiness, and provisions
multiple local tenants when no API key is supplied. It ramps to 100,000
requests/s; a failure at saturation is evidence, not a successful capacity
claim. Configure `PULSE_API_URL`, `PULSE_API_KEY` or `PULSE_API_KEYS`, and
`PULSE_STRESS_TENANTS` as needed.

For a reproducible capacity run under the compact Compose budget, use:

```sh
make capacity-test
```

The scenario raises the rate limit to `100000/1s`, provisions 128 tenants by
default to distribute Kafka keys, samples every service's CPU and memory, and
writes evidence under `artifacts/capacity/`. The compact budget is a resource
containment profile of about 2 vCPU and 3 GiB; it is not expected to sustain
100k/s until a measured run proves otherwise.

## API

The HTTP contract is [`docs/api/openapi.yaml`](docs/api/openapi.yaml). The main
routes are:

- `POST /v1/events` — authenticate, validate, rate-limit, and publish an event.
- `POST /v1/tenants` — create a tenant with the operator admin token.
- `POST /v1/tenants/{id}/rotate` — rotate a tenant API key.
- `POST /v1/webhooks` — register a signed webhook endpoint.
- `GET /v1/analytics/summary` — query tenant-scoped ClickHouse summaries.
- `GET /health/live` and `GET /health/ready` — process and dependency probes.

The browser UI uses the analytics route through its same-origin proxy; it does
not connect directly to ClickHouse.

## Documentation map

| Topic | Document |
| --- | --- |
| Architecture and failure boundaries | [`docs/architecture/overview.md`](docs/architecture/overview.md) |
| Technology and resource rationale | [`docs/resources.md`](docs/resources.md) |
| Architecture decisions | [`docs/adr/`](docs/adr/) |
| Security controls and production requirements | [`docs/security.md`](docs/security.md) |
| Operations and incident actions | [`docs/runbooks/README.md`](docs/runbooks/README.md) |
| Troubleshooting | [`docs/troubleshooting.md`](docs/troubleshooting.md) |
| SLI/SLO definitions | [`docs/operations/sli-slo.md`](docs/operations/sli-slo.md) |
| Load profiles and result template | [`docs/benchmarks/README.md`](docs/benchmarks/README.md) |
| Capacity planning | [`docs/capacity-planning.md`](docs/capacity-planning.md) |
| Chaos test notes | [`docs/chaos-report.md`](docs/chaos-report.md) |
| Code review and open risks | [`docs/CODE_REVIEW.md`](docs/CODE_REVIEW.md) |

The executable test suites have their own safety gates and usage notes in
`tests/integration/README.md`, `tests/e2e/README.md`, `tests/load/README.md`,
and `tests/chaos/README.md`.

## Validation policy

Compilation is not treated as production validation. The repository is a
measured local baseline: use the validation snapshot and capacity artifacts to
understand what passed, where the compact profile saturated, and which
production requirements belong to the target deployment.
