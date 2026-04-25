package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/manifest"
	"github.com/kronos/kronos/internal/storage/local"
)

func TestRunGCDryRun(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	repo := t.TempDir()
	backend, err := local.New("local", repo)
	if err != nil {
		t.Fatalf("local.New() error = %v", err)
	}
	putLocalObject(t, backend, "data/keep", []byte("keep"))
	putLocalObject(t, backend, "data/drop", []byte("drop"))
	putLocalManifest(t, backend, privateKey, "manifests/2026/04/23/backup-1.manifest", "data/keep")

	var out bytes.Buffer
	err = run(context.Background(), &out, []string{
		"gc",
		"--storage-local", repo,
		"--public-key", hex.EncodeToString(publicKey),
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("gc --dry-run error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) || !strings.Contains(out.String(), `"dry_run":true`) || !strings.Contains(out.String(), `"orphan_chunks":1`) {
		t.Fatalf("gc --dry-run output = %q", out.String())
	}
	exists, err := backend.Exists(context.Background(), "data/drop")
	if err != nil {
		t.Fatalf("Exists(drop) error = %v", err)
	}
	if !exists {
		t.Fatal("dry-run deleted orphan chunk")
	}
}

func TestRunGCDeletesOrphan(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	repo := t.TempDir()
	backend, err := local.New("local", repo)
	if err != nil {
		t.Fatalf("local.New() error = %v", err)
	}
	putLocalObject(t, backend, "data/keep", []byte("keep"))
	putLocalObject(t, backend, "data/drop", []byte("drop"))
	putLocalManifest(t, backend, privateKey, "manifests/2026/04/23/backup-1.manifest", "data/keep")

	var out bytes.Buffer
	err = run(context.Background(), &out, []string{
		"gc",
		"--storage-local", repo,
		"--public-key", hex.EncodeToString(publicKey),
	})
	if err != nil {
		t.Fatalf("gc error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) || !strings.Contains(out.String(), `"dry_run":false`) || !strings.Contains(out.String(), `"deleted_chunks":1`) {
		t.Fatalf("gc output = %q", out.String())
	}
	exists, err := backend.Exists(context.Background(), "data/drop")
	if err != nil {
		t.Fatalf("Exists(drop) error = %v", err)
	}
	if exists {
		t.Fatal("orphan chunk still exists")
	}
}

func putLocalObject(t *testing.T, backend *local.Backend, key string, payload []byte) {
	t.Helper()
	if _, err := backend.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("Put(%q) error = %v", key, err)
	}
}

func putLocalManifest(t *testing.T, backend *local.Backend, privateKey ed25519.PrivateKey, key string, chunkKey string) {
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
	putLocalObject(t, backend, key, data)
}
