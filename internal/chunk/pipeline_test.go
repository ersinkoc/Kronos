package chunk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	kcompress "github.com/kronos/kronos/internal/compress"
	kcrypto "github.com/kronos/kronos/internal/crypto"
	"github.com/kronos/kronos/internal/storage/storagetest"
)

func TestPipelineFeedWithFakeStages(t *testing.T) {
	t.Parallel()

	pipeline := &Pipeline{
		Concurrency:     2,
		ChannelCapacity: 2,
		ChunkFunc: func(ctx context.Context, r io.Reader, out chan<- PipelineChunk) error {
			chunks := []string{"alpha", "beta", "alpha", "gamma"}
			var offset int64
			for i, text := range chunks {
				data := []byte(text)
				item := PipelineChunk{
					Sequence: int64(i),
					Offset:   offset,
					Size:     len(data),
					Data:     data,
				}
				if err := sendChunk(ctx, out, item); err != nil {
					return err
				}
				offset += int64(len(data))
			}
			return nil
		},
		HashFunc: func(ctx context.Context, chunk PipelineChunk) (HashedChunk, error) {
			hash := HashBytes(chunk.Data)
			return HashedChunk{
				PipelineChunk: chunk,
				Hash:          hash,
				Duplicate:     chunk.Sequence == 2,
			}, nil
		},
		EncodeFunc: func(ctx context.Context, chunk HashedChunk) (EncodedChunk, error) {
			if chunk.Duplicate {
				return EncodedChunk{HashedChunk: chunk, Key: contentKey(chunk.Hash)}, nil
			}
			payload := append([]byte("encoded:"), chunk.Data...)
			return EncodedChunk{
				HashedChunk: chunk,
				Key:         contentKey(chunk.Hash),
				Payload:     payload,
			}, nil
		},
		UploadFunc: func(ctx context.Context, chunk EncodedChunk) (ChunkRef, error) {
			return ChunkRef{
				Sequence:   chunk.Sequence,
				Hash:       chunk.Hash,
				Key:        chunk.Key,
				Offset:     chunk.Offset,
				Size:       chunk.Size,
				StoredSize: int64(len(chunk.Payload)),
				ETag:       fmt.Sprintf("etag-%d", chunk.Sequence),
				Duplicate:  chunk.Duplicate,
			}, nil
		},
	}

	refs, stats, err := pipeline.Feed(context.Background(), bytes.NewReader([]byte("ignored")))
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	if len(refs) != 4 {
		t.Fatalf("len(refs) = %d, want 4", len(refs))
	}
	for i, ref := range refs {
		if ref.Sequence != int64(i) {
			t.Fatalf("refs[%d].Sequence = %d, want %d", i, ref.Sequence, i)
		}
		if ref.Key == "" {
			t.Fatalf("refs[%d].Key is empty", i)
		}
	}
	if !refs[2].Duplicate {
		t.Fatal("refs[2].Duplicate = false, want true")
	}
	if stats.Chunks != 4 || stats.UploadedChunks != 3 || stats.DedupedChunks != 1 {
		t.Fatalf("Stats() = %#v", stats)
	}
	if stats.BytesIn != int64(len("alpha")+len("beta")+len("alpha")+len("gamma")) {
		t.Fatalf("BytesIn = %d, want input chunk bytes", stats.BytesIn)
	}
}

func TestPipelineDefaultStagesUploadAndDedup(t *testing.T) {
	t.Parallel()

	backend := storagetest.NewMemoryBackend("memory")
	index, err := NewIndex(8)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	chunker, err := NewFastCDC(4, 8, 16)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}
	compressor, err := kcompress.New(kcompress.AlgorithmNone)
	if err != nil {
		t.Fatalf("compress.New() error = %v", err)
	}
	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	pipeline := &Pipeline{
		Chunker:     chunker,
		Compressor:  compressor,
		Cipher:      cipher,
		KeyID:       "test-key",
		Backend:     backend,
		Index:       index,
		Concurrency: 2,
	}

	input := []byte("abcabcabcabcabcabc")
	refs, stats, err := pipeline.Feed(context.Background(), bytes.NewReader(input))
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	if len(refs) == 0 {
		t.Fatal("Feed() returned no refs")
	}
	page, err := backend.List(context.Background(), "data/", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if stats.UploadedChunks != len(page.Objects) {
		t.Fatalf("UploadedChunks = %d, stored objects = %d", stats.UploadedChunks, len(page.Objects))
	}
	for _, ref := range refs {
		if ref.Key == "" || ref.Hash == (Hash{}) {
			t.Fatalf("invalid ref: %#v", ref)
		}
		if ref.Compression != kcompress.AlgorithmNone || ref.Encryption != kcrypto.AlgorithmAES256GCM || ref.KeyID != "test-key" {
			t.Fatalf("unexpected ref codec fields: %#v", ref)
		}
	}

	var got bytes.Buffer
	restoreStats, err := pipeline.Restore(context.Background(), refs, &got)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if !bytes.Equal(got.Bytes(), input) {
		t.Fatalf("restored payload = %q, want %q", got.Bytes(), input)
	}
	if restoreStats.Chunks != len(refs) || restoreStats.BytesIn != int64(len(input)) {
		t.Fatalf("Restore() stats = %#v", restoreStats)
	}
}

