# Troubleshooting

This guide covers common production-support failures for the implemented Kronos
paths.

## Token Scopes

Symptoms:

- API returns `403 Forbidden`.
- CLI commands fail after token rotation.
- Agent heartbeat or job claim requests stop succeeding.

Checks:

- Verify the token is not revoked or expired with `kronos token list`.
- Confirm the token includes the exact endpoint scope, a matching
  `resource:*` scope, `admin:*`, or `*`.
- Confirm the user role still allows the requested scope. Kronos caps token
  creation to the current user's role.
- Check for auth throttling in `kronos_auth_rate_limited_total`.

Fixes:

- Reissue a token with the smallest required scope set.
- Rotate agent tokens after changing role or scope policy.
- Keep local/no-token mode on loopback only; do not use it to bypass missing
  production scopes.

## Agent Claims

Symptoms:

- Jobs remain queued.
- Jobs move to failed with `server_lost`.
- An agent appears stale in `/api/v1/agents`.

Checks:

- Confirm the agent can reach the control plane URL and TLS CA/cert/key paths.
- Confirm the agent token can call heartbeat, claim, and finish endpoints.
- Inspect labels and capacity in the heartbeat payload; schedules or jobs may
  require labels that no live agent advertises.
- Check clock skew if heartbeat timestamps look old.
- Review control-plane logs for request IDs returned to the agent.

Fixes:

- Restart the agent after correcting token, TLS, or label configuration.
- Increase capacity only after confirming the worker host has enough CPU, disk,
  memory, and database-client concurrency budget.
- Retry failed jobs only after confirming the previous failure was transient.

## Storage Credentials

Symptoms:

- Backup jobs fail before writing chunks.
- Restore previews find metadata but restore jobs cannot read objects.
- S3-compatible repositories return authorization, region, or endpoint errors.

Checks:

- Confirm target/storage option placeholders resolve on the agent, not only on
  the control plane.
- Confirm `server.master_passphrase` is configured before relying on encrypted
  sensitive options in `state.db`.
- Verify local repository paths exist and are writable by the agent user.
- Verify S3 endpoint, bucket, region, path-style setting, access key, and secret
  key against the actual object store.
- Check whether temporary disk is large enough for S3 upload spooling.

Fixes:

- Prefer `*-ref` CLI flags or full-value placeholders for credentials.
- Rotate object-store credentials and update the storage resource if a secret
  was exposed.
- Run a small backup/restore drill after changing repository credentials.

## Restore Failures

Symptoms:

- Restore preview succeeds but live restore fails.
- Live restore refuses to replace an existing target.
- Verification jobs report missing or corrupted chunks.

Checks:

- Confirm the backup chain contains every parent required by the restore plan.
- Confirm `replace_existing=true` is intentional for non-dry-run restores that
  overwrite a target.
- Confirm matching database client tools are installed on the claiming agent:
  `psql`/`pg_dump`, `mysql`/`mysqldump` or MariaDB equivalents, or
  `mongodump`/`mongorestore`.
- Inspect `/api/v1/jobs/{id}/evidence` for restore reports or failure evidence.
- Verify the repository still contains every referenced manifest and chunk.

Fixes:

- Start with dry-run restore queueing, then promote to live restore after the
  plan matches the intended target and timestamp.
- Re-run verification for the selected backup before live restore.
- Restore into a disposable target first when changing database versions,
  client-tool versions, or repository credentials.
- Do not delete retained parent backups unless retention planning confirms the
  child chains remain restorable.
