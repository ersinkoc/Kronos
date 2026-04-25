<p align="center">
  <img src="docs/assets/kronos-banner.png" alt="Kronos" width="720" />
</p>

<h1 align="center">Kronos</h1>

<p align="center">
  <strong>Time devours. Kronos preserves.</strong><br/>
  <em>Scheduled, encrypted, verified database backups for PostgreSQL, MySQL, MongoDB, and Redis — in one Go binary.</em>
</p>

<p align="center">
  <a href="#"><img src="https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go" /></a>
  <a href="#"><img src="https://img.shields.io/badge/license-Apache%202.0-blue" /></a>
  <a href="#"><img src="https://img.shields.io/badge/deps-stdlib%20%2B%202-B87333" /></a>
  <a href="#"><img src="https://img.shields.io/badge/binary-%3C%2045%20MB-D4A963" /></a>
  <a href="#"><img src="https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20windows-0B0B12" /></a>
</p>

---

Kronos is a **single, zero-dependency Go binary** that performs scheduled, encrypted, deduplicated, verified backups with point-in-time recovery across **PostgreSQL, MySQL/MariaDB, MongoDB, and Redis** — with a modern WebUI, a full CLI, an MCP server, and REST + gRPC APIs. It speaks the database wire protocols directly: **no `pg_dump`, `mysqldump`, or `mongodump` required on any host**.

> The `0 3 * * * pg_dump | gzip | aws s3 cp` in your crontab silently failed last Tuesday. Kronos is what you install instead.

## Why Kronos

- **Four databases, one binary.** PostgreSQL 14–17, MySQL 8.0/8.4, MariaDB 10.11/11.4, MongoDB 6–8, Redis 7/8 + Valkey.
- **Native wire protocols.** No shelling out. No version drift between your DB and the backup tool.
- **Real PITR.** Streaming WAL / binlog / oplog / AOF, replay to a timestamp, LSN, GTID, or opcode.
- **Client-side encryption.** AES-256-GCM or ChaCha20-Poly1305. Your storage backend sees ciphertext only.
- **Content-defined deduplication.** FastCDC chunking + BLAKE3. Daily fulls cost what daily incrementals cost elsewhere.
- **Verified restores.** Schedule sandbox restores that actually apply the backup to a throwaway DB and run your checks. The feature every competitor leaves to you.
- **Zero dependencies.** `golang.org/x/crypto`, `golang.org/x/sys`, `gopkg.in/yaml.v3`, `klauspost/compress`, `blake3`. That's it. No `aws-sdk-go`. No ORMs. No SQLite.
- **Single binary.** < 45 MB stripped. `CGO_ENABLED=0`. Runs on distroless.
- **Control plane + agent fleet.** Agents live next to your databases; server orchestrates. Agents dial out — NAT-friendly.
- **WebUI. CLI. REST. gRPC. MCP.** All five, all first-class.

## Quick Start (5 minutes)

```bash
# 1. Install
brew install kronosbackup/kronos/kronos           # macOS
curl -fsSL https://get.kronos.dev | sh            # Linux

# 2. Bootstrap a single-node deployment
kronos local init --data-dir ~/.kronos

# 3. Add a target
kronos target add prod-pg \
    --driver postgres \
    --host 127.0.0.1 --port 5432 \
    --user kronos --password-file ~/.kronos/pg.pw \
    --database myapp

# 4. Add a storage
kronos storage add primary-s3 \
    --backend s3 \
    --bucket my-kronos-backups \
    --region eu-north-1 \
    --credentials-env AWS

# 5. Schedule nightly backups
kronos schedule add prod-pg-nightly \
    --target prod-pg \
    --storage primary-s3 \
    --type full \
    --cron "0 2 * * *" \
    --retention gfs:daily=7,weekly=4,monthly=12

# 6. Start the server+agent (one process for single-host ops)
kronos local

# 7. Open the WebUI
open http://localhost:8500
```

Your first backup will run at 02:00 local time. To trigger one now:

```bash
kronos backup now --target prod-pg
```

To restore to a specific moment:

```bash
kronos restore latest --target prod-pg --at "2026-04-23T14:37:00Z"
```

## How It Works (30 seconds)

```
 kronos-server   ◄───── gRPC/mTLS ─────►   kronos-agent   ───►   your DB
     (control)                               (executor)           (wire
                                                                   protocol)
         ▲                                      │
         │                                      ▼
         └── WebUI / CLI / MCP                 streams encrypted chunks
                                                to Storage Backend
                                                (S3, Azure, GCS, SFTP,
                                                 local, ... )
```

