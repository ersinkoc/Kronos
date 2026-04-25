# 0001: Embedded KV Store Direction

Status: provisional

Kronos keeps server state in a single embedded database file. The current Phase 1 implementation continues with an in-repository, pure-Go page store and B+Tree:

- 4 KiB mmap-backed pages with a persistent free list.
- B+Tree point lookup, range scan, insert, update, delete, root growth, and nested branch growth.
- Single-writer/read transaction facade with rollback-WAL recovery.
- Bucket-style key namespaces.
- Repair routine that rebuilds the free list from pages reachable through the root tree.

The fallback remains vendoring `go.etcd.io/bbolt` if the in-repo implementation fails durability, chaos, or performance checks once the Go toolchain is available in CI/local verification.

Decision gate:

- Continue custom KV store if `go test ./...`, pager/BTREE chaos tests, rollback recovery, and benchmarks pass.
- Fall back to vendored bbolt if branch split/merge, WAL recovery, or performance targets remain unstable by the Phase 1 exit.
