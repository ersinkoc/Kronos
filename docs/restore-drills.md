# Restore Drills

Backups are only production-ready after restore paths are rehearsed. Use this
checklist after release upgrades, storage migrations, key rotations, retention
policy changes, and new target onboarding.

## Drill Cadence

| Cadence | Scope | Goal |
| --- | --- | --- |
| Weekly | One recent backup per critical storage backend | Confirm credentials, manifests, chunks, and agent execution still work. |
| Monthly | One representative target per driver | Confirm operator runbooks and restore timing remain accurate. |
| Before major changes | A backup affected by the change | Catch key, storage, driver, or retention mistakes before rollout. |
| After incidents | The exact affected target class | Prove the recovery path that would have mattered. |

## Preflight

```bash
./bin/kronos ready
./bin/kronos backup list --since 7d
./bin/kronos storage test --uri <storage-uri>
./bin/kronos agent list
```

Pick a backup with a known target, storage, public signing key, and chunk
decryption key. If the backup is protected, keep it protected until the drill is
closed and documented.

## Manifest And Chunk Verification

```bash
./bin/kronos backup verify \
  --manifest-key <manifest-key> \
  --level manifest \
  --public-key <ed25519-public-key-hex> \
  --storage-local <repo-path>

./bin/kronos backup verify \
  --manifest-key <manifest-key> \
  --level chunk \
  --public-key <ed25519-public-key-hex> \
  --chunk-key <32-byte-decryption-key-hex> \
  --storage-local <repo-path>
```

Record verification duration, manifest key, backup ID, storage ID, and the
build version from `./bin/kronos version`.

## Restore Preview

```bash
./bin/kronos restore preview \
  --backup-id <backup-id> \
  --target-id <target-id> \
  --replace-existing=false
```

The preview should show the expected restore chain and should not require
unexpected parent backups. Stop here if the plan is surprising.

## Isolated Restore

Run restore drills against a disposable target or sandbox namespace. Never point
a drill at production data unless this is an actual incident response.

```bash
./bin/kronos restore start \
  --backup-id <backup-id> \
  --target-id <sandbox-target-id> \
  --dry-run=false \
  --replace-existing=false

./bin/kronos jobs list --operation restore
./bin/kronos jobs inspect --id <restore-job-id>
```

After the job completes, validate the restored database with application-level
checks, row/key counts, and at least one representative read query.

## Closeout

- Save the backup ID, manifest key, target ID, storage ID, restore job ID,
  Kronos version, operator, start time, finish time, and result.
- Note any manual step that was unclear, slow, or missing from the runbook.
- Confirm backup freshness alerts remained quiet or were intentionally muted.
- Remove disposable restore targets and temporary credentials.
- Keep the drill record with the same retention policy as other operational
  change evidence.
