# Kronos — TASKS

Ordered, atomic tasks. Each task:
- has an ID, a title, a clear deliverable, an acceptance criterion, and an estimated size.
- is ≤ 4 hours of focused work, or it's split further.
- is sequential within its phase but parallelisable across work-streams (marked).
- is explicit about which PR it should ship in.

**Work-streams** (parallel tracks):
- `CORE` — domain types, config, secrets, embedded KV, clock, IDs
- `DRV` — database drivers
- `STG` — storage backends
- `CRY` — crypto, chunking, compression
- `SCH` — scheduling, retention, hooks
- `SRV` — server/agent wiring, API surfaces
- `UI` — WebUI
- `OBS` — metrics, tracing, logs, audit
- `REL` — release engineering, CI, docs

Task sizes: `XS` ≤ 1h, `S` 1–2h, `M` 2–4h, `L` split into sub-tasks.

---

## Phase 0 — Foundation (Week 1)

Goal: Repo, CI, skeleton binary, shared utilities.

| ID | Stream | Title | Deliverable | Acceptance | Size |
|----|--------|-------|-------------|-----------|------|
| F-01 | REL | Initialise Go module | `go.mod` with module path `github.com/kronos/kronos`, Go 1.23. `go mod tidy` succeeds. | `go build ./...` produces no output. | XS |
| F-02 | REL | Licence, README stub, CONTRIBUTING | `LICENSE` (Apache 2.0), minimal `README.md` linking SPEC/IMPL/TASKS, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`. | Files present, `golint` clean. | XS |
| F-03 | REL | Makefile | Targets: `build`, `test`, `lint`, `fmt`, `vet`, `vuln`, `bench`, `integration`, `e2e`, `release`, `clean`. | `make help` prints target list; `make build` produces `./bin/kronos`. | S |
| F-04 | REL | GitHub Actions: lint + unit tests | `.github/workflows/ci.yml` — gofmt check, `go vet`, `staticcheck`, `govulncheck`, `go test -race ./...`. | PR workflow green on empty repo. | S |
| F-05 | CORE | `internal/core` package: IDs & Clock | UUIDv7 generator (stdlib crypto/rand), `Clock` interface, `RealClock`, `FakeClock` for tests. | 100% test coverage on `core/id` and `core/clock`. | S |
| F-06 | CORE | `internal/core/errors` | Typed errors: `ErrNotFound`, `ErrConflict`, `ErrAuth`, `ErrTransient`, with wrapping helpers. | Round-trip `errors.Is` tests pass. | XS |
| F-07 | CORE | Domain types | `Target`, `Storage`, `Schedule`, `Job`, `Backup`, `Manifest`, `RetentionPolicy`, `User`, `Role`, `Token`, `AuditEvent`. Pure data, no methods that do I/O. | `go doc` renders cleanly; JSON round-trip tests. | M |
| F-08 | CORE | `internal/config` | YAML load using `gopkg.in/yaml.v3`; JSON-Schema validation; env/file/vault placeholder expansion via `${scheme:path#field}` syntax. | Load-and-validate of a sample config file in `testdata/`. | M |
| F-09 | CORE | `internal/secret` secret resolver interface + env/file providers | `Resolver` interface; `env`, `file` providers built-in. | Unit tests resolve `${env:X}` and `${file:/tmp/x}`. | S |
| F-10 | CORE | Logger setup | `log/slog` wrapper with JSON handler; context-scoped correlation ID; secret redaction hook. | A log statement including a `password: hunter2` field emits `password: ***REDACTED***`. | S |
| F-11 | CORE | `cmd/kronos` dispatcher | Subcommand registry; `kronos server`, `kronos agent`, `kronos local`, `kronos help`, `kronos version` stubs. | `./bin/kronos version` prints build info. | S |
| F-12 | REL | `scripts/build.sh` | Cross-compile matrix, embed build metadata via `-ldflags`. | `./scripts/build.sh linux/amd64` produces stripped binary ≤ 10 MB (stub). | S |
| F-13 | REL | Bench scaffolding | `bench/` directory with a trivial benchmark and a Makefile target that runs it. | `make bench` produces `bench.out`. | XS |

Phase 0 exit criteria: green CI; skeleton binary builds on Linux + macOS; domain types, config loader, logger done.

---

## Phase 1 — Storage & Crypto Core (Weeks 2–3)

Goal: The pipeline that can take a `[]byte` stream and put it encrypted-and-deduplicated onto any supported backend.

### 1A — Storage backends

| ID | Stream | Title | Acceptance | Size |
|----|--------|-------|-----------|------|
| S-01 | STG | `storage.Backend` interface, `ObjectInfo`, `ListPage` types | Interface compiles; mock implementation for tests. | S |
| S-02 | STG | `storage/local` backend | Put/Get/Head/Exists/Delete/List with atomic rename on Put. `fsync` on file + parent dir. | Fuzz Put→Get round-trip for 10k objects. | M |
| S-03 | STG | `storage/local` concurrency tests | Parallel writers to same key fail cleanly; no partial files. | `go test -race` with 100 goroutines. | S |
| S-04 | STG | `storage/s3` — SigV4 signer | Canonical request, string-to-sign, signature. | Passes all vectors in `aws-sig-v4-test-suite`. | M |
| S-05 | STG | `storage/s3` — simple PUT/GET/DELETE/HEAD | Against MinIO in integration test. | All CRUD ops work; ETag matches. | M |
| S-06 | STG | `storage/s3` — multipart upload | Threshold 64 MiB, part size 16 MiB, up to 10k parts. Abort on error. | 2 GiB object upload and download verifies byte-identical. | M |
| S-07 | STG | `storage/s3` — retry middleware | Exponential backoff, jitter, respect `Retry-After`. | Injected 503s recover within 5 retries. | S |
| S-08 | STG | `storage/s3` — IAM auth modes | Static, STS, IMDSv2, EC2 instance profile, IRSA. | Each mode obtains credentials in mock environment. | M |
| S-09 | STG | `storage/s3` — compatibility matrix test | MinIO, R2, B2, Wasabi mock. Document quirks per backend. | Matrix test passes in CI. | S |
| S-10 | STG | `storage/sftp` — transport | `golang.org/x/crypto/ssh` connection management; auth: password, publickey, agent. | Connects to a local SFTP container. | M |
| S-11 | STG | `storage/sftp` — SFTP v3 packets | OPEN, CLOSE, READ, WRITE, STAT, REMOVE, RENAME, MKDIR, READDIR. | All CRUD ops against a real `openssh-sftp-server`. | M |
| S-12 | STG | `storage/ftp` — transport | `net/textproto`; PASV/EPSV; FTPS. | Against `vsftpd` container. | M |
| S-13 | STG | `storage/azure` — blob API | Block blob upload (put block + commit block list), GET, DELETE, HEAD, LIST. | Against Azurite emulator. | M |
| S-14 | STG | `storage/azure` — auth | Shared key + SAS + bearer token. | Each mode authenticates successfully. | S |
| S-15 | STG | `storage/gcs` — JSON API | Simple + resumable upload, GET, DELETE, HEAD, LIST. | Against fake-gcs-server emulator. | M |
| S-16 | STG | `storage/gcs` — auth | Service account JSON (JWT → token exchange), ADC. | JWT signing tests pass. | S |
| S-17 | STG | Backend-level integrity test | Any backend round-trips random bytes up to 2 GiB. | All backends pass. | S |
| S-18 | STG | Backend-level chaos test | Inject 1% network drops and 100 ms jitter; backend still completes all operations. | All backends pass within 2× runtime budget. | S |

### 1B — Crypto & chunking

| ID | Stream | Title | Acceptance | Size |
|----|--------|-------|-----------|------|
| C-01 | CRY | AEAD wrapper | AES-256-GCM and ChaCha20-Poly1305 behind a common `Cipher` interface. | Round-trip tests + known-answer tests. | S |
| C-02 | CRY | KDF: Argon2id | `DeriveKey(passphrase, salt) → 32 B` with recommended params. | Known-answer tests. | XS |
| C-03 | CRY | Key hierarchy & slots | Root key, per-backup DEK via HKDF, slot-file format with multiple passphrases. | Add/remove slot operations; lock/unlock cycle. | M |
| C-04 | CRY | age-style sealed repo | `age` recipient encryption mode for team deployments. | `age` private key unlocks a test repo. | M |
| C-05 | CRY | `chunk.FastCDC` | Implementation with min=512K, avg=2M, max=8M; deterministic gear table. | Determinism test: same input → same chunk boundaries. | M |
| C-06 | CRY | BLAKE3 vendor or vendor-alt | Pure-Go BLAKE3 with SIMD paths (if x86-64). | Known-answer vectors pass; ≥ 1 GB/s on dev machine. | S |
| C-07 | CRY | Compressors | zstd (via klauspost/compress), gzip (stdlib), lz4 (pure-Go). | Round-trip on 100 MB payloads. | S |
| C-08 | CRY | Adaptive compression | Sample first 1 MB; pick algo based on entropy. | Benchmark shows no regression on already-compressed input. | S |
| C-09 | CRY | Chunk envelope | Version byte + key_id + nonce + AEAD payload format. | Parse/serialise round-trip; bad version rejected. | S |
| C-10 | CRY | `chunk.Index` | In-memory dedup index with Bloom filter front; persisted to disk per repo. | 1 M inserts, 1 M lookups at < 50 µs p99. | M |
| C-11 | CRY | `chunk.Pipeline` | End-to-end: reader → chunk → hash → dedup → compress → encrypt → upload. | Streams 10 GB through with constant memory. | L → split into P1/P2/P3 |
| C-11a | CRY | Pipeline worker skeleton | Goroutine topology + bounded channels. | Unit test with fake stages. | S |
| C-11b | CRY | Pipeline real stages | Wire real chunker, hasher, cipher, backend. | End-to-end test on 1 GB input. | M |
| C-11c | CRY | Pipeline error handling | Cancel propagation, partial cleanup. | Chaos test: kill any worker, pipeline exits within 1s, `staging/` cleaned. | S |
| C-12 | CRY | Bench: pipeline throughput | Target: ≥ 400 MB/s on NVMe with zstd-3. | `make bench BENCH=Pipeline` passes. | S |

### 1C — Embedded KV Store

| ID | Stream | Title | Acceptance | Size |
|----|--------|-------|-----------|------|
| K-01 | CORE | `kvstore` page + pager | 4 KiB pages, mmap, free-list. | Alloc/free 100k pages without leak. | M |
| K-02 | CORE | `kvstore` B+Tree: read path | Lookup by key; iterator with range scan. | 1 M keys, lookup < 1 µs, scan < 50 ns/key. | M |
| K-03 | CORE | `kvstore` B+Tree: write path | Insert, delete, split, merge. | 1 M insert+delete with integrity check. | M |
| K-04 | CORE | `kvstore` WAL + fsync | Write-ahead log, recovery on open. | Kill -9 during a write loop; on restart, no data loss up to last successful commit. | M |
| K-05 | CORE | `kvstore` transactions | Single-writer, multi-reader MVCC via root swap. | Writer + 10 concurrent readers; readers see consistent snapshot. | M |
| K-06 | CORE | `kvstore` buckets | Nested key namespaces (like bbolt). | Bucket CRUD tests pass. | S |
| K-07 | CORE | `kvstore` bench | `BenchmarkKVRandomWrite`, `...Read`. Compare against bbolt baseline. | ≥ 80% of bbolt performance. | S |
| K-08 | CORE | `kvstore` repair | `kronos repair-db` walks pages, rebuilds free list. | Corrupt a page header; repair recovers. | M |
| K-09 | CORE | RISK-R1 decision | Either K-01..K-08 passes all above, or we swap to vendored bbolt. Document outcome in `docs/decisions/0001-kvstore.md`. | Decision recorded. | XS |

Phase 1 exit: pipeline reads a real file, emits encrypted chunks to local + S3 backends, and restoring them back produces the identical bytes. KV store passes chaos tests.

---

## Phase 2 — Drivers (Weeks 4–6)

Each driver is a parallel work-stream. Four engineers could take one each.

### 2A — PostgreSQL

| ID | Stream | Title | Size |
|----|--------|-------|------|
| D-PG-01 | DRV | Wire protocol: startup, auth (scram-sha-256, md5, password) | M |
| D-PG-02 | DRV | Wire protocol: simple query + extended query | M |
| D-PG-03 | DRV | `COPY TO STDOUT` with BINARY format | M |
| D-PG-04 | DRV | `COPY FROM STDIN` (restore) | M |
| D-PG-05 | DRV | Schema DDL extraction (tables, indexes, views, sequences) | M |
| D-PG-06 | DRV | Schema DDL extraction (functions, triggers, types, constraints) | M |
| D-PG-07 | DRV | Schema DDL extraction (roles, grants) | S |
| D-PG-08 | DRV | Consistent snapshot via `pg_export_snapshot` | S |
| D-PG-09 | DRV | Replication protocol: `IDENTIFY_SYSTEM`, slot create | S |
| D-PG-10 | DRV | Physical: `BASE_BACKUP` command + tar stream parser | M |
| D-PG-11 | DRV | WAL streaming (`START_REPLICATION`, `XLogData`, keepalive) | M |
| D-PG-12 | DRV | WAL replay to a target LSN / timestamp | M |
| D-PG-13 | DRV | Incremental (block-level, via WAL LSN range) | M |
| D-PG-14 | DRV | Table/schema include/exclude globs | S |
| D-PG-15 | DRV | Cleanup on cancel (drop replication slot) | S |
| D-PG-16 | DRV | CI integration: PG 14, 15, 16, 17 | S |
| D-PG-17 | DRV | End-to-end backup+restore test with fixture schema | M |
| D-PG-18 | DRV | PITR end-to-end test: backup, INSERT, record timestamp, INSERT, restore-to-timestamp, verify | M |

### 2B — MySQL / MariaDB

| ID | Stream | Title | Size |
|----|--------|-------|------|
| D-MY-01 | DRV | Wire protocol: handshake v10, auth (caching_sha2, native, ed25519) | M |
| D-MY-02 | DRV | `COM_QUERY`, result set parsing | M |
| D-MY-03 | DRV | Typed row decoder (all MySQL types incl. JSON, GEOMETRY, ENUM, SET) | M |
| D-MY-04 | DRV | Schema DDL via `SHOW CREATE` | M |
| D-MY-05 | DRV | Consistent snapshot (FTWRL + `START TRANSACTION WITH CONSISTENT SNAPSHOT`) | S |
| D-MY-06 | DRV | Chunked SELECT by PK range | M |
| D-MY-07 | DRV | Binlog: `COM_REGISTER_SLAVE` + `COM_BINLOG_DUMP_GTID` | M |
| D-MY-08 | DRV | Binlog parser: FORMAT_DESCRIPTION, GTID, TABLE_MAP, ROWS events | M |
| D-MY-09 | DRV | Binlog replay to GTID / timestamp | M |
| D-MY-10 | DRV | Incremental via GTID delta | M |
| D-MY-11 | DRV | Restore: DDL then data apply | M |
| D-MY-12 | DRV | CI matrix: MySQL 8.0, 8.4, MariaDB 10.11, 11.4 | S |
| D-MY-13 | DRV | PITR end-to-end test | M |

### 2C — MongoDB

| ID | Stream | Title | Size |
|----|--------|-------|------|
| D-MO-01 | DRV | OP_MSG framing | S |
| D-MO-02 | DRV | BSON encoder/decoder (all type tags) | M |
| D-MO-03 | DRV | `hello` / auth (SCRAM-SHA-256) | S |
| D-MO-04 | DRV | `listDatabases`, `listCollections`, `listIndexes` | S |
| D-MO-05 | DRV | `find` / `getMore` streaming with causal consistency | M |
| D-MO-06 | DRV | Replica-set snapshot via `atClusterTime` | M |
| D-MO-07 | DRV | Oplog tail via tailable cursor | M |
| D-MO-08 | DRV | Oplog replay to timestamp | M |
| D-MO-09 | DRV | Sharded cluster discovery and fan-out | M |
| D-MO-10 | DRV | Restore via `insert` batching + index rebuild | M |
| D-MO-11 | DRV | CI matrix: Mongo 6, 7, 8 (replica set + sharded) | S |
| D-MO-12 | DRV | PITR end-to-end test | M |

### 2D — Redis

| ID | Stream | Title | Size |
|----|--------|-------|------|
| D-RD-01 | DRV | RESP2 + RESP3 codec | M |
| D-RD-02 | DRV | `HELLO` + auth (password, ACL) | S |
| D-RD-03 | DRV | `SCAN` iteration + per-key `DUMP`/`RESTORE` | M |
| D-RD-04 | DRV | Replica protocol: `PSYNC ? -1` | M |
| D-RD-05 | DRV | RDB parser (all opcodes; string, list, hash, set, zset, stream, module stubs) | L → split by type family |
| D-RD-05a | DRV | RDB: string, list, hash | M |
| D-RD-05b | DRV | RDB: set, zset | M |
| D-RD-05c | DRV | RDB: stream, module, function (stub OK) | M |
| D-RD-06 | DRV | Continuous command capture for AOF-like PITR | M |
| D-RD-07 | DRV | Restore via `RESTORE` + pipeline | M |
| D-RD-08 | DRV | ACL snapshot and restore | S |
| D-RD-09 | DRV | CI matrix: Redis 7, 8, Valkey 7.2 | S |
| D-RD-10 | DRV | PITR end-to-end test | M |

Phase 2 exit: each driver can take a non-trivial fixture database, back it up to S3 and local, restore it, and the roundtrip passes a structural+content comparison.

---

## Phase 3 — Orchestration & API (Weeks 7–8)

| ID | Stream | Title | Acceptance | Size |
|----|--------|-------|-----------|------|
| O-01 | SRV | Server skeleton: config load, auth, shutdown | `kronos server --config c.yaml` starts and serves `/healthz`. | S |
| O-02 | SRV | Agent skeleton + gRPC `AgentConnect` client stream | Agent connects, heartbeats, appears in server's agent list. | M |
| O-03 | SRV | Orchestrator: dispatch a backup job to an agent | End-to-end "local PG → local filesystem" backup succeeds via server+agent. | M |
| O-04 | SRV | Job state persistence (queued/running/finalize/success/failed) | Kill server mid-job; restart; job shows `failed: server_lost`. Kill agent mid-job; server moves job to `failed: agent_lost`. | M |
| O-05 | SRV | Resume on agent crash (chunk staging) | Backup 10 GB; kill agent at 50%; restart; backup resumes and completes. | M |
| O-06 | SRV | Manifest commit atomicity | Kill before manifest write → chunks stay in staging; GC reclaims. | S |
| O-07 | SCH | Cron parser (5-field + 6-field + extensions) | 400-case test file passes. | M |
| O-08 | SCH | Scheduler tick loop | 10k schedules, tick in ≤ 5 ms. | S |
| O-09 | SCH | Per-target concurrency queue | Two schedules for same target run sequentially. | S |
| O-10 | SCH | Catch-up policies: skip, queue, run_once | Unit tests for each. | S |
| O-11 | SCH | Jitter + `@between random` | Statistical test over 1000 evaluations. | S |
| O-12 | SCH | `retention/gfs` | Simulation over 5 years; keep-set matches ground truth. | M |
| O-13 | SCH | `retention/count`, `retention/time`, `retention/size` | Each has unit tests. | S |
| O-14 | SCH | Retention resolver (combine rules → keep-set) | Combined policies idempotent. | S |
| O-15 | SCH | GC mark-and-sweep | Orphan chunks collected; protected backups never dropped. | M |
| O-16 | SRV | REST API handlers (targets, schedules, backups) | All endpoints return schema-correct JSON. OpenAPI spec generated. | L → split per resource |
| O-16a | SRV | REST /targets CRUD | — | M |
| O-16b | SRV | REST /schedules CRUD | — | M |
| O-16c | SRV | REST /backups list/inspect/protect | — | S |
| O-16d | SRV | REST /restore wizard endpoints | — | M |
| O-16e | SRV | REST /auth (login, refresh, logout) | — | M |
| O-17 | SRV | gRPC admin surface (`ListTargets`, `StartBackup`, `StreamJob`) | CLI can use either REST or gRPC. | M |
| O-18 | SRV | Auth: local users + bcrypt + TOTP 2FA | Login flow; TOTP mandatory for admin role. | M |
| O-19 | SRV | Auth: OIDC (Google, Keycloak, Authentik, Auth0, GitHub) | Works against Keycloak in integration tests. | M |
| O-20 | SRV | Auth: API tokens (scoped, expiring, revocable) | `kronos token create --scope backup:read` works. | S |
| O-21 | SRV | RBAC enforcement middleware | Operator cannot change storage config; viewer cannot trigger backup. | S |
| O-22 | SRV | MCP server | Tools listed in SPEC §3.16 functional; conformance test passes. | M |

Phase 3 exit: full end-to-end via WebUI's eventual consumption of these APIs; from scheduled trigger to manifest commit in storage, including retention and GC.

---

## Phase 4 — WebUI & CLI (Weeks 9–10)

### CLI

| ID | Stream | Title | Size |
|----|--------|-------|------|
| U-CLI-01 | UI | Dispatcher + global flags (`--server`, `--token`, `--output`) | S |
| U-CLI-02 | UI | `kronos target add/list/test/remove` | M |
| U-CLI-03 | UI | `kronos storage add/list/test/du` | M |
| U-CLI-04 | UI | `kronos schedule add/list/pause/resume/remove` | M |
| U-CLI-05 | UI | `kronos backup now/list/inspect/protect/verify` | M |
| U-CLI-06 | UI | `kronos restore` with wizard flags | M |
| U-CLI-07 | UI | `kronos retention plan/apply` | S |
| U-CLI-08 | UI | `kronos gc` | S |
| U-CLI-09 | UI | `kronos key rotate/add-slot/remove-slot/escrow` | M |
| U-CLI-10 | UI | `kronos user/token` admin commands | M |
| U-CLI-11 | UI | `kronos audit tail/search/verify` | S |
| U-CLI-12 | UI | `kronos completion bash/zsh/fish` | S |
| U-CLI-13 | UI | Colourised, TTY-aware output with `--no-color` + `NO_COLOR` | S |

### WebUI

| ID | Stream | Title | Size |
|----|--------|-------|------|
| U-WEB-01 | UI | Scaffold: Vite 6 + React 19 + TypeScript 5.6 + pnpm; `web/package.json`; `vite.config.ts`; strict tsconfig. | S |
| U-WEB-02 | UI | Tailwind v4.1 install + `@theme` with full Kronos palette + Cinzel/Inter/JetBrains Mono fonts | S |
| U-WEB-03 | UI | shadcn/ui init (`components.json`, "new-york" style, neutral base, CSS-variable theming pointed at Kronos tokens) | S |
| U-WEB-04 | UI | lucide-react wired; icon import convention + tree-shake check | XS |
| U-WEB-05 | UI | `useTheme` hook + ThemeProvider + localStorage persistence + `matchMedia` OS listener + `<ThemeToggle/>` | S |
| U-WEB-06 | UI | Generate TypeScript API types from OpenAPI (`openapi-typescript`); commit generated file | S |
| U-WEB-07 | UI | `lib/api.ts` — fetch wrapper with auth header, 401 refresh flow, typed responses | M |
| U-WEB-08 | UI | TanStack Query provider + query key conventions + error boundary | S |
| U-WEB-09 | UI | Zustand auth store + token refresh + logout | S |
| U-WEB-10 | UI | TanStack Router: programmatic routes + `beforeLoad` auth guard + redirect-back search param | M |
| U-WEB-11 | UI | Layout shell: responsive sidebar (off-canvas < md, rail md-lg, full ≥ lg) + header + user menu + breadcrumbs | M |
| U-WEB-12 | UI | Login page (email + password + TOTP) with react-hook-form + Zod | M |
| U-WEB-13 | UI | Dashboard: recent jobs table, repo size over time chart (Recharts), failing targets alert, next scheduled runs | M |
| U-WEB-14 | UI | Targets list (shadcn `<DataTable/>`) + detail view with backup history | M |
| U-WEB-15 | UI | Schedules list + calendar heatmap (custom component over Recharts `HeatMapGrid`) | M |
| U-WEB-16 | UI | Backups explorer: filter/search + chain visualisation (d3-hierarchy, rendered as SVG) | M |
| U-WEB-17 | UI | Restore wizard: horizontal stepper desktop / vertical mobile; 4 steps with form state; preview before execute | M |
| U-WEB-18 | UI | Storage page: per-backend stats cards + usage over time + GC status + "run GC" action | M |
| U-WEB-19 | UI | Audit log: infinite-scroll paginated view with shadcn `<Table/>` + hash-chain verification indicator | S |
| U-WEB-20 | UI | Settings → Users: CRUD + role grants | M |
| U-WEB-21 | UI | Settings → Tokens: create scoped token, copy-once modal, revoke | S |
| U-WEB-22 | UI | Settings → OIDC / Notifications / Keys | M |
| U-WEB-23 | UI | SSE live job updates (`/api/v1/jobs/:id/events`) feeding React Query cache | M |
| U-WEB-24 | UI | Toast notifications (shadcn `<Toaster/>`) for mutations; skeleton loaders for queries | S |
| U-WEB-25 | UI | Accessibility: axe-core vitest suite across all `ui/` components in both themes; CI-enforced | M |
| U-WEB-26 | UI | Bundle-size CI guard: initial route ≤ 500 KB gzipped | XS |
| U-WEB-27 | UI | Responsive audit: every page at 360/768/1024/1440/1920 px; screenshot diffs committed | S |
| U-WEB-28 | UI | `web/build.sh` + `make ui` target → `internal/webui/static/` via go:embed; SPA fallback in `internal/webui/handler.go` | S |

Phase 4 exit: the "first admin" bootstrap → add a target → add a storage → add a schedule → see a backup happen → restore it, all via WebUI.

---

## Phase 5 — Observability, Notifications, Hooks (Week 11)

| ID | Stream | Title | Size |
|----|--------|-------|------|
| OBS-01 | OBS | Prometheus metrics registry + exposition on `/metrics` | M |
| OBS-02 | OBS | Core metrics (job duration, bytes, dedup ratio, backend latencies, scheduler lag, agent health) | M |
| OBS-03 | OBS | OpenTelemetry tracing (OTLP/gRPC) | M |
| OBS-04 | OBS | Job correlation ID propagation across server↔agent↔storage | S |
| OBS-05 | OBS | Audit log hash chain | S |
| OBS-06 | OBS | Audit log query + `kronos audit verify` | S |
| OBS-07 | OBS | Notification channels: Slack, Discord, Webhook, Email | M |
| OBS-08 | OBS | Notification channels: Telegram, PagerDuty, Opsgenie, Teams | M |
| OBS-09 | OBS | Router: rule evaluator (CEL-lite) | M |
| OBS-10 | OBS | Hooks: shell executor (sandboxed env, timeout) | M |
| OBS-11 | OBS | Hooks: webhook | S |
| OBS-12 | OBS | Hooks: WASM via `wazero` (behind build tag) | M |
| OBS-13 | OBS | Hook context JSON schema | S |
| OBS-14 | OBS | Fail-closed vs fail-open policy per hook | S |

---

## Phase 6 — Verification, Secrets, Hardening (Week 12)

| ID | Stream | Title | Size |
|----|--------|-------|------|
| V-01 | SRV | Verification level 1: manifest check, chunk HEAD | S |
| V-02 | SRV | Verification level 2: chunk integrity (BLAKE3 recompute) with sampling | M |
| V-03 | SRV | Verification level 3: logical replay to `/dev/null` per driver | M |
| V-04 | SRV | Verification level 4: sandbox live restore | L → split per driver |
| V-04a | SRV | Sandbox for PG (Docker-based) | M |
| V-04b | SRV | Sandbox for MySQL | M |
| V-04c | SRV | Sandbox for Mongo | M |
| V-04d | SRV | Sandbox for Redis | S |
| V-05 | SRV | Verification schedule + metrics + notification on failure | S |
| SEC-01 | CORE | Secret provider: Vault KV v2 + AppRole | M |
| SEC-02 | CORE | Secret provider: AWS Secrets Manager | M |
| SEC-03 | CORE | Secret provider: GCP Secret Manager | M |
| SEC-04 | CORE | Secret provider: Azure Key Vault | M |
| SEC-05 | CORE | Secret provider: Doppler | S |
| SEC-06 | CORE | Secret provider: 1Password Connect | S |
| SEC-07 | CORE | Secret rotation: re-resolve on next job without restart | S |
| SEC-08 | CORE | `secrets.age` built-in encrypted store | M |
| HRD-01 | SRV | First-admin bootstrap token + mandatory password change | S |
| HRD-02 | SRV | Rate limiting on auth endpoints | S |
| HRD-03 | SRV | CSRF + secure cookie flags on WebUI | S |
| HRD-04 | SRV | 24h soak test: no goroutine leaks | M |
| HRD-05 | REL | `govulncheck` clean in CI | XS |
| HRD-06 | REL | `staticcheck` clean in CI | S |
| HRD-07 | REL | Reproducible builds + cosign keyless signing | M |

---

## Phase 7 — Release, Documentation, Distribution (Week 13)

| ID | Stream | Title | Size |
|----|--------|-------|------|
| R-01 | REL | `goreleaser`-equivalent release script | M |
| R-02 | REL | OCI image (distroless static) + `ghcr.io` publish | S |
| R-03 | REL | Homebrew tap + formula | S |
| R-04 | REL | Scoop bucket + manifest | S |
| R-05 | REL | systemd units in `contrib/systemd/` | S |
| R-06 | REL | Helm chart in `contrib/helm/kronos/` | M |
| R-07 | REL | Ansible role in `contrib/ansible/` | M |
| R-08 | REL | Debian/Ubuntu APT repo + `.deb` packages | M |
| R-09 | REL | RPM for RHEL/Fedora | M |
| DOC-01 | REL | Docs site (MkDocs Material) | M |
| DOC-02 | REL | Quick start guide (5-minute path) | S |
| DOC-03 | REL | Operations runbook (upgrade, key rotation, DR) | M |
| DOC-04 | REL | Migration guide: from `pgBackRest` | S |
| DOC-05 | REL | Migration guide: from `Barman` | S |
| DOC-06 | REL | Migration guide: from shell scripts | S |
| DOC-07 | REL | Architecture deep-dive doc | M |
| DOC-08 | REL | OpenAPI rendered reference | S |
| DOC-09 | REL | CLI reference (auto-generated from help output) | S |
| DOC-10 | REL | `kronos(8)` manpage | S |

---

## Phase 8 — v0.2 Stretch (post-MVP)

Deferred to v0.2 but tracked.

| ID | Stream | Title |
|----|--------|-------|
| X-01 | SRV | Kubernetes operator with Target/Schedule/Storage CRDs |
| X-02 | STG | WebDAV backend |
| X-03 | DRV | PostgreSQL physical (BASE_BACKUP) GA |
| X-04 | DRV | MySQL physical (InnoDB pages) |
| X-05 | DRV | ClickHouse driver |
| X-06 | DRV | SQLite driver |
| X-07 | DRV | MSSQL driver |
| X-08 | SRV | HA cluster mode (Raft + Gossip) |
| X-09 | SRV | Hot-standby agent topology |
| X-10 | UI | Plugin marketplace UI |
| X-11 | OBS | Kronos-native Grafana dashboards |

---

## Risk Register

| ID | Risk | Impact | Likelihood | Mitigation |
|----|------|--------|-----------|-----------|
| R1 | B+Tree implementation slips | High | Medium | Fallback to vendored bbolt; decision gate at Phase 1 end. |
| R2 | SigV4 edge-case bugs cause data integrity risk | Critical | Low | AWS test vectors mandatory, plus MinIO + R2 integration tests. |
| R3 | PITR WAL replay divergence vs native Postgres | Critical | Medium | Differential testing: `pg_dump -Fc` output after PITR replayed matches baseline. |
| R4 | Binlog parser misses event types → silent data loss | Critical | Medium | Fuzz + exhaustive fixture tests; parser fails closed on unknown event type in v1. |
| R5 | BSON corner cases | High | Medium | Fuzz parser; cross-check with official driver's output byte-for-byte on fixtures. |
| R6 | RDB format changes in Redis major version | Medium | Low | Driver version-pins the RDB version it handles; refuses unknown. |
| R7 | Scheduler clock skew causes missed runs | High | Low | Clock abstraction; tests with `FakeClock`; NTP-drift simulation. |
| R8 | WebUI bundle too large | Medium | Low | Size budget enforced in CI (< 300 KB gzipped). |
| R9 | Go plugin system removes target OS support | Medium | Medium | Build-tag gated; default off; WASM plugins are the recommended path. |
| R10 | 7 dependency creep | Medium | High | `go mod why` CI check; PRs that add deps require explicit justification in PR description. |

---

## Count

Phase 0: **13** tasks  
Phase 1: **35** tasks (S:18 + C:12 + K:9, minus the split sub-tasks counted separately = ~35)  
Phase 2: **51** tasks (PG:18 + MY:13 + MO:12 + RD:10+splits)  
Phase 3: **27** tasks  
Phase 4: **41** tasks (CLI:13 + Web:28)  
Phase 5: **14** tasks  
Phase 6: **18** tasks  
Phase 7: **19** tasks

**Total: ~218 tasks** for v0.1.0 (MVP), ~11–14 weeks for one full-time engineer; 4–6 weeks if four work-streams are parallelised.

Phase 8 (stretch): **11** tasks, targeted v0.2.

---

*End of TASKS.md*
