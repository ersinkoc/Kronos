package manifest

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/chunk"
	"github.com/kronos/kronos/internal/core"
)

func TestManifestSignVerifyParse(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	started := time.Date(2026, 4, 23, 2, 0, 0, 0, time.UTC)
	m := New()
	m.BackupID = "backup-1"
	m.Target = "prod-postgres"
	m.Driver = Driver{Name: "postgres", Version: "17.2", SourceTZ: "UTC"}
	m.Type = core.BackupTypeFull
	m.StartedAt = started
	m.FinishedAt = started.Add(7 * time.Minute)
	m.Compression = "none"
	m.Encryption = Encryption{Algorithm: "aes-256-gcm", KeyID: "k7", KDF: "argon2id"}
	m.Objects = []Object{{
		Schema: "public",
		Name:   "users",
		Chunks: []ChunkRef{ChunkFromPipeline(chunk.ChunkRef{
			Hash:       chunk.HashBytes([]byte("users")),
			Key:        "data/aa/bb/hash",
			Offset:     0,
			Size:       5,
			StoredSize: 64,
			Encryption: "aes-256-gcm",
			KeyID:      "k7",
		})},
	}}
	m.Stats = Stats{LogicalSizeBytes: 5, StoredSizeBytes: 64, ChunkCount: 1}

	if err := m.Sign(privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if err := m.Verify(publicKey); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	data, err := m.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := parsed.Verify(publicKey); err != nil {
		t.Fatalf("Verify(parsed) error = %v", err)
	}
}

func TestManifestVerifyRejectsTamper(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	m := New()
	m.BackupID = "backup-1"
	m.Target = "target"
	m.Driver = Driver{Name: "postgres"}
	m.Type = core.BackupTypeFull
	m.StartedAt = time.Now().UTC()
	m.FinishedAt = m.StartedAt
	m.Encryption = Encryption{Algorithm: "aes-256-gcm", KeyID: "k1"}
	if err := m.Sign(privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	m.Target = "other-target"
	if err := m.Verify(publicKey); err == nil {
		t.Fatal("Verify(tampered) error = nil, want error")
	}
}

func TestManifestParseRejectsVersion(t *testing.T) {
	t.Parallel()

	if _, err := Parse([]byte(`{"kronos_manifest":99}`)); err == nil {
		t.Fatal("Parse(version 99) error = nil, want error")
	}
}

func TestPipelineRefsRoundTripAndRejectsBadHash(t *testing.T) {
	t.Parallel()

	ref := ChunkFromPipeline(chunk.ChunkRef{
		Sequence:    12,
		Hash:        chunk.HashBytes([]byte("payload")),
		Key:         "chunks/aa/hash",
		Offset:      9,
		Size:        128,
		StoredSize:  96,
		ETag:        "etag",
		Compression: "gzip",
		Encryption:  "aes-256-gcm",
		KeyID:       "key-1",
	})
	refs, err := PipelineRefs([]ChunkRef{ref})
	if err != nil {
		t.Fatalf("PipelineRefs() error = %v", err)
	}
	if len(refs) != 1 || refs[0].Sequence != 0 || refs[0].Hash.String() != ref.Hash || refs[0].Compression != "gzip" {
		t.Fatalf("PipelineRefs() = %#v", refs)
	}

	_, err = PipelineRefs([]ChunkRef{{Hash: "not-a-hash"}})
	if err == nil || !strings.Contains(err.Error(), "parse chunk 0 hash") {
		t.Fatalf("PipelineRefs(bad hash) error = %v", err)
	}
}
