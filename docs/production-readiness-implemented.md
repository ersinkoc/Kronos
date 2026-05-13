# Implemented-Path Production Readiness

Last reviewed from the repository state on May 13, 2026.

This track covers what the current repository can honestly support: the Kronos
control plane, CLI, polling agent, local/S3-compatible/SFTP/Azure Blob/GCS
repositories, and native database protocol drivers for Redis, PostgreSQL,
MySQL, and MongoDB.

## Ready

- Core backup objects are chunked, compressed, encrypted, hashed, and recorded
  in signed manifests.
- Local filesystem, S3-compatible, SFTP, Azure Blob, and Google Cloud Storage
  backends are implemented and covered by conformance and fault-injection tests.
- Redis/Valkey native driver with PSYNC streaming and AOF replay for PITR.
- PostgreSQL native driver using pgwire protocol with COPY BINARY bulk export
  and LSN-based incremental backup support.
- MySQL native driver using custom wire protocol handshake and SELECT-based
  backup with SHOW CREATE TABLE.
- MongoDB native driver using OP_MSG wire protocol with BSON encoding/decoding,
  SCRAM-SHA-256 authentication, and change stream / oplog capture for PITR.
- Fault injection test suite for storage backends covering latency, corruption,
  disconnect, and transient error scenarios.
- API integration tests for job and token lifecycle operations.
- The control plane persists agents, jobs, backups, schedules, retention
  policies, users, tokens, notification rules, and audit events.
- Request IDs, security headers, scoped bearer tokens, token lifecycle
  operations, token verification rate limiting, TLS/mTLS configuration, and
  optional state secret encryption are implemented.
- Prometheus metrics, health/readiness probes, operations docs, deployment
  topology docs, Kubernetes single-replica examples, systemd units, release
  verification docs, SBOM checks, release smoke checks, and a reusable
  production gate exist.
- CI exercises formatting, vet, staticcheck, govulncheck, unit tests with race
  detection, tagged E2E tests, release artifact checks, container builds,
  and database conformance/restore rehearsal jobs.

## Required Operating Boundaries

- Run the control plane as a single replica with one authoritative `state.db`.
- Keep local/no-token mode bound to loopback only.
- Configure TLS for networked control-plane access.
- Configure `server.master_passphrase` before storing sensitive target or
  storage option values in the control-plane state DB.
- Treat external-tool driver credentials as host-sensitive because environment
  variables and temporary config files remain observable to sufficiently
  privileged local users.
- Validate restore drills against representative data before promoting a target
  family to production.
- gRPC agent communication requires protoc binary for code generation;
  HTTP fallback is available when gRPC is blocked.

## Release Gate

Run before a release candidate:

```bash
GO=.tools/go/bin/go GOFMT=.tools/go/bin/gofmt ./scripts/production-check.sh
.tools/go/bin/go test -tags=e2e ./cmd/kronos
```

Release promotion also requires signed tag verification and archived evidence
for checksums, signatures, SBOM/provenance, and smoke-test output.

## Not Ready In This Track

- HA control-plane operation.
- Automated state DB migration and rollback framework.
- gRPC agent transport (protocol definition exists, code generation pending).
- PostgreSQL WAL/PITR, MySQL binlog replay, or MongoDB oplog replay as
  first-class Kronos-managed chains (foundations exist, streaming not wired).
- Full browser-auth flow with password login, enforced TOTP, or OIDC.
- Frontend component/security/a11y test suite (Playwright).
- WebUI bundle size CI guard (< 200KB target).
- MinIO real-service conformance tests for S3 backend.
- Broad storage matrix real-service conformance beyond local, S3-compatible,
  SFTP, Azure Blob, and GCS backends.
