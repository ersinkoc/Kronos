# Kronos — IMPLEMENTATION

Companion to `SPECIFICATION.md`. This document is the blueprint an engineer (human or LLM) reads before writing code. It freezes the package layout, the internal interfaces, the state machines, and the non-obvious algorithm choices.

---

## 1. Module & Repository Layout

Go module path: `github.com/kronos/kronos` (subject to domain availability; `github.com/kronosdb/kronos` is the fallback).

```
kronos/
├── cmd/
│   └── kronos/                 # single binary, subcommand dispatcher
│       ├── main.go
│       ├── server.go           # `kronos server`
│       ├── agent.go            # `kronos agent`
│       ├── local.go            # `kronos local` (server+agent in one process)
│       └── cli/                # all `kronos <verb>` subcommands
│           ├── target.go
│           ├── backup.go
│           ├── restore.go
│           ├── schedule.go
│           ├── storage.go
│           ├── key.go
│           ├── user.go
│           ├── audit.go
│           └── ...
│
├── internal/
│   ├── core/                   # domain types, no I/O
│   │   ├── types.go            # Target, Storage, Schedule, Job, Backup, Manifest
│   │   ├── id.go               # UUIDv7 generator
│   │   ├── errors.go
│   │   ├── clock.go            # Clock interface for testability
│   │   └── policy/             # retention rules, concurrency rules
│   │
│   ├── config/                 # YAML load, schema validate, env/vault substitution
│   ├── secret/                 # secret resolvers: env, file, vault, aws-sm, gcp-sm, az-kv
│   │
│   ├── drivers/                # database driver interface + implementations
│   │   ├── driver.go           # the Driver interface
│   │   ├── registry.go
│   │   ├── postgres/
│   │   │   ├── driver.go
│   │   │   ├── protocol/       # pure-Go wire protocol
│   │   │   ├── copy.go
│   │   │   ├── basebackup.go   # physical via replication protocol
│   │   │   ├── wal.go          # WAL streaming
│   │   │   └── restore.go
│   │   ├── mysql/
│   │   │   ├── driver.go
│   │   │   ├── protocol/       # pure-Go MySQL wire
│   │   │   ├── dump.go
│   │   │   ├── binlog.go
│   │   │   └── restore.go
│   │   ├── mongodb/
│   │   │   ├── driver.go
│   │   │   ├── wire/           # OP_MSG implementation
│   │   │   ├── dump.go
│   │   │   ├── oplog.go
│   │   │   └── restore.go
│   │   └── redis/
│   │       ├── driver.go
│   │       ├── resp/           # RESP2+RESP3 codec
│   │       ├── rdb.go
│   │       ├── aof.go
│   │       └── restore.go
│   │
│   ├── storage/                # storage backends
│   │   ├── backend.go          # Backend interface + helpers
│   │   ├── registry.go
│   │   ├── local/
│   │   ├── s3/                 # pure-Go sigv4 + multipart
│   │   ├── sftp/               # pure-Go (wraps golang.org/x/crypto/ssh)
│   │   ├── ftp/                # stdlib net/textproto based
│   │   ├── azure/              # REST against blob endpoint
│   │   ├── gcs/                # REST against storage endpoint
│   │   └── webdav/             # Phase 2
│   │
│   ├── compress/
│   │   ├── zstd.go             # wraps klauspost/compress/zstd
│   │   ├── gzip.go
│   │   ├── lz4.go
│   │   └── auto.go             # entropy-based selector
│   │
│   ├── crypto/
│   │   ├── cipher.go           # AEAD abstraction (AES-GCM / ChaCha20-Poly1305)
│   │   ├── kdf.go              # Argon2id
│   │   ├── key.go              # key hierarchy, slots, escrow
│   │   └── repokey.go          # repository header parsing
│   │
│   ├── chunk/
│   │   ├── fastcdc.go          # content-defined chunking
│   │   ├── hash.go             # BLAKE3 wrapper
│   │   ├── index.go            # in-memory chunk index + bloom filter
│   │   └── pipeline.go         # chunk→compress→encrypt→upload pipeline
│   │
│   ├── manifest/
│   │   ├── manifest.go         # parse/serialise, signature verify
│   │   ├── chain.go            # parent/child graph walk
│   │   └── gc.go               # mark-and-sweep
│   │
│   ├── schedule/
│   │   ├── cron.go             # RFC 5-field + 6-field parser
│   │   ├── calendar.go         # @hourly, @every, @between parsers
│   │   ├── scheduler.go        # tick loop, job dispatcher
│   │   └── queue.go            # concurrency-limited per-target queue
│   │
│   ├── retention/
│   │   ├── gfs.go
│   │   ├── count.go
│   │   ├── time.go
│   │   ├── size.go
│   │   └── resolver.go         # combine rules → keep-set
│   │
│   ├── hooks/
│   │   ├── shell.go
│   │   ├── webhook.go
│   │   ├── goplugin.go         # behind build tag
│   │   └── wasm.go             # behind build tag; wazero
│   │
│   ├── notify/
│   │   ├── channel.go          # Channel interface
│   │   ├── slack.go
│   │   ├── discord.go
│   │   ├── email.go
│   │   ├── telegram.go
│   │   ├── webhook.go
│   │   ├── pagerduty.go
│   │   ├── opsgenie.go
│   │   ├── teams.go
│   │   └── router.go           # CEL-lite expression evaluator for routing rules
│   │
│   ├── audit/
│   │   ├── log.go              # hash-chained append
│   │   ├── query.go
│   │   └── redact.go           # secret redaction
│   │
│   ├── kvstore/                # embedded pure-Go B+Tree
│   │   ├── tree.go
│   │   ├── page.go
│   │   ├── wal.go
│   │   └── txn.go
│   │
│   ├── api/
│   │   ├── rest/               # net/http handlers + OpenAPI emission
│   │   ├── grpc/               # gRPC server for agent+admin
│   │   └── mcp/                # MCP server
│   │
│   ├── webui/
│   │   ├── embed.go            # go:embed static FS
│   │   └── handler.go
│   │
│   ├── obs/                    # observability
│   │   ├── metrics.go          # Prometheus exposition
│   │   ├── tracing.go          # OTLP exporter
│   │   └── log.go              # slog setup
│   │
│   ├── server/                 # control plane wiring
│   │   ├── server.go
│   │   ├── orchestrator.go     # dispatches jobs to agents
│   │   └── verify.go           # verification scheduler
│   │
│   └── agent/
│       ├── agent.go
│       ├── executor.go         # runs a job end-to-end
│       └── stream.go           # persistent connection back to server
│
├── api/
│   ├── proto/                  # .proto files
│   └── openapi/                # generated openapi.yaml
│
├── web/                        # Vite + React 19 + Tailwind v4 + shadcn/ui source tree
│   ├── src/
│   │   ├── components/
│   │   │   ├── ui/             # shadcn/ui primitives (button, dialog, toast, …)
│   │   │   ├── layout/         # shell: sidebar, header, topbar, breadcrumbs
│   │   │   ├── charts/         # chart composites built on recharts
│   │   │   └── features/       # feature-scoped composites (BackupRow, ChainGraph …)
│   │   ├── pages/              # one file per route
│   │   │   ├── dashboard.tsx
│   │   │   ├── targets.tsx
│   │   │   ├── backups.tsx
│   │   │   ├── schedules.tsx
│   │   │   ├── restore.tsx
│   │   │   ├── storage.tsx
│   │   │   ├── audit.tsx
│   │   │   └── settings/
│   │   ├── hooks/              # useTheme, useAuth, useQueryWithToast …
│   │   ├── lib/
│   │   │   ├── api.ts          # fetch wrapper, auth headers, refresh flow
│   │   │   ├── utils.ts        # cn() helper (clsx + tailwind-merge)
│   │   │   └── format.ts       # byte/duration/relative-time formatters
│   │   ├── stores/             # zustand stores (auth, theme, ui)
│   │   ├── router.tsx          # TanStack Router (type-safe, code-split)
│   │   ├── main.tsx            # entry; mounts <App/>
│   │   ├── App.tsx
│   │   └── index.css           # Tailwind v4 @theme directive + tokens
│   ├── public/                 # static assets: logo, favicons
│   ├── index.html
│   ├── components.json         # shadcn/ui generator config
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── package.json            # only build-time; never shipped at runtime
│   └── build.sh                # pnpm install + vite build → internal/webui/static/
│
├── bench/                      # benchmark harness + reference datasets
├── contrib/                    # systemd units, Helm chart, Ansible role
├── docs/                       # Hugo/MkDocs source
│
├── go.mod
├── go.sum
├── Makefile
├── SPECIFICATION.md
├── IMPLEMENTATION.md
├── TASKS.md
├── BRANDING.md
├── README.md
└── LICENSE
```

