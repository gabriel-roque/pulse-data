# Final Validation Report

This report is intentionally a template. Replace `PENDING` only with captured
command output, metrics, or reproducible artifact references. A target value
from the project plan is not a result.

| Field | Result/evidence |
| --- | --- |
| Commit tested | PENDING |
| Date and timezone | PENDING |
| Environment/hardware | PENDING |
| Versions | PENDING |
| Build | PENDING |
| Lint | PENDING |
| Unit | PENDING |
| Race | PENDING |
| Integration | PENDING |
| E2E | PENDING |
| Security | PENDING |
| Load | PENDING |
| Stress | PENDING |
| Spike | PENDING |
| Soak | PENDING |
| Chaos | PENDING |
| Maximum sustainable throughput | PENDING |
| p50/p95/p99 | PENDING |
| Error rate | PENDING |
| Consumer lag | PENDING |
| Problems found | PENDING |
| Corrections made | PENDING |
| Limitations | PENDING |
| Criteria not met | PENDING |
| Conclusion | NOT VALIDATED until this report is completed |

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

Record the separate load profile commands from
[`docs/benchmarks/README.md`](benchmarks/README.md) and the gated chaos
commands from [`docs/chaos-report.md`](chaos-report.md). Include failures and
their reruns; do not hide a failed criterion.