1. You declare **targets** (databases), **storages** (buckets), **schedules** (crons), and **retentions** (GFS rules).
2. The **server** orchestrates. The **agent** (co-located with the DB) speaks the wire protocol and streams records.
3. Records are **chunked** (FastCDC), **hashed** (BLAKE3), **deduplicated**, **compressed** (zstd), **encrypted** (AES-256-GCM), and **uploaded** in parallel.
4. A signed, versioned **manifest** is committed last. Until the manifest exists, nothing is visible in the repo.
5. **Streams** (WAL/binlog/oplog/AOF) run continuously between fulls, giving you PITR.
6. **Retention** walks the chain and evicts obsolete manifests; **GC** reclaims unreferenced chunks.
7. **Verification** periodically restores into a sandbox and runs your checks. If the restore fails, you get paged — not your users.

## Features

### Databases (core 4 at v0.1)

| Database | Versions | Logical | Physical | PITR | Incremental |
|----------|----------|---------|----------|------|-------------|
| PostgreSQL | 14, 15, 16, 17 | ✅ | ✅ | ✅ WAL | ✅ block |
| MySQL | 8.0, 8.4 | ✅ | 🟡 v0.2 | ✅ binlog (GTID) | ✅ chunk |
| MariaDB | 10.11, 11.4 | ✅ | 🟡 v0.2 | ✅ binlog (GTID) | ✅ chunk |
| MongoDB | 6, 7, 8 | ✅ | — | ✅ oplog | ✅ chunk |
| Redis / Valkey | 7, 8 / 7.2+ | ✅ | ✅ RDB | ✅ AOF stream | ✅ chunk |

### Storage backends

Local • S3 (+ MinIO, R2, B2, Wasabi, Ceph RGW) • Azure Blob • Google Cloud Storage • SFTP • FTP/FTPS • WebDAV *(v0.2)*.

### Scheduling & Retention

5- and 6-field cron • friendly `@daily`, `@hourly`, `@every 15m`, `@between 02:00-04:00 random` • one-off and event-triggered jobs • **GFS** (grandfather-father-son) • count-based • time-based • size-capped • manual legal hold.

### Security

Client-side **AES-256-GCM** or **ChaCha20-Poly1305** • Argon2id KDF • age-style sealed repositories • multi-slot unlock (LUKS-style) • key rotation without chunk re-encryption • mTLS server↔agent • TOTP-mandatory admin 2FA • OIDC (Google, Keycloak, Authentik, Auth0, GitHub) • scoped API tokens • hash-chained tamper-evident audit log.

### Verification levels

1. **Manifest check** (every run).  
2. **Chunk integrity** (scheduled; BLAKE3 recompute).  
3. **Logical replay to `/dev/null`** (weekly).  
4. **Sandbox live restore** (monthly; actual DB comes up, your SQL checks run).

### Interfaces

- **CLI** — full administrative surface, `--output json|yaml|table`, shell completion for bash/zsh/fish.
- **WebUI** — dashboard, targets, backups explorer, schedules, restore wizard, audit log, settings. React 19 + Tailwind v4.1 + shadcn/ui + lucide-react, dark/light/system theme, responsive from 360 px phones to 1920 px desktops. Pre-built and embedded via `go:embed` — no Node runtime in the shipped binary.
- **REST API** — OpenAPI 3.1, cursor-paginated.
- **gRPC** — streaming job events for CLI & agent.
- **MCP server** — tools for LLMs: list targets/backups, trigger, preview restore, repo usage.
- **Prometheus** `/metrics`, **OpenTelemetry** OTLP traces, structured **slog** JSON logs.

## Comparison

| | **Kronos** | pgBackRest | Barman | WAL-G | `pg_dump` + cron | Velero |
|---|---|---|---|---|---|---|
| Single binary | ✅ | ❌ (Perl) | ❌ (Python) | ✅ | ❌ | ✅ |
| All-of-PG/MySQL/Mongo/Redis | ✅ | PG only | PG only | PG/MySQL/SQL Srv | varies | k8s only |
| Native wire protocol | ✅ | needs `pg_*` | needs `pg_*` | needs `pg_*` | needs `pg_dump` | uses plugin-per-DB |
| PITR | ✅ | ✅ | ✅ | ✅ | ❌ | depends |
| Client-side encryption | ✅ | 🟡 | 🟡 | ✅ | manual | limited |
| Deduplication | ✅ chunk | ❌ | ❌ | ❌ | ❌ | ❌ |
| Verified sandbox restore | ✅ | manual | manual | manual | ❌ | manual |
| WebUI | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| MCP server | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Zero-dependency | ✅ | ❌ | ❌ | ✅ | n/a | ❌ |

## Architecture

