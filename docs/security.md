# Security Considerations

This document describes controls visible in the current repository and checks
that still require deployment-specific verification. It is not a security
certification.

## Implemented controls

- Tenant API keys are generated with cryptographic randomness and stored as
  SHA-256 hashes; plaintext keys are returned only when created or rotated.
- Constant-time comparison is used by the in-memory authentication path.
- Event tenant identity is derived from the authenticated credential, not a
  client-provided event field.
- Event JSON uses strict decoding, validation, payload limits, and timestamp
  checks.
- PostgreSQL statements use parameters and enforce unique tenant/event IDs.
- Redis rate limiting uses an atomic Lua script and a per-tenant key.
- Webhook requests use HMAC-SHA256 signatures and timestamp/event headers.
- Webhook URLs are restricted to HTTP(S), reject user info and known/private
  address classes, and use a safe dialer that rechecks resolved addresses.
- HTTP server timeouts and webhook client timeout bound request duration.
- Helm defaults request non-root execution, drop Linux capabilities, disable
  privilege escalation, and use a read-only root filesystem.

## Production requirements

- Terminate TLS for client, Kafka, database, Redis, ClickHouse, and webhook
  traffic where the deployment requires it; Compose uses local plaintext and
  is not a production security profile.
- Replace all `change-me-*` values with an external secret mechanism. Never
  commit `.env` files, admin tokens, API keys, webhook secrets, or credentials.
- Restrict the admin token path to an operator-controlled network and rotate it
  under an access-controlled process.
- Apply least-privilege database, Kafka, Redis, ClickHouse, Kubernetes, and
  observability permissions.
- Configure network policies and egress controls in Kubernetes; SSRF checks do
  not replace network-layer egress restrictions.
- Define API-key rotation/revocation operations and audit logging for the
  production control plane.
- Validate container image provenance, dependency advisories, TLS settings,
  and non-root behavior with the target runtime.
- Confirm sensitive fields are absent from logs, traces, metric labels, and
  error responses.

## Verification checklist

| Check | Command/evidence | Status |
| --- | --- | --- |
| Go tests and race detector | `make test`, `make race` | PENDING |
| Static analysis | `make lint`, `make vet` | PENDING |
| Dependency vulnerabilities | `make scan` | PENDING |
| Compose and Helm validation | `make validate` | PENDING |
| API key hash and rotation | unit/integration output | PENDING |
| SSRF and HMAC tests | unit/E2E output | PENDING |
| Secret scan and image scan | CI/security tooling | PENDING |
| TLS, network policy, RBAC | target deployment evidence | PENDING |
