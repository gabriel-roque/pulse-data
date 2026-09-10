# End-to-End Tests

The E2E test creates a tenant, creates a webhook, sends the same event twice,
and checks the webhook receiver and service dependencies. A duplicate final
webhook delivery is a failure, not an implicit retry success.

The receiver must be reachable from the `webhook-worker` container and must
resolve to a non-private address because the application enforces SSRF policy.
The included deterministic receiver can be exposed through a test network
endpoint:

```sh
python3 tests/e2e/webhook_mock.py --port 8090
```

Set `PULSE_E2E_WEBHOOK_URL`, `PULSE_E2E_WEBHOOK_STATUS_URL`,
`PULSE_E2E_WEBHOOK_RESET_URL`, and `PULSE_E2E_WEBHOOK_CONFIG_URL` to that
reachable endpoint. The config endpoint receives the generated secret and
`PULSE_E2E_WEBHOOK_FAIL_FIRST=N` makes the receiver return deterministic 500s
before accepting, proving retry behavior. The script fails explicitly when
these URLs or credentials are absent.
