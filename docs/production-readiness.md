# Production Readiness

Last reviewed from the repository state on April 27, 2026.

Kronos is close to production-ready for the implemented Redis or Valkey backup
path with local or S3-compatible storage. It is not yet production-ready for
the full product vision across PostgreSQL, MySQL, MongoDB, SFTP, Azure Blob,
Google Cloud Storage, deeper WebUI workflows, and multi-instance control-plane
operation.

## Readiness Estimate

| Scope | Estimate | Notes |
| --- | ---: | --- |
| Implemented Redis/local/S3 path | 85% | Core pipeline, agent/server flow, restore planning, retention, audit, metrics, release scripts, Kubernetes examples, and runbooks are in place. |
| Broad multi-database product vision | 65% | The architecture is strong, but major drivers, storage backends, WebUI workflows, and multi-instance deployment patterns remain roadmap work. |
| Current repository release hygiene | 80% | Tests, vet, format checks, OpenAPI checks, release artifacts, provenance, SBOM metadata, and the production check script are present. Dependency vulnerability review still needs to be cleared before a real release. |

## Current Release Gate

Use this command before a release candidate:

```bash
GO=.tools/go/bin/go ./scripts/production-check.sh
```

The gate checks formatting, runs `go vet`, runs the full Go test suite, builds
the binary, validates shell scripts, validates bash completion syntax, and
executes `kronos version`.

## Production-Ready Strengths

- Core streaming backup pipeline with chunking, compression, encryption
  envelopes, signed manifests, restore planning, and verification.
- Redis/Valkey driver coverage with backup and restore paths.
- Local and S3-compatible storage backends.
- Persistent control plane state, scheduler state, jobs, backups, retention,
  notifications, users, tokens, and audit log.
- Scoped bearer tokens, role-capped token creation, token lifecycle operations,
  request IDs, security headers, and mutation audit events.
- Health, readiness, metrics, OpenAPI, operations docs, deployment topology
  docs, restore drill docs, release scripts, provenance metadata, SBOM
  metadata, and Kubernetes examples.

## Blocking Work Before Calling The Whole Product Production-Ready

1. Clear dependency security alerts reported by GitHub Dependabot before
   cutting a release.
2. Add at least one more first-class database driver, starting with PostgreSQL
   or MySQL, plus backup and restore conformance tests.
3. Add end-to-end tests that run server, worker agent, storage backend, and
   target driver together.
4. Expand the WebUI from dashboard shell into live resource CRUD, job detail,
   backup detail, and restore workflows.
5. Decide the supported multi-instance story for control-plane state, or
   document single-replica constraints as a hard production boundary.
6. Sign or attest release provenance and SBOM metadata in CI.

## Next Engineering Slices

1. Dependency security cleanup and vulnerability gate.
2. End-to-end Redis backup/restore scenario that starts a control plane and
   worker agent in the same test.
3. PostgreSQL driver MVP with schema/data backup and restore smoke tests.
4. WebUI live API wiring for overview, jobs, backups, agents, and readiness.
5. Production deployment hardening for single-replica Kubernetes and external
   secret management.