Rationale for `internal/` vs `pkg/`:
- Everything lives under `internal/` to forbid external imports — Kronos is an application, not a library. We deliberately avoid exposing a stable Go API surface.
- Only `api/proto` is public (the wire contract).

---

## 2. Key Interfaces

### 2.1 `drivers.Driver`

```go
// Package drivers defines the contract every database integration implements.
package drivers

import (
    "context"
    "io"
    "time"
)

// Driver is the contract implemented by every database integration.
// Implementations MUST be safe for concurrent use across different Targets.
type Driver interface {
    Name() string
    Version(ctx context.Context, t Target) (string, error)

    // Test validates connectivity, auth and required permissions.
    Test(ctx context.Context, t Target) error

    // BackupFull produces a logical or physical full backup and writes
    // encoded records to w. The returned ResumePoint identifies the
    // starting position for subsequent stream capture (WAL/binlog/oplog).
    BackupFull(ctx context.Context, t Target, w RecordWriter) (ResumePoint, error)

    // BackupIncremental writes records changed since the parent backup.
    // Drivers that cannot do cheap incrementals MAY return
    // ErrIncrementalUnsupported; the engine then falls back to a full.
    BackupIncremental(ctx context.Context, t Target, parent Manifest, w RecordWriter) (ResumePoint, error)

    // Stream writes a continuous change stream starting from rp to w
    // until ctx is cancelled. Used for PITR.
    Stream(ctx context.Context, t Target, rp ResumePoint, w StreamWriter) error

    // Restore consumes records from r and applies them to t.
    Restore(ctx context.Context, t Target, r RecordReader, opts RestoreOptions) error

    // ReplayStream applies stream records from r to t, stopping at target.
    ReplayStream(ctx context.Context, t Target, r StreamReader, target ReplayTarget) error
}

// RecordWriter is the driver's output. Drivers emit logical records;
// the engine is responsible for chunking, compressing, encrypting,
// and uploading. Drivers MUST NOT touch storage directly.
type RecordWriter interface {
    WriteRecord(obj ObjectRef, payload []byte) error
    FinishObject(obj ObjectRef, rows int64) error
}
```

Why this shape:
- **Drivers know nothing about storage, crypto, or compression.** They emit a typed record stream. The engine runs the pipeline. This makes drivers trivially unit-testable (write to a `bytes.Buffer`).
- **No file paths.** Drivers never write to disk. Everything streams.
- **One method per backup kind** rather than a union `Options` struct, because the signatures differ in what they consume (`parent Manifest` for incr).

### 2.2 `storage.Backend`

