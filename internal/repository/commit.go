package repository

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"time"

	"github.com/kronos/kronos/internal/chunk"
	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/engine"
	"github.com/kronos/kronos/internal/manifest"
	"github.com/kronos/kronos/internal/storage"
)

const maxManifestBytes = 64 * 1024 * 1024

// CommitOptions describes one manifest commit.
type CommitOptions struct {
	BackupID   string
	Target     string
	Driver     manifest.Driver
	Type       core.BackupType
	ParentID   *string
	StartedAt  time.Time
	FinishedAt time.Time
	Encryption manifest.Encryption
	PrivateKey ed25519.PrivateKey
}

// CommitResult is returned after manifest bytes are written.
type CommitResult struct {
	Manifest manifest.Manifest
	Key      string
	Info     storage.ObjectInfo
}

// CommitManifest signs and writes a manifest pointer JSON file to storage.
func CommitManifest(ctx context.Context, backend storage.Backend, backup engine.BackupResult, opts CommitOptions) (CommitResult, error) {
	if backend == nil {
		return CommitResult{}, fmt.Errorf("storage backend is required")
	}
	if opts.BackupID == "" {
		return CommitResult{}, fmt.Errorf("backup id is required")
	}
	if opts.Target == "" {
		return CommitResult{}, fmt.Errorf("target is required")
	}
	m := manifest.New()
	m.BackupID = opts.BackupID
	m.Target = opts.Target
	m.Driver = opts.Driver
	m.Type = opts.Type
	m.ParentID = opts.ParentID
	m.StartedAt = opts.StartedAt
	m.FinishedAt = opts.FinishedAt
	m.Encryption = opts.Encryption
	m.Stats = manifest.Stats{
		LogicalSizeBytes: backup.Stats.BytesIn,
		StoredSizeBytes:  backup.Stats.BytesUploaded,
		ChunkCount:       backup.Stats.Chunks,
		DedupRatio:       dedupRatio(backup.Stats),
	}
	chunks := make([]manifest.ChunkRef, 0, len(backup.Chunks))
	for _, ref := range backup.Chunks {
		chunks = append(chunks, manifest.ChunkFromPipeline(ref))
	}
	m.Objects = []manifest.Object{{Name: "stream", Chunks: chunks}}
	if err := m.Sign(opts.PrivateKey); err != nil {
		return CommitResult{}, err
	}
	data, err := m.Marshal()
	if err != nil {
		return CommitResult{}, err
	}
	key := manifestKey(opts.FinishedAt, opts.BackupID)
	info, err := backend.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Manifest: m, Key: key, Info: info}, nil
}

// LoadManifest reads, parses, and verifies a committed manifest object.
func LoadManifest(ctx context.Context, backend storage.Backend, key string, publicKey ed25519.PublicKey) (manifest.Manifest, storage.ObjectInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if backend == nil {
		return manifest.Manifest{}, storage.ObjectInfo{}, fmt.Errorf("storage backend is required")
	}
	if key == "" {
		return manifest.Manifest{}, storage.ObjectInfo{}, fmt.Errorf("manifest key is required")
	}
	rc, info, err := backend.Get(ctx, key)
	if err != nil {
		return manifest.Manifest{}, storage.ObjectInfo{}, err
	}
	defer rc.Close()

	var buf bytes.Buffer
	written, err := io.Copy(&buf, io.LimitReader(rc, maxManifestBytes+1))
	if err != nil {
		return manifest.Manifest{}, storage.ObjectInfo{}, err
	}
	if written > maxManifestBytes {
		return manifest.Manifest{}, storage.ObjectInfo{}, fmt.Errorf("manifest %q exceeds %d bytes", key, maxManifestBytes)
	}
	m, err := manifest.Parse(buf.Bytes())
	if err != nil {
		return manifest.Manifest{}, storage.ObjectInfo{}, err
	}
	if err := m.Verify(publicKey); err != nil {
		return manifest.Manifest{}, storage.ObjectInfo{}, err
	}
	return m, info, nil
}

func manifestKey(finishedAt time.Time, backupID string) string {
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	finishedAt = finishedAt.UTC()
	return fmt.Sprintf("manifests/%04d/%02d/%02d/%s.manifest", finishedAt.Year(), finishedAt.Month(), finishedAt.Day(), backupID)
}

func dedupRatio(stats chunk.Stats) float64 {
	if stats.Chunks == 0 {
		return 0
	}
	return float64(stats.DedupedChunks) / float64(stats.Chunks)
}