func TestPipelineRestoreVerifiesHash(t *testing.T) {
	t.Parallel()

	backend := storagetest.NewMemoryBackend("memory")
	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{9}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	pipeline := &Pipeline{
		Chunker:     mustChunker(t, 4, 8, 16),
		Compressor:  mustCompressor(t, kcompress.AlgorithmNone),
		Cipher:      cipher,
		KeyID:       "test-key",
		Backend:     backend,
		Concurrency: 1,
	}
	refs, _, err := pipeline.Feed(context.Background(), bytes.NewReader([]byte("restore me")))
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	refs[0].Hash = HashBytes([]byte("wrong"))

	var got bytes.Buffer
	if _, err := pipeline.Restore(context.Background(), refs, &got); err == nil {
		t.Fatal("Restore() error = nil, want hash mismatch")
	}
}

func TestPipelineDedupsWithinRunWithoutIndex(t *testing.T) {
	t.Parallel()

	backend := storagetest.NewMemoryBackend("memory")
	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	pipeline := &Pipeline{
		Compressor:  mustCompressor(t, kcompress.AlgorithmNone),
		Cipher:      cipher,
		KeyID:       "test-key",
		Backend:     backend,
		Concurrency: 2,
		ChunkFunc: func(ctx context.Context, r io.Reader, out chan<- PipelineChunk) error {
			for i := 0; i < 2; i++ {
				chunk := PipelineChunk{
					Sequence: int64(i),
					Offset:   int64(i * 4),
					Size:     4,
					Data:     []byte("same"),
				}
				if err := sendChunk(ctx, out, chunk); err != nil {
					return err
				}
			}
			return nil
		},
	}

	refs, stats, err := pipeline.Feed(context.Background(), bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("Feed() error = %v", err)
	}
	if len(refs) != 2 || stats.UploadedChunks != 1 || stats.DedupedChunks != 1 {
		t.Fatalf("refs=%d stats=%#v, want one upload and one dedup", len(refs), stats)
	}
	page, err := backend.List(context.Background(), "data/", "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page.Objects) != 1 {
		t.Fatalf("stored objects = %d, want 1", len(page.Objects))
	}
}

func TestPipelineReturnsFirstStageError(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	pipeline := &Pipeline{
		Concurrency: 2,
		ChunkFunc: func(ctx context.Context, r io.Reader, out chan<- PipelineChunk) error {
			for i := 0; i < 10; i++ {
				item := PipelineChunk{Sequence: int64(i), Size: 1, Data: []byte{byte(i)}}
				if err := sendChunk(ctx, out, item); err != nil {
					return err
				}
			}
			return nil
		},
		HashFunc: func(ctx context.Context, chunk PipelineChunk) (HashedChunk, error) {
			if chunk.Sequence == 3 {
				return HashedChunk{}, want
			}
			return HashedChunk{PipelineChunk: chunk, Hash: HashBytes(chunk.Data)}, nil
		},
		EncodeFunc: func(ctx context.Context, chunk HashedChunk) (EncodedChunk, error) {
			return EncodedChunk{HashedChunk: chunk, Payload: chunk.Data}, nil
		},
		UploadFunc: func(ctx context.Context, chunk EncodedChunk) (ChunkRef, error) {
			return ChunkRef{
				Sequence:   chunk.Sequence,
				Hash:       chunk.Hash,
				Size:       chunk.Size,
				StoredSize: int64(len(chunk.Payload)),
			}, nil
		},
	}

	_, _, err := pipeline.Feed(context.Background(), bytes.NewReader(nil))
	if !errors.Is(err, want) {
		t.Fatalf("Feed() error = %v, want %v", err, want)
	}
}

func mustChunker(t *testing.T, min int, avg int, max int) *FastCDC {
	t.Helper()
	chunker, err := NewFastCDC(min, avg, max)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}
	return chunker
}

func mustCompressor(t *testing.T, algorithm kcompress.Algorithm) kcompress.Compressor {
	t.Helper()
	compressor, err := kcompress.New(algorithm)
	if err != nil {
		t.Fatalf("compress.New() error = %v", err)
	}
	return compressor
}

func TestPipelineSizing(t *testing.T) {
	t.Parallel()

	pipeline := &Pipeline{Concurrency: 3}
	if got := pipeline.workerCount(); got != 3 {
		t.Fatalf("workerCount() = %d, want 3", got)
	}
	if got := pipeline.channelCapacity(3); got != 6 {
		t.Fatalf("channelCapacity() = %d, want 6", got)
	}
	pipeline.ChannelCapacity = 5
	if got := pipeline.channelCapacity(3); got != 5 {
		t.Fatalf("channelCapacity(override) = %d, want 5", got)
	}
}