```go
package storage

import (
    "context"
    "io"
)

type Backend interface {
    Name() string

    // Put streams r to the object at key. Returns the resulting object
    // metadata (etag/size) after successful upload.
    Put(ctx context.Context, key string, r io.Reader, size int64) (ObjectInfo, error)

    // Get streams the object at key.
    Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)

    // GetRange streams bytes [off, off+len).
    GetRange(ctx context.Context, key string, off, length int64) (io.ReadCloser, error)

    // Head returns metadata without data.
    Head(ctx context.Context, key string) (ObjectInfo, error)

    // Exists is a shortcut over Head that returns only a boolean.
    Exists(ctx context.Context, key string) (bool, error)

    // Delete removes the object; missing is not an error.
    Delete(ctx context.Context, key string) error

    // List streams matching keys with an optional prefix and continuation token.
    List(ctx context.Context, prefix string, token string) (ListPage, error)
}

type ObjectInfo struct {
    Key       string
    Size      int64
    ETag      string
    UpdatedAt time.Time
}
```

Why:
- Everything is `io.Reader` / `io.ReadCloser`. No in-memory buffers for objects. Chunks are typically 1-4 MiB so this matters for throughput, not just big files.
- `GetRange` mandatory — required for partial restores and resuming uploads.
- No "CreateMultipart/UploadPart/Complete" leak in the interface. Multipart is S3 implementation detail, handled inside `s3/put.go`.

### 2.3 `chunk.Pipeline`

```go
package chunk

type Pipeline struct {
    Chunker    *FastCDC           // 512KiB / 2MiB / 8MiB
    Compressor Compressor
    Cipher     crypto.AEAD
    Backend    storage.Backend
    Index      *Index             // BLAKE3 hex → Presence
    Concurrency int               // worker pool size
}

// Feed processes an input stream into (compressed, encrypted, deduplicated,
// uploaded) chunks. Returns a slice of references suitable for inclusion
// in a manifest. Uploads duplicate chunks are suppressed.
func (p *Pipeline) Feed(ctx context.Context, r io.Reader) ([]ChunkRef, Stats, error)
```

Internally:

```
+-----------+   +--------+   +---------+   +--------+   +--------+
|  Reader   |-->| FastCDC|-->| Hasher  |-->| Encode |-->|Upload  |
| (driver)  |   | chunks |   | BLAKE3  |   | zstd+  |   |worker  |
|           |   |        |   | dedup?  |   | AES-GCM|   |pool    |
+-----------+   +--------+   +---------+   +--------+   +--------+
                              |
                              v
                       chunk.Index (local cache)
```

