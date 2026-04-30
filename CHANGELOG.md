# Changelog

All notable user-facing changes are recorded here.

This project does not have a public tagged release yet. Until the first release
tag is cut, entries are grouped under `Unreleased` and should move into a
versioned section during release preparation.

## Unreleased

### Added

- Kubernetes managed-cluster overlays for EKS, GKE, and AKS.
- Immutable Kubernetes image digest overlay guidance.
- Systemd unit examples and environment templates for bare-metal and VM
  deployments.
- Upgrade rollback procedure for binary and control-plane `state.db` changes.
- Importable Grafana overview dashboard for Kronos Prometheus metrics.
- Release signing preflight, signed-tag verification, release evidence archive
  validation, and consumer-side release rehearsal helpers.
- Release artifact SBOM module coverage and `govulncheck` vulnerability gate.
- PostgreSQL 15-to-17 and 16-to-17 restore rehearsals, PostgreSQL full global
  restore rehearsal, and PostgreSQL operator-scale restore drill coverage.
- MongoDB replica-set oplog archive/replay rehearsal.
- Backup hook execution and e2e coverage for pre-backup and failure hooks.
- Retention-chain edge case coverage for protected backups and parent chains.

### Changed

- Public README and project specification now distinguish implemented MVP scope
  from long-term pure-Go/native-driver product vision.
- PostgreSQL, MySQL/MariaDB, and MongoDB support is documented as an
  external-tool MVP requiring matching client tools on worker agents.

### Fixed

- Release verification evidence now includes archived tag-signature output when
  `KRONOS_RELEASE_TAG` is set.
- Release evidence verification can require GitHub provenance/SBOM attestation
  logs for production promotion records.
