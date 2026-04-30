# 0004: Secret Storage Boundaries

Status: accepted for MVP

Kronos supports two MVP secret-handling paths:

- Control-plane state can encrypt sensitive target/storage option values when
  `server.master_passphrase` is configured.
- Target and storage options can use full-value environment or file
  placeholders that worker agents resolve at execution time.

This is a bounded implementation, not a full secrets-management system.

## Rationale

The current approach avoids introducing a hard dependency on a specific external
secret manager while still reducing the highest-risk plaintext persistence
cases:

- Small self-hosted deployments can run with one encrypted `state.db` and a
  backed-up passphrase.
- Kubernetes and VM deployments can inject worker-side credentials through
  environment variables, mounted files, External Secrets Operator, or systemd
  environment files.
- Database tool-wrapper drivers can receive credentials without placing
  passwords directly in process-visible command arguments.

The tradeoff is operational responsibility. Deployments must protect the master
passphrase, environment variables, mounted files, temporary tool config files,
and the worker host itself.

## Requirements

- Sensitive target/storage option values should be encrypted before persisting
  to `state.db` when `server.master_passphrase` is configured.
- Placeholder syntax must be validated before accepting target/storage
  resources.
- Worker agents must resolve placeholders at execution time, so secrets do not
  need to be stored in the control plane.
- Logs and CLI output must redact secret-like values.
- Release docs must not claim host-level exposure is eliminated while external
  database tools are used.

## Decision Gate

For v0.1/MVP:

- Support passphrase-backed state encryption and worker-side placeholders.
- Require deployment docs to call out passphrase backup and host-level secret
  exposure.
- Keep first-class integrations with cloud secret managers on the roadmap.

For broader production use:

- Add provider-native secret reference validation and rotation workflows.
- Add tests that prove sensitive values remain redacted in state, logs, CLI
  output, audit metadata, and job evidence.
