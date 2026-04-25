package chunk

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	kcompress "github.com/kronos/kronos/internal/compress"
	kcrypto "github.com/kronos/kronos/internal/crypto"
	"github.com/kronos/kronos/internal/storage/storagetest"
)

func BenchmarkPipelineFeed(b *testing.B) {
	data := deterministicData(32 * 1024 * 1024)
	chunker, err := NewFastCDC(64*1024, 256*1024, 1024*1024)
	if err != nil {
		b.Fatalf("NewFastCDC() error = %v", err)
	}
	compressor, err := kcompress.New(kcompress.AlgorithmNone)
	if err != nil {
		b.Fatalf("compress.New() error = %v", err)
	}
	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{5}, 32))
	if err != nil {
		b.Fatalf("NewAES256GCM() error = %v", err)
	}

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend := storagetest.NewMemoryBackend(fmt.Sprintf("memory-%d", i))
		pipeline := &Pipeline{
			Chunker:     chunker,
			Compressor:  compressor,
			Cipher:      cipher,
			KeyID:       "bench-key",
			Backend:     backend,
			Concurrency: 4,
		}
		if _, _, err := pipeline.Feed(context.Background(), bytes.NewReader(data)); err != nil {
			b.Fatalf("Feed() error = %v", err)
		}
	}
}
