package repository

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/chunk"
	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/engine"
	"github.com/kronos/kronos/internal/manifest"
	"github.com/kronos/kronos/internal/storage"
	"github.com/kronos/kronos/internal/storage/storagetest"
)

func TestCommitManifestWritesSignedManifest(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	backend := storagetest.NewMemoryBackend("repo")
	finished := time.Date(2026, 4, 23, 5, 11, 0, 0, time.UTC)
	payloadHash := chunk.HashBytes([]byte("payload"))
	backup := engine.BackupResult{
		Chunks: []chunk.ChunkRef{{
			Sequence:    1,
			Hash:        payloadHash,
			Key:         "chunks/aa/bb/payload",
			Offset:      0,
			Size:        7,
			StoredSize:  51,
			ETag:        "etag-1",
			Compression: "zstd",
			Encryption:  "aes-256-gcm",
			KeyID:       "key-1",
		}},
		Stats: chunk.Stats{
			Chunks:         2,
			BytesIn:        14,
			UploadedChunks: 1,
			DedupedChunks:  1,
			BytesUploaded:  51,
		},
	}

	result, err := CommitManifest(context.Background(), backend, backup, CommitOptions{
		BackupID:   "backup-1",
		Target:     "prod-redis",
		Driver:     manifest.Driver{Name: "redis", Version: "7.2"},
		Type:       core.BackupTypeFull,
		StartedAt:  finished.Add(-time.Minute),
		FinishedAt: finished,
		Encryption: manifest.Encryption{Algorithm: "aes-256-gcm", KeyID: "key-1"},
		PrivateKey: privateKey,
	})
	if err != nil {
		t.Fatalf("CommitManifest() error = %v", err)
	}
	if result.Key != "manifests/2026/04/23/backup-1.manifest" {
		t.Fatalf("manifest key = %q, want dated key", result.Key)
	}
	if result.Info.Key != result.Key {
		t.Fatalf("object info key = %q, want %q", result.Info.Key, result.Key)
	}

	rc, _, err := backend.Get(context.Background(), result.Key)
	if err != nil {
		t.Fatalf("Get(manifest) error = %v", err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		t.Fatalf("copy manifest: %v", err)
	}
	parsed, err := manifest.Parse(buf.Bytes())
	if err != nil {
		t.Fatalf("Parse(manifest) error = %v", err)
	}
	if err := parsed.Verify(publicKey); err != nil {
		t.Fatalf("Verify(manifest) error = %v", err)
	}
	if parsed.Stats.DedupRatio != 0.5 {
		t.Fatalf("dedup ratio = %v, want 0.5", parsed.Stats.DedupRatio)
	}
	if len(parsed.Objects) != 1 || len(parsed.Objects[0].Chunks) != 1 {
		t.Fatalf("manifest objects = %#v, want one stream with one chunk", parsed.Objects)
	}
	if parsed.Objects[0].Chunks[0].Hash != payloadHash.String() {
		t.Fatalf("chunk hash = %q, want %q", parsed.Objects[0].Chunks[0].Hash, payloadHash.String())
	}

	loaded, info, err := LoadManifest(context.Background(), backend, result.Key, publicKey)
	if err != nil {
		t.Fatalf("LoadManifest() error = %v", err)
	}
	if info.Key != result.Key {
		t.Fatalf("loaded info key = %q, want %q", info.Key, result.Key)
	}
	if loaded.BackupID != "backup-1" {
		t.Fatalf("loaded backup id = %q, want backup-1", loaded.BackupID)
	}
}

func TestCommitManifestValidatesRequiredInput(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	backend := storagetest.NewMemoryBackend("repo")

	tests := []struct {
		name    string
		backend storage.Backend
		opts    CommitOptions
	}{
		{name: "backend", backend: nil, opts: CommitOptions{BackupID: "backup-1", Target: "target", PrivateKey: privateKey}},
		{name: "backup id", backend: backend, opts: CommitOptions{Target: "target", PrivateKey: privateKey}},
		{name: "target", backend: backend, opts: CommitOptions{BackupID: "backup-1", PrivateKey: privateKey}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := CommitManifest(context.Background(), tt.backend, engine.BackupResult{}, tt.opts); err == nil {
				t.Fatal("CommitManifest() error = nil, want error")
			}
		})
	}
}