- Back-pressure via bounded channels (capacity = 2 × concurrency).
- Dedup decision before compress/encrypt (don't waste CPU on chunks we already have).
- Local index is a persistent map (`kvstore`) of known chunk hashes for this repo. Bloom filter in front for O(1) "definitely not seen". Full set for certainty.

### 2.4 `schedule.Scheduler`

```go
package schedule

type Scheduler struct {
    Clock     core.Clock
    Store     *SchedStore      // persistent state: next-run, last-run, catchup
    Queue     *Queue           // concurrency-limited dispatch
    JobCh     chan<- Dispatch  // consumed by orchestrator
    TickEvery time.Duration    // default 1s
}

func (s *Scheduler) Run(ctx context.Context) error
```

- Wheel-less, pure-tick: every `TickEvery` the scheduler scans schedules whose `next_run ≤ now` and dispatches.
- For 10k schedules, O(n) scan at 1 Hz is ~100 µs in benchmark; simpler than a priority queue and immune to time-jump bugs.
- Catch-up: on startup, iterates schedules and for each schedule runs catch-up policy exactly once.

### 2.5 `audit.Log`

Hash-chained:

```go
type Event struct {
    Seq       uint64
    Timestamp time.Time
    Actor     string     // user id or "system:scheduler"
    Action    string     // "backup.started", "user.created", ...
    Subject   string     // target name, user id, ...
    Details   json.RawMessage
    PrevHash  [32]byte
    ThisHash  [32]byte   // BLAKE3(PrevHash || Seq || canonical(rest))
}
```

- Canonical form: sorted JSON keys, no whitespace.
- `ThisHash` of event N is `PrevHash` of event N+1.
- On startup the server verifies the last 1000 events; full verification available via `kronos audit verify`.

---

## 3. Backup Job State Machine

```
              +--------------+
              |  SCHEDULED   |
              +------+-------+
                     |
        dispatcher picks up
                     |
                     v
              +--------------+
              |   QUEUED     |  (waiting for target slot)
              +------+-------+
                     |
      queue releases slot; agent available
                     |
                     v
              +--------------+
              |   RUNNING    |
              +---+------+---+
                  |      |
   success signal |      | error / cancel
                  |      |
                  v      v
          +----------+  +----------+
          | FINALIZE |  |  FAILED  |
          +-----+----+  +----------+
                |
       manifest commit fsync
                |
                v
          +-----------+
          |  SUCCESS  |
          +-----------+
```

- Transitions are persisted in `kronos.db` atomically with the `Job` record updates.
- Orphan detection on startup: any `RUNNING`/`FINALIZE` job whose agent has not checked in within heartbeat_timeout (default 5 min) is moved to `FAILED` with reason `agent_lost`.
- `RUNNING → FINALIZE` is the "all chunks uploaded, about to write manifest" boundary.
- `FINALIZE → SUCCESS` is atomic: the manifest write is the commit marker. A crash mid-finalize leaves staged chunks that GC reclaims.

---

## 4. Restore State Machine

```
  +-----------+        +-----------+       +-----------+
  |  PLAN     | -----> |  PRE      | ----->|  DATA     |
  | (compute  |        |  (drop/   |       |  (stream  |
  |  chain &  |        |  create   |       |  chunks   |
  |  stream   |        |  target)  |       |  into DB) |
  |  range)   |        +-----------+       +-----+-----+
  +-----+-----+                                   |
        |                                         v
        |                                   +-----------+
        | (dry-run)                         |  STREAM   |
        +-----------------+                 | (replay   |
                          v                 |  WAL/bin) |
                     +---------+            +-----+-----+
                     | DONE    |                  |
                     +---------+                  v
                                             +-----------+
                                             |  VERIFY   |
                                             | (checksum |
                                             |  / query) |
                                             +-----+-----+
                                                   |
                                                   v
                                             +---------+
                                             |  DONE   |
                                             +---------+
```

Failure at any stage emits an event and rolls back as far as each driver supports.

---

## 5. Agent ↔ Server Protocol

Transport: gRPC over HTTP/2 over mTLS. Default agent-facing port `8600` (server side). Servers open the listening socket; agents dial.

**But**: agents often sit behind NAT. So Kronos supports an inverted mode — the **agent dials the server** on `8500` and keeps a bidirectional streaming RPC alive. The server then sends `Dispatch` messages into the stream when it wants the agent to do work. This is the preferred default; classic "server dials agent" is opt-in.

Core RPCs:

```proto
service ControlPlane {
  // AgentConnect: the agent dials in and keeps this stream open for its lifetime.
  rpc AgentConnect(stream AgentMessage) returns (stream ServerMessage);

  // Admin surface — used by CLI and WebUI when talking to the server.
  rpc ListTargets(ListTargetsRequest) returns (ListTargetsResponse);
  rpc StartBackup(StartBackupRequest) returns (StartBackupResponse);
  rpc StreamJob(StreamJobRequest) returns (stream JobEvent);
  // ... rest of the admin surface
}

message AgentMessage {
  oneof payload {
    AgentHello hello = 1;
    Heartbeat heartbeat = 2;
    JobProgress progress = 3;
    JobFinished finished = 4;
    LogEntry log = 5;
  }
}

message ServerMessage {
  oneof payload {
    JobDispatch dispatch = 1;
    JobCancel cancel = 2;
    PingRequest ping = 3;
    ConfigUpdate config = 4;
  }
}
```

Heartbeat cadence: 10s. Loss after 30s → server marks agent as degraded and stops scheduling. Loss after 5 min → running jobs go to `FAILED`.

---

## 6. PostgreSQL Wire Protocol Notes

We implement the subset needed for backup/restore — not a general-purpose driver.

**Needed message types (frontend → backend)**: `StartupMessage`, `PasswordMessage` (incl. SCRAM-SHA-256 exchange), `Query`, `Parse`, `Bind`, `Execute`, `Describe`, `Close`, `Sync`, `CopyData`, `CopyDone`, `CopyFail`, `Terminate`.

**Needed message types (backend → frontend)**: `AuthenticationRequest` family, `BackendKeyData`, `ParameterStatus`, `ReadyForQuery`, `RowDescription`, `DataRow`, `CommandComplete`, `CopyInResponse`, `CopyOutResponse`, `CopyData`, `CopyDone`, `ErrorResponse`, `NoticeResponse`.

**Replication subprotocol**: `StartupMessage` with `replication=database` or `replication=true`, then `IDENTIFY_SYSTEM`, `CREATE_REPLICATION_SLOT`, `START_REPLICATION SLOT <n> <LSN>`, consume XLogData (0x77) and keepalive (0x6b).

We do **not** implement: prepared statement cache, connection pooling, query result caching, `pg_type` introspection beyond what's needed to serialise/deserialise the types present in the user's schema.

Logical backup algorithm:
1. Start a replication-protocol connection and `CREATE_REPLICATION_SLOT kronos_<uuid> LOGICAL pgoutput` (for PITR) OR a plain connection with `BEGIN ISOLATION LEVEL REPEATABLE READ; SELECT pg_export_snapshot();` (for single-shot logical).
2. Enumerate schemas: `SELECT nspname FROM pg_namespace WHERE ...`.
3. For each table: `SET TRANSACTION SNAPSHOT '<snap>'; COPY "<schema>"."<t>" TO STDOUT WITH (FORMAT BINARY)`.
4. Emit schema DDL by querying `pg_catalog` (we reimplement `pg_dump -s` surface — scope-limited to supported object kinds: schemas, tables, indexes, sequences, views, functions, materialized views, triggers, types, constraints, roles/grants).
5. Capture final LSN: `SELECT pg_current_wal_flush_lsn()`.
6. Emit manifest `streams.wal_start = <captured LSN>`.

Physical backup algorithm: use `BASE_BACKUP` replication command with `PROGRESS`, `MANIFEST 'yes'`, `MANIFEST_CHECKSUMS 'SHA256'`, `WAL 'stream'`. Parse the tar-like stream.

---

## 7. MySQL Wire Protocol Notes

Subset implementation too.

**Handshake**: Server Handshake v10 → `HandshakeResponse41` → `AuthSwitchRequest` / `AuthMoreData` (caching_sha2_password with RSA public-key exchange; mysql_native_password; ed25519 for MariaDB).

**Commands needed**: `COM_QUERY`, `COM_STMT_PREPARE`, `COM_STMT_EXECUTE`, `COM_STMT_CLOSE`, `COM_REGISTER_SLAVE`, `COM_BINLOG_DUMP_GTID`, `COM_BINLOG_DUMP`, `COM_SEMI_SYNC_ACK` (for semisync replicas).

**Binlog parsing**: row-based (`ROW_FORMAT=ROW`) is required for faithful PITR. We parse events: `FORMAT_DESCRIPTION_EVENT`, `PREVIOUS_GTIDS_LOG_EVENT`, `GTID_EVENT`, `QUERY_EVENT`, `TABLE_MAP_EVENT`, `WRITE_ROWS_EVENTv2`, `UPDATE_ROWS_EVENTv2`, `DELETE_ROWS_EVENTv2`, `XID_EVENT`, `ROTATE_EVENT`.

Logical backup:
1. `FLUSH TABLES WITH READ LOCK` (or `LOCK TABLES` per-table if FTWRL is denied).
2. `SHOW MASTER STATUS` → capture `(file, position, gtid_executed)`.
3. `START TRANSACTION WITH CONSISTENT SNAPSHOT` (InnoDB).
4. `UNLOCK TABLES`.
5. For each table: `SELECT *` chunked by primary-key range, emitted as logical rows with full column metadata.
6. DDL via `SHOW CREATE TABLE`, `SHOW CREATE VIEW`, `SHOW CREATE PROCEDURE`, `SHOW CREATE TRIGGER`, `SHOW CREATE FUNCTION`.

---

## 8. MongoDB Wire Protocol Notes

Only `OP_MSG` (MongoDB 3.6+). Earlier opcodes not supported.

**BSON**: use `go.mongodb.org/mongo-driver/bson`? **No** — #NOFORKANYMORE. We implement a focused BSON parser (read/write all type tags; write: all except `dbPointer`, `symbol`, deprecated). Parser lives in `drivers/mongodb/bson/`.

**Commands**: `hello`, `saslStart/saslContinue` (SCRAM-SHA-256), `listDatabases`, `listCollections`, `listIndexes`, `find` (with `cursor`, `getMore`, `killCursors`), `insert`, `createIndexes`.

Replica-set consistent snapshot:
1. `hello` → detect replica set or mongos.
2. On replica set: choose the primary or a secondary with `readPreference=secondary`, set `readConcern: { level: "snapshot", atClusterTime: <ts> }`.
3. `find` with that read concern for every collection, streamed via cursor batches.
4. Capture oplog position at the start of the snapshot for PITR.
5. On sharded cluster: connect to each shard primary independently, coordinate `atClusterTime`.

Oplog streaming: tail `local.oplog.rs` with `find ... tailable=true, awaitData=true, noCursorTimeout=true`, starting from saved position.

---

## 9. Redis Specifics

- RESP2 default; upgrade to RESP3 if `HELLO 3` succeeds.
- For RDB capture: issue `REPLICAOF NO ONE` not needed — we use the replica protocol: `SYNC` / `PSYNC ? -1`. The master then streams RDB bytes followed by a continuous command stream. We parse RDB incrementally (no intermediate file, constant memory) and keep consuming commands for PITR.
- Keys during full backup are captured via the RDB stream (fast; no `SCAN` needed). `SCAN` is only used for filtered/partial backups (`--include "user:*"`).
- ACL handling: `ACL LIST` + `ACL GETUSER` captured separately, restored via `ACL SETUSER`.

---

## 10. Storage Backend Specifics

### 10.1 S3

- Signature V4 implementation in `storage/s3/sig/`. Reference: AWS docs; we re-check against the standardised test vectors in `aws-sig-v4-test-suite`.
- Multipart upload threshold: 64 MiB. Part size: 16 MiB. Max 10 000 parts per object (S3 hard limit).
- `MinIO`, `R2`, `B2`, `Wasabi`, `Ceph RGW` tested with `endpoint` + `force_path_style` option.
- Retry policy: exponential backoff with jitter, max 5 retries, respect `Retry-After`.
- Checksums: `x-amz-checksum-sha256` on every part; full-object SHA-256 assembled and verified.
- No `aws-sdk-go`. It imports ~4 MB of code. Our implementation is under 3 000 LOC and only does what we need.

### 10.2 Azure Blob

- REST API version `2024-08-04`.
- Auth: SAS (query string), Shared Key (signature in `Authorization`), or OAuth 2.0 bearer via managed identity / service principal.
- Block blobs only. Block size 4 MiB. Commit list at end.

### 10.3 GCS

- JSON API. Auth: service account JSON (JWT → token exchange) or ADC.
- Resumable uploads for objects > 16 MiB.

### 10.4 SFTP

- `golang.org/x/crypto/ssh` for transport; we write the SFTP protocol ourselves (packets 3-4 KiB each, window-aware).
- Auth: password, publickey (ed25519, rsa-sha2-256/512, ecdsa), ssh-agent forwarding.

### 10.5 Local

- Writes are `O_CREAT|O_EXCL` to a temp file then `rename` for atomicity. `fsync` on the file then on the parent directory before declaring success.
- Sharded directory structure mirrors the repo spec (`data/<aa>/<bb>/<hash>`).

---

## 11. Chunk Pipeline Concurrency Model

```
reader (driver goroutine)
   |
   v
[chunk_ch  cap=2N]    FastCDC worker  (1 goroutine — chunking is inherently serial)
   |
   v
[hash_ch   cap=2N]    hasher workers  (N goroutines, BLAKE3)
   |
   v
[comp_ch   cap=2N]    compressor+encryptor workers (N goroutines)
   |
   v
[upload_ch cap=2N]    uploader workers (N goroutines)
   |
   v
manifest builder (main goroutine)
```

`N = min(runtime.GOMAXPROCS(0), 8)` by default, configurable.

Backpressure handling:
- Bounded channels → natural flow control.
- If uploader is slow (saturated bandwidth), compressors block, then hashers, then chunker, then reader — the driver stops producing data until storage catches up.
- Dedup path: after hashing, check index. If present, skip compress/encrypt/upload and go straight to manifest builder with the existing chunk reference.

Error propagation:
- First error cancels the shared `ctx`.
- `errgroup.Group` aggregates worker exits.
- Partial uploads in `staging/<uuid>/` are left for GC; nothing under the repo's live namespace is touched.

---

## 12. Embedded KV Store (`internal/kvstore`)

We need a persistent, transactional key-value store for server state. We refuse to depend on SQLite (CGo), BoltDB (unmaintained but stdlib-only and still usable), Badger (huge dep tree), Pebble (huge dep tree).

**Decision**: implement our own B+Tree with a WAL. Scope:
- Single-file, mmap-backed.
- Page-based (4 KiB pages), copy-on-write.
- B+Tree with keys and values stored inline or in overflow pages.
- Transactions: single writer, multi-reader (MVCC via root swap).
- Durability: WAL + `fsync`; crash recovery replays WAL on open.
- No compaction needed at our scale (server state is small, < 1 GB typical).

Target size: ~3 500 LOC including tests. Reference: the Ben Johnson Bolt paper and the CMU DB course's B+Tree lecture.

**Alternative considered**: vendor `go.etcd.io/bbolt` (the Bolt fork). It's stdlib-only, battle-tested, ~5 MB binary impact. If implementation risk on our B+Tree runs hot, we fall back to bbolt. Recorded in TASKS.md as a risk-tracked choice.

---

## 13. Cron Parser

5-field (`m h dom mon dow`) and 6-field (`s m h dom mon dow`) syntaxes. We support:
- Ranges (`1-5`), lists (`1,3,5`), steps (`*/5`, `1-10/2`), names (`JAN`, `MON`).
- `L` (last) and `W` (weekday) extensions for `dom`.
- `#` (nth weekday) for `dow`.
- Predefined: `@yearly`, `@annually`, `@monthly`, `@weekly`, `@daily`, `@midnight`, `@hourly`.
- Kronos extensions: `@every <duration>`, `@between HH:MM-HH:MM`, `@jitter Nm`.

Parser: recursive-descent, produces an AST. Evaluator: `Next(after time.Time, tz *time.Location) time.Time` — computes the next firing time. Unit tests from the `cron-parser` reference test set.

---

## 14. FastCDC Implementation

FastCDC is a content-defined chunking algorithm; rolling hash with a gear table, with masks controlling chunk size distribution.

Parameters used:
- `min = 512 KiB`
- `avg = 2 MiB` (mask = 21 bits)
- `max = 8 MiB`
- Gear table: 256-entry random-permutation from a fixed seed (so chunk boundaries are deterministic across runs).

Implementation: streaming over `io.Reader`, emits `[]byte` per chunk. Zero-copy where possible (we use a 16 MiB ring buffer and return slices into it; the pipeline immediately hashes and copies on dedup-miss).

---

## 15. Encryption Layout

Per-chunk envelope:

```
+----+-----------+------------------+-----------+
| 02 |  key_id   |  nonce (12 B)    |  payload  |
| 1B |   8 B     |                  |           |
+----+-----------+------------------+-----------+
                 [                  AEAD encrypted                  ]
```

- `02` version byte (`01` reserved for future algos; `02` = AES-256-GCM; `03` = ChaCha20-Poly1305).
- `key_id`: which active slot's DEK was used.
- `nonce`: per-chunk random 96-bit.
- `payload`: ciphertext || tag (GCM tag 16 B).

Key derivation:
- Master key = Argon2id(passphrase, per-repo salt, 64 MiB, 3 iters, 4 threads) → 32 B.
- Repo root key = master key (slot unwrap: actually the slot stores HMAC-wrapped-root-key under master; rotating a slot doesn't re-derive everything).
- Per-backup DEK = HKDF-Expand(root key, "kronos.dek." || backup_id, 32 B).
- Per-chunk DEK = per-backup DEK (GCM nonce uniqueness guards us — 2^32 chunks per DEK max, well within the ~2^48 GCM birthday boundary).

Alternative deployment: **sealed-repo / age mode.** Repository is sealed to a set of `age` recipient public keys; any holder of a matching identity can unseal. This is the recommended mode for multi-team deployments — nobody types a passphrase into a CI config.

---

## 16. Manifest Signing

Every manifest is Ed25519-signed. The repo stores the public key in `repo.json`; the private key can live with the server (centralised trust) or with the agents (decentralised).

Signature covers a canonical form (keys sorted, no whitespace) of everything except the `signature` field itself.

Verification happens on every read — a cold restore from storage without the server still produces trustworthy data as long as the verifier has the public key.

---

## 17. Web UI Decisions

**Stack: Vite 6 + React 19 + TypeScript 5.6 + Tailwind CSS v4.1 + shadcn/ui + lucide-react.**

The UI is a type-safe SPA built at CI time, its production artefacts committed under `internal/webui/static/` and embedded into the Go binary via `go:embed`. **Node is a build-time dependency only; the shipped binary never runs JavaScript on the server.**

### 17.1 Build-time stack

| Layer | Choice | Version pin | Why |
|-------|--------|-------------|-----|
| Bundler | **Vite** | 6.x | Fast dev server, tree-shaken production bundle, first-class React + TS + Tailwind v4 integration. |
| Framework | **React** | 19.x | Actions, `useOptimistic`, server component-ready APIs; stable and modern. |
| Language | **TypeScript** | 5.6+ | Strict mode; no `any` anywhere in `src/`. |
| Styling | **Tailwind CSS** | 4.1.x | CSS-first configuration via `@theme` directive; no `tailwind.config.js`. |
| Component library | **shadcn/ui** | latest (copy-paste) | Accessible Radix primitives + Tailwind; we own the source. |
| Icons | **lucide-react** | latest | Consistent stroke weight, tree-shaken. |
| Routing | **TanStack Router** | 1.x | Type-safe file-based routing; route-level code splitting. |
| Server state | **@tanstack/react-query** | 5.x | Caching, retries, streaming, suspense. |
| Client state | **Zustand** | 5.x | Minimal; used for auth, theme, transient UI state. |
| Forms | **react-hook-form + Zod** | latest | Schema-driven validation; Zod schemas mirror API contracts. |
| Charts | **Recharts** | 2.x | Pairs with shadcn's chart primitives. |
| Date/time | **date-fns** | 4.x | Tree-shakable, no moment-size bloat. |

Pattern-align with the rest of the portfolio (Rampart, Clarion, Anvil, Kervan): React 19 stack with shadcn/ui is the standard.

### 17.2 Tailwind v4 configuration

Tailwind v4 is CSS-first. No `tailwind.config.js`. All tokens live inside `src/index.css` under the `@theme` directive:

```css
@import "tailwindcss";

@theme {
  --color-bronze:       #B87333;   /* Kronos Bronze — primary */
  --color-bronze-dark:  #8B5523;   /* Titan Forge */
  --color-bronze-light: #D4A963;   /* Aged Gold */
  --color-void:         #0B0B12;
  --color-basalt:       #17171F;
  --color-parchment:    #F5EFE4;
  --color-ivory:        #FBF7EF;
  --color-marble:       #ECE7DC;
  --color-ink:          #1A1820;
  --color-laurel:       #4CB07A;   /* success */
  --color-ember:        #E8A93A;   /* warning */
  --color-sacrifice:    #C0352B;   /* danger */
  --color-styx:         #4B3F72;   /* info */

  --font-display: "Cinzel", serif;
  --font-sans:    "Inter", system-ui, sans-serif;
  --font-mono:    "JetBrains Mono", ui-monospace, monospace;

  --radius-sm: 0.25rem;
  --radius:    0.5rem;
  --radius-lg: 0.75rem;
}
```

shadcn/ui's `components.json` is configured for `style: "new-york"`, `baseColor: "neutral"`, and CSS-variable-based theming. We override the generated `--primary`, `--accent`, `--destructive` etc. to point at the Kronos palette.

### 17.3 Dark / light theme

- **Default**: `prefers-color-scheme` from OS.
- **Override**: persisted in Zustand store backed by `localStorage` (key `kronos.theme`).
- **Switch**: `<ThemeToggle/>` component in the header — `Sun` / `Moon` / `Monitor` (lucide-react).
- Implementation: toggles `class="dark"` on `<html>`; Tailwind's `dark:` variants handle the rest.
- No `next-themes` dependency; a 40-line custom `useTheme` hook with `matchMedia` listener covers the full feature.

Contrast verification is automated: a vitest + axe-core check in CI walks every `ui/` component in both themes and fails on any WCAG 2.1 AA violation.

### 17.4 Responsive design

Tailwind's default breakpoints (`sm: 640`, `md: 768`, `lg: 1024`, `xl: 1280`, `2xl: 1536`). Mobile-first.

- **Navigation**: off-canvas sidebar under `md`, persistent rail under `lg`, full sidebar from `lg` up.
- **Data tables**: horizontal scroll containers under `md`; stacked card view for the backups list on phone.
- **Restore wizard**: vertical stepper on mobile, horizontal stepper on desktop.
- **Charts**: `ResponsiveContainer` (recharts) across the board; no fixed pixel widths.

Target: usable at 360 px (iPhone SE baseline); every interactive control has a ≥ 44 px tap target.

### 17.5 Routing topology

TanStack Router with file-based routing under `src/routes/` (or programmatic in `src/router.tsx` — we use programmatic for clearer auth-guard composition):

```
/                         → dashboard
/targets                  → list
/targets/:name            → detail
/backups                  → explorer
/backups/:id              → detail (chain graph)
/schedules                → list + calendar heatmap
/schedules/:name
/storage                  → repo health
/restore                  → wizard
/audit
/settings                 → nested:
  /settings/users
  /settings/roles
  /settings/tokens
  /settings/oidc
  /settings/notifications
  /settings/keys
/login
```

All authenticated routes are wrapped in a `beforeLoad` guard that checks the auth store and redirects to `/login` if absent, surfacing the intended route via search params for post-login redirect.

### 17.6 API layer

- `lib/api.ts` exports a `kronosClient` with typed methods per resource. Types are generated from the OpenAPI spec (`api/openapi/openapi.yaml`) via `openapi-typescript` at build time; commit the generated file for CI reproducibility.
- React Query hooks (`useTargets`, `useBackup(id)`, `useCreateSchedule`, …) wrap the client. Query keys are derived systematically: `['targets', 'list', filters]`, `['backups', 'detail', id]`.
- Mutations invalidate the appropriate query keys and show shadcn `<Toast/>` on success/failure.
- Long-running operations (backup in progress, restore) subscribe to an SSE stream at `/api/v1/jobs/:id/events` for live status.

### 17.7 Bundle budget

- Target: **≤ 500 KB gzipped** for the initial route; route-level code splitting keeps most pages under 100 KB additional.
- Enforced in CI: `vite build` output scanned, PR fails on regression > 5 %.
- No Moment.js, no Lodash-full (use `lodash-es` with tree-shaking and only for what stdlib equivalents are awkward about), no icon fonts (lucide only).

### 17.8 Accessibility (WCAG 2.1 AA)

- shadcn primitives are Radix-based → ARIA out of the box.
- Every custom component pairs a visual review with an axe-core assertion.
- Keyboard navigation: `Tab` traversal order matches visual order; skip-to-content link at top of `<body>`.
- Focus rings preserved (`focus-visible:ring-2 ring-bronze`).
- Colour is never the sole information channel (status icons always accompany colour).
- Respect `prefers-reduced-motion`: all framer-motion-style transitions are behind this media query.

### 17.9 Build & embed pipeline

`web/build.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
pnpm install --frozen-lockfile
pnpm run build                     # outputs to web/dist/
rm -rf ../internal/webui/static
mkdir -p ../internal/webui/static
cp -r dist/* ../internal/webui/static/
```

`internal/webui/embed.go`:
```go
package webui

import "embed"

//go:embed all:static
var Assets embed.FS
```

`internal/webui/handler.go` serves `index.html` for every non-`/api` route (SPA fallback) and sets cache headers: hashed assets → 1 year immutable, `index.html` → no-cache.

The `web/` tree's `node_modules/` is not committed. The `web/dist/` intermediate is not committed. Only `internal/webui/static/` is — it is regenerated by `make ui` and checked into each release tag so the binary is always buildable without Node.

---

## 18. Testing Strategy

**Unit tests**: every package, table-driven where possible, `-race` always on.

**Integration tests**: `integration/` directory with Go files gated by `//go:build integration`. Spins up real databases via `testcontainers-go`? **No** — we use `docker` directly via a tiny `internal/testdocker` package that wraps `net.Dial("unix", "/var/run/docker.sock")` and speaks the Docker HTTP API. Same stance as everywhere else: no heavy deps.

Databases covered in integration: PostgreSQL 14/15/16/17; MySQL 8.0/8.4, MariaDB 10.11/11.4; MongoDB 6/7/8; Redis 7/8, Valkey 7.2; Dragonfly latest.

Storage covered: local, MinIO (for S3), a mock Azure emulator (`Azurite`), a mock GCS emulator, and a tiny SFTP server (`golang.org/x/crypto/ssh` + SFTP responder we implement for tests).

**E2E tests**: `e2e/` scenarios: "nightly backup, restore, PITR", "GFS retention correctness over 400 simulated days", "key rotation with active restore in flight", "agent crash mid-backup resumes cleanly", "server restart loses no state".

**Property-based tests** (`testing/quick` stdlib): cron parser, FastCDC boundaries are stable for stable input, manifest parse/serialise round-trip preservation, retention keep-set is idempotent.

**Fuzz tests**: every wire protocol parser (PG, MySQL, Mongo, Redis) has a `FuzzParse_*` corpus. Run for 24h in nightly CI.

**Chaos tests**: toxi-proxy analogue (we write our own tiny in-process network chaos layer) — delay, drop, corrupt. Verify Kronos never commits a corrupted backup.

Coverage goals: `core/`, `drivers/`, `storage/`, `crypto/`, `chunk/` ≥ 80 %. Overall project ≥ 60 %.

---

## 19. CI/CD

GitHub Actions. Stages:

1. **lint** — `gofmt -l`, `go vet`, `staticcheck`, `govulncheck`.
2. **test-unit** — `go test -race ./...`.
3. **test-integration** — matrix over database versions.
4. **test-e2e** — longer-running scenarios, release-branch only.
5. **bench** — run `bench/` and post regression report as PR comment.
6. **build** — `goreleaser`-style cross-compile matrix (we script it in `scripts/release.sh` to avoid the dep; `goreleaser` is a CI tool, not a runtime dep).
7. **sign** — cosign keyless sign of binaries + OCI images.
8. **publish** — GitHub Releases, Homebrew tap PR, Scoop manifest, OCI image to `ghcr.io`.

Release cadence: patch weekly, minor monthly, major yearly (tentative).

---

## 20. Open Risks & Decisions Still to Make

Tracked in TASKS.md under the `RISK-*` prefix. Summary:

- **R1**: B+Tree vs vendored bbolt. Default plan is to implement; fallback is bbolt. Decision deadline: end of Phase 2.
- **R2**: BLAKE3 — vendor `lukechampine.com/blake3` or reimplement. Plan: vendor.
- **R3**: Chart library — Recharts (paired with shadcn chart primitives). Decision: use Recharts; it's the ecosystem standard with shadcn/ui and works well with Tailwind v4 theming.
- **R4**: MongoDB BSON — reimplement (done) or vendor the driver's bson sub-package. Plan: reimplement.
- **R5**: AWS sigv4 — reimplement (planned) or vendor `aws/aws-sdk-go-v2`. Plan: reimplement (~1 500 LOC with test vectors).
- **R6**: WebUI delivery — Vite 6 + React 19 + Tailwind v4 + shadcn/ui + lucide-react. Decision: pattern-aligned with Rampart/Clarion/Anvil. Build artefacts committed to `internal/webui/static/`; Node is build-time only, never runtime.
- **R7**: Physical backup for MySQL — likely slipped to v0.2 (requires deep InnoDB knowledge).
- **R8**: Hot-standby agent topology (two agents for same target with leader election) — slipped to v0.2.

---

## 21. What "Done" Looks Like (per subsystem)

| Subsystem | Done when |
|-----------|-----------|
| `drivers/postgres` | Full + incr + PITR against PG 14–17 in CI; COPY BINARY round-trips every column type in the test schema; replication slot is cleaned up on cancel. |
| `drivers/mysql` | Full + incr + PITR against MySQL 8.0/8.4 and MariaDB 10.11/11.4; GTID-aware binlog replay. |
| `drivers/mongodb` | Full + PITR against MongoDB 6/7/8 replica set; sharded cluster backup known to work. |
| `drivers/redis` | Full RDB capture and AOF streaming for Redis 7/8; ACL restore verified. |
| `storage/s3` | Passes AWS SigV4 test suite; works against AWS, MinIO, R2, B2, Wasabi. |
| `chunk` | Deterministic chunking; dedup ratio ≥ 0.75 on a realistic daily-incremental-of-same-DB benchmark. |
| `schedule` | 10 000 schedules tick in ≤ 5 ms; cron parser passes the 400-case test set. |
| `retention` | GFS simulation over 5 years deterministic; keep-set matches a hand-computed ground truth. |
| `audit` | Hash chain verifies; `kronos audit verify` passes; `kronos audit tamper-test` correctly fails on a flipped byte. |
| `webui` | All routes functional; Vite build output ≤ 500 KB gzipped (initial route); lighthouse ≥ 90 perf, ≥ 95 a11y; axe-core CI green in both themes; TanStack Router type-safe routes; responsive 360 px → 1920 px. |
| `mcp` | MCP conformance test passes; mutating tools require approval; read tools idempotent. |

---

*End of IMPLEMENTATION.md*
