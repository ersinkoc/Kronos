package manifest

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kronos/kronos/internal/chunk"
	kcompress "github.com/kronos/kronos/internal/compress"
	"github.com/kronos/kronos/internal/core"
)

const Version = 1

// Manifest is the signed metadata for one committed backup.
type Manifest struct {
	Version     int               `json:"kronos_manifest"`
	BackupID    string            `json:"backup_id"`
	Target      string            `json:"target"`
	Driver      Driver            `json:"driver"`
	Type        core.BackupType   `json:"type"`
	ParentID    *string           `json:"parent_id"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Compression string            `json:"compression,omitempty"`
	Encryption  Encryption        `json:"encryption"`
	Stats       Stats             `json:"stats"`
	Objects     []Object          `json:"objects"`
	Streams     map[string]string `json:"streams,omitempty"`
	Signature   *Signature        `json:"signature,omitempty"`
}

// Driver identifies the backup driver that produced the manifest.
type Driver struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	SourceTZ string `json:"source_tz,omitempty"`
}

// Encryption describes the manifest/chunk encryption configuration.
type Encryption struct {
	Algorithm string `json:"algo"`
	KeyID     string `json:"key_id"`
	KDF       string `json:"kdf,omitempty"`
}

// Stats records manifest-level size and deduplication counters.
type Stats struct {
	LogicalSizeBytes int64   `json:"logical_size_bytes"`
	StoredSizeBytes  int64   `json:"stored_size_bytes"`
	ChunkCount       int     `json:"chunk_count"`
	DedupRatio       float64 `json:"dedup_ratio"`
}

// Object maps one logical database object to ordered chunks.
type Object struct {
	Schema string     `json:"schema,omitempty"`
	Name   string     `json:"name"`
	Chunks []ChunkRef `json:"chunks"`
}

// ChunkRef is the manifest representation of a stored chunk.
type ChunkRef struct {
	Hash        string `json:"hash"`
	Key         string `json:"key"`
	Offset      int64  `json:"offset"`
	Size        int    `json:"size"`
	StoredSize  int64  `json:"stored_size"`
	ETag        string `json:"etag,omitempty"`
	Compression string `json:"compression,omitempty"`
	Encryption  string `json:"encryption,omitempty"`
	KeyID       string `json:"key_id,omitempty"`
}

// Signature is an Ed25519 signature over canonical manifest JSON.
type Signature struct {
	Algorithm string `json:"algo"`
	Value     string `json:"value"`
}

// New returns a manifest with Kronos' current manifest version.
func New() Manifest {
	return Manifest{Version: Version}
}

// ChunkFromPipeline maps a chunk pipeline reference into manifest metadata.
func ChunkFromPipeline(ref chunk.ChunkRef) ChunkRef {
	return ChunkRef{
		Hash:        ref.Hash.String(),
		Key:         ref.Key,
		Offset:      ref.Offset,
		Size:        ref.Size,
		StoredSize:  ref.StoredSize,
		ETag:        ref.ETag,
		Compression: string(ref.Compression),
		Encryption:  ref.Encryption,
		KeyID:       ref.KeyID,
	}
}

// PipelineRefs maps ordered manifest chunk references back to pipeline references.
func PipelineRefs(refs []ChunkRef) ([]chunk.ChunkRef, error) {
	out := make([]chunk.ChunkRef, 0, len(refs))
	for i, ref := range refs {
		hash, err := chunk.ParseHash(ref.Hash)
		if err != nil {
			return nil, fmt.Errorf("parse chunk %d hash: %w", i, err)
		}
		out = append(out, chunk.ChunkRef{
			Sequence:    int64(i),
			Hash:        hash,
			Key:         ref.Key,
			Offset:      ref.Offset,
			Size:        ref.Size,
			StoredSize:  ref.StoredSize,
			ETag:        ref.ETag,
			Compression: kcompress.Algorithm(ref.Compression),
			Encryption:  ref.Encryption,
			KeyID:       ref.KeyID,
		})
	}
	return out, nil
}

// Sign signs m in canonical form and stores the signature.
func (m *Manifest) Sign(privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("ed25519 private key must be %d bytes", ed25519.PrivateKeySize)
	}
	canonical, err := m.CanonicalJSON()
	if err != nil {
		return err
	}
	sig := ed25519.Sign(privateKey, canonical)
	m.Signature = &Signature{
		Algorithm: "ed25519",
		Value:     hex.EncodeToString(sig),
	}
	return nil
}

// Verify checks m's stored Ed25519 signature.
func (m Manifest) Verify(publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("ed25519 public key must be %d bytes", ed25519.PublicKeySize)
	}
	if m.Signature == nil {
		return fmt.Errorf("manifest signature is missing")
	}
	if m.Signature.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm %q", m.Signature.Algorithm)
	}
	sig, err := hex.DecodeString(m.Signature.Value)
	if err != nil {
		return fmt.Errorf("decode manifest signature: %w", err)
	}
	canonical, err := m.CanonicalJSON()
	if err != nil {
		return err
	}
	if !ed25519.Verify(publicKey, canonical, sig) {
		return fmt.Errorf("manifest signature verification failed")
	}
	return nil
}

// CanonicalJSON returns stable JSON with the signature field omitted.
func (m Manifest) CanonicalJSON() ([]byte, error) {
	m.Signature = nil
	return json.Marshal(m)
}

// Marshal returns pretty JSON for storage or debugging.
func (m Manifest) Marshal() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// Parse decodes manifest JSON and validates the manifest version.
func Parse(data []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if m.Version != Version {
		return Manifest{}, fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	return m, nil
}
