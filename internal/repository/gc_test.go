package repository

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/manifest"
	"github.com/kronos/kronos/internal/storage/storagetest"
)

func TestGarbageCollectDeletesUnreferencedChunks(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	backend := storagetest.NewMemoryBackend("repo")
	putObject(t, backend, "data/aa/keep", []byte("keep"))
	putObject(t, backend, "data/bb/drop", []byte("drop"))
	putManifest(t, backend, privateKey, "manifests/2026/04/23/backup-1.manifest", "data/aa/keep")

	report, err := GarbageCollect(context.Background(), backend, publicKey, GCOptions{})
	if err != nil {
		t.Fatalf("GarbageCollect() error = %v", err)
	}
	if report.Manifests != 1 || report.ReferencedChunks != 1 || report.ScannedChunks != 2 || report.DeletedChunks != 1 || report.DeletedBytes != 4 {
		t.Fatalf("report = %#v", report)
	}
	exists, err := backend.Exists(context.Background(), "data/aa/keep")
	if err != nil {
		t.Fatalf("Exists(keep) error = %v", err)
	}
	if !exists {
		t.Fatal("referenced chunk was deleted")
	}
	exists, err = backend.Exists(context.Background(), "data/bb/drop")
	if err != nil {
		t.Fatalf("Exists(drop) error = %v", err)
	}
	if exists {
		t.Fatal("orphan chunk still exists")
	}
}

func TestGarbageCollectDryRunKeepsOrphans(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	backend := storagetest.NewMemoryBackend("repo")
	putObject(t, backend, "data/aa/keep", []byte("keep"))
	putObject(t, backend, "data/bb/drop", []byte("drop"))
	putManifest(t, backend, privateKey, "manifests/2026/04/23/backup-1.manifest", "data/aa/keep")

	report, err := GarbageCollect(context.Background(), backend, publicKey, GCOptions{DryRun: true})
	if err != nil {
		t.Fatalf("GarbageCollect(dry-run) error = %v", err)
	}
	if report.DeletedChunks != 0 || len(report.OrphanChunks) != 1 || report.OrphanChunks[0].Key != "data/bb/drop" {
		t.Fatalf("report = %#v", report)
	}
	exists, err := backend.Exists(context.Background(), "data/bb/drop")
	if err != nil {
		t.Fatalf("Exists(drop) error = %v", err)
	}
	if !exists {
		t.Fatal("dry-run deleted orphan chunk")
	}
}

func TestGarbageCollectFailsClosedOnBadManifestSignature(t *testing.T) {
	t.Parallel()

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey(second) error = %v", err)
	}
	backend := storagetest.NewMemoryBackend("repo")
	putObject(t, backend, "data/orphan", []byte("drop"))
	putManifest(t, backend, privateKey, "manifests/2026/04/23/backup-1.manifest", "data/orphan")

	if _, err := GarbageCollect(context.Background(), backend, publicKey, GCOptions{}); err == nil {
		t.Fatal("GarbageCollect() error = nil, want bad manifest signature")
	}
	exists, err := backend.Exists(context.Background(), "data/orphan")
	if err != nil {
		t.Fatalf("Exists(orphan) error = %v", err)
	}
	if !exists {
		t.Fatal("GC deleted chunk after bad manifest signature")
	}
}

func putObject(t *testing.T, backend *storagetest.MemoryBackend, key string, payload []byte) {
	t.Helper()
	if _, err := backend.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Put(%q) error = %v", key, err)
	}
}

func putManifest(t *testing.T, backend *storagetest.MemoryBackend, privateKey ed25519.PrivateKey, key string, chunkKey string) {
	t.Helper()
	now := time.Date(2026, 4, 23, 2, 0, 0, 0, time.UTC)
	m := manifest.New()
	m.BackupID = "backup-1"
	m.Target = "target"
	m.Driver = manifest.Driver{Name: "redis", Version: "7.2"}
	m.Type = core.BackupTypeFull
	m.StartedAt = now
	m.FinishedAt = now
	m.Encryption = manifest.Encryption{Algorithm: "aes-256-gcm", KeyID: "k1"}
	m.Objects = []manifest.Object{{
		Name:   "stream",
		Chunks: []manifest.ChunkRef{{Hash: "abc", Key: chunkKey, Size: 4, StoredSize: 4}},
	}}
	if err := m.Sign(privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	data, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	putObject(t, backend, key, data)
}