```
                +---------------------------+
                |  CLI / WebUI / MCP /      |
                |  REST / gRPC              |
                +--------------+------------+
                               │
                               ▼
+----------------+    +------------------+    +-------------------+
| Secret Stores  |◄───┤  kronos-server   ├───►|  Notifications    |
| vault, age,    |    |  (control plane) |    |  slack, email,    |
| env, k8s ...   |    +---------+--------+    |  pagerduty ...    |
+----------------+              │             +-------------------+
                                │ gRPC + mTLS
      ┌──────────────────┬──────┴───────┬──────────────────┐
      ▼                  ▼              ▼                  ▼
+------------+     +------------+    +------------+    +------------+
| agent #1   |     | agent #2   |    | agent #3   |    | agent #N   |
| PostgreSQL |     | MySQL      |    | MongoDB    |    | Redis      |
+------+-----+     +------+-----+    +------+-----+    +------+-----+
       │                  │                 │                 │
       └── streams encrypted, deduplicated chunks to ──► Storage
           (S3 / Azure / GCS / SFTP / Local / ...)
```

- **kronos-server** keeps scheduler state, job history, audit log, and config in a single embedded B+Tree file. No external DB.
- **kronos-agent** is stateless; it can be killed and restarted; interrupted backups resume from the last-acknowledged chunk.
- **Agents dial the server** by default (NAT-friendly). Server-initiated mode is also supported.
- **Chunks go directly from agent to storage**; never relayed through the server.

See [IMPLEMENTATION.md](./IMPLEMENTATION.md) for the full architecture.

## Installation

### Package managers

```bash
brew install kronosbackup/kronos/kronos          # macOS, Linux (brew)
scoop bucket add kronos https://github.com/kronosbackup/scoop
scoop install kronos                             # Windows (scoop)

# Debian / Ubuntu
curl -fsSL https://pkg.kronos.dev/key | sudo tee /etc/apt/keyrings/kronos.asc
echo "deb [signed-by=/etc/apt/keyrings/kronos.asc] https://pkg.kronos.dev/apt stable main" | sudo tee /etc/apt/sources.list.d/kronos.list
sudo apt update && sudo apt install kronos

# RHEL / Fedora
sudo dnf install https://pkg.kronos.dev/rpm/kronos-latest.rpm
```

### Container

```bash
docker run --rm -v $PWD/kronos.yaml:/etc/kronos/config.yaml \
    ghcr.io/kronosbackup/kronos:latest server
```

### Kubernetes (Helm)

```bash
helm repo add kronos https://charts.kronos.dev
helm install kronos kronos/kronos -f values.yaml
```

### From source

```bash
git clone https://github.com/kronosbackup/kronos.git
cd kronos
make build
./bin/kronos version
```

## Configuration

Minimal `kronos.yaml`:

```yaml
server:
  listen: "0.0.0.0:8500"
  data_dir: "/var/lib/kronos"
  master_passphrase: "${file:/run/secrets/kronos-master}"

projects:
  - name: default
    storages:
      - name: primary-s3
        backend: s3
        bucket: "acme-kronos-backups"
        region: "eu-north-1"
    targets:
      - name: prod-pg
        driver: postgres
        connection:
          host: "10.0.1.5"
          user: "kronos"
          password: "${vault:secret/kronos/pg#password}"
    schedules:
      - name: prod-pg-nightly
        target: prod-pg
        type: full
        cron: "0 2 * * *"
        storage: primary-s3
        retention: { gfs: {daily: 7, weekly: 4, monthly: 12} }
```

Full reference in [docs/CONFIGURATION.md](./docs/CONFIGURATION.md).

## Status

**Alpha.** Phase 0 (foundation) → Phase 1 (storage + crypto) → Phase 2 (drivers) → Phase 3 (orchestration) → Phase 4 (WebUI + CLI) → **v0.1.0 MVP**.

Track progress in [TASKS.md](./TASKS.md).

## Dependency Policy

Kronos follows **`#NOFORKANYMORE`**: we do not adopt dependencies that can be replaced by ~1 500 LOC of our own. Full allowed list:

- Go standard library
- `golang.org/x/{crypto, sys, net, term, sync, text}`
- `gopkg.in/yaml.v3`
- `github.com/klauspost/compress` (zstd)
- `lukechampine.com/blake3`
- `google.golang.org/grpc` (unavoidable for gRPC)

Explicit **no**: `aws-sdk-go`, `gorm`, `sqlx`, `cobra`, `viper`, `logrus`, `zap`, `gin`, `echo`, `fiber`, `robfig/cron`, `minio-go`, SQLite.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md).

TL;DR: `make lint test` passes, commits signed (cosign or GPG), PR description references a task ID from [TASKS.md](./TASKS.md) if applicable, new dependencies require explicit justification.

## License

Apache 2.0. See [LICENSE](./LICENSE).

## Acknowledgements

Kronos stands on the work of many projects it refuses to depend on at runtime: `pgBackRest`, `Barman`, `WAL-G`, `mydumper`, `Velero`, `Restic`, `BorgBackup`. Each informed the design of Kronos; none is a dependency.

---

<p align="center"><em>Kronos — the oldest operator on your team.</em></p>
