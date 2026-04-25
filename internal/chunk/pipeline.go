package chunk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"runtime"
	"sort"
	"sync"

	kcompress "github.com/kronos/kronos/internal/compress"
	kcrypto "github.com/kronos/kronos/internal/crypto"
	"github.com/kronos/kronos/internal/storage"
	"lukechampine.com/blake3"
)

const defaultMaxPipelineWorkers = 8

// ChunkFunc emits content chunks into out. It must stop promptly when ctx is canceled.
type ChunkFunc func(ctx context.Context, r io.Reader, out chan<- PipelineChunk) error

// HashFunc attaches a content hash and dedup decision to a chunk.
type HashFunc func(ctx context.Context, chunk PipelineChunk) (HashedChunk, error)

// EncodeFunc transforms a non-deduplicated chunk into the payload that upload workers store.
type EncodeFunc func(ctx context.Context, chunk HashedChunk) (EncodedChunk, error)

// UploadFunc stores an encoded chunk and returns its manifest reference.
type UploadFunc func(ctx context.Context, chunk EncodedChunk) (ChunkRef, error)

// Pipeline coordinates the bounded worker graph for chunk processing.
type Pipeline struct {
	Chunker         *FastCDC
	Compressor      kcompress.Compressor
	Cipher          kcrypto.Cipher
	KeyID           string
	Backend         storage.Backend
	Index           *Index
	Concurrency     int
	ChannelCapacity int

	ChunkFunc  ChunkFunc
	HashFunc   HashFunc
	EncodeFunc EncodeFunc
	UploadFunc UploadFunc
}

// PipelineChunk is one content-defined chunk moving through the pipeline.
type PipelineChunk struct {
	Sequence int64
	Offset   int64
	Size     int
	Data     []byte
}

// HashedChunk is a chunk after BLAKE3 identity and dedup lookup.
type HashedChunk struct {
	PipelineChunk
	Hash      Hash
	Duplicate bool
}

// EncodedChunk is a chunk after encode/encrypt preparation.
type EncodedChunk struct {
	HashedChunk
	Key         string
	Compression kcompress.Algorithm
	Encryption  string
	KeyID       string
	Payload     []byte
}

// ChunkRef is the manifest-ready reference emitted by Feed.
type ChunkRef struct {
	Sequence    int64
	Hash        Hash
	Key         string
	Offset      int64
	Size        int
	StoredSize  int64
	ETag        string
	Compression kcompress.Algorithm
	Encryption  string
	KeyID       string
	Duplicate   bool
}

// Stats summarizes one pipeline run.
type Stats struct {
	Chunks         int
	BytesIn        int64
	UploadedChunks int
	DedupedChunks  int
	BytesUploaded  int64
}

// Feed processes an input stream through chunk, hash, encode, and upload workers.
func (p *Pipeline) Feed(ctx context.Context, r io.Reader) ([]ChunkRef, Stats, error) {
	if p == nil {
		return nil, Stats{}, fmt.Errorf("pipeline is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if r == nil {
		return nil, Stats{}, fmt.Errorf("reader is required")
	}

	workers := p.workerCount()
	capacity := p.channelCapacity(workers)
	chunkFn := p.chunkFunc()
	hashFn := p.hashFunc()
	encodeFn := p.encodeFunc()
	uploadFn := p.uploadFunc()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	chunkCh := make(chan PipelineChunk, capacity)
	hashCh := make(chan HashedChunk, capacity)
	encodeCh := make(chan EncodedChunk, capacity)
	refCh := make(chan ChunkRef, capacity)
	errCh := make(chan error, 1)

	var once sync.Once
	var errMu sync.Mutex
	var firstErr error
	fail := func(err error) {
		if err == nil {
			return
		}
		once.Do(func() {
			errMu.Lock()
			firstErr = err
			errMu.Unlock()
			errCh <- err
			cancel()
		})
	}
	getErr := func() error {
		errMu.Lock()
		defer errMu.Unlock()
		return firstErr
	}

	go func() {
		defer close(chunkCh)
		if err := chunkFn(ctx, r, chunkCh); err != nil {
			fail(fmt.Errorf("chunk pipeline input: %w", err))
		}
	}()

	var hashWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		hashWG.Add(1)
		go func() {
			defer hashWG.Done()
			for chunk := range chunkCh {
				hashed, err := hashFn(ctx, chunk)
				if err != nil {
					fail(fmt.Errorf("hash chunk %d: %w", chunk.Sequence, err))
					return
				}
				if err := sendHashed(ctx, hashCh, hashed); err != nil {
					return
				}
			}
		}()
	}
	go func() {
		hashWG.Wait()
		close(hashCh)
	}()

	var encodeWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		encodeWG.Add(1)
		go func() {
			defer encodeWG.Done()
			for hashed := range hashCh {
				encoded, err := encodeFn(ctx, hashed)
				if err != nil {
					fail(fmt.Errorf("encode chunk %d: %w", hashed.Sequence, err))
					return
				}
				if err := sendEncoded(ctx, encodeCh, encoded); err != nil {
					return
				}
			}
		}()
	}
	go func() {
		encodeWG.Wait()
		close(encodeCh)
	}()

	var uploadWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		uploadWG.Add(1)
		go func() {
			defer uploadWG.Done()
			for encoded := range encodeCh {
				ref, err := uploadFn(ctx, encoded)
				if err != nil {
					fail(fmt.Errorf("upload chunk %d: %w", encoded.Sequence, err))
					return
				}
				if err := sendRef(ctx, refCh, ref); err != nil {
					return
				}
			}
		}()
	}
	go func() {
		uploadWG.Wait()
		close(refCh)
	}()

	var refs []ChunkRef
	var stats Stats
	for {
		select {
		case ref, ok := <-refCh:
			if !ok {
				if err := getErr(); err != nil {
					return nil, stats, err
				}
				sort.Slice(refs, func(i, j int) bool {
					return refs[i].Sequence < refs[j].Sequence
				})
				return refs, stats, nil
			}
			refs = append(refs, ref)
			stats.Chunks++
			stats.BytesIn += int64(ref.Size)
			if ref.Duplicate {
				stats.DedupedChunks++
			} else {
				stats.UploadedChunks++
				stats.BytesUploaded += ref.StoredSize
			}
		case err := <-errCh:
			cancel()
			for range refCh {
			}
			return nil, stats, err
		}
	}
}

// Restore reconstructs chunk references into w, verifying every chunk hash.
func (p *Pipeline) Restore(ctx context.Context, refs []ChunkRef, w io.Writer) (Stats, error) {
	if p == nil {
		return Stats{}, fmt.Errorf("pipeline is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if p.Backend == nil {
		return Stats{}, fmt.Errorf("storage backend is required")
	}
	if p.Cipher == nil {
		return Stats{}, fmt.Errorf("cipher is required")
	}
	if w == nil {
		return Stats{}, fmt.Errorf("writer is required")
	}

	ordered := append([]ChunkRef(nil), refs...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Sequence < ordered[j].Sequence
	})

	var stats Stats
	for _, ref := range ordered {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if ref.Key == "" {
			return stats, fmt.Errorf("chunk %d key is empty", ref.Sequence)
		}
		stream, _, err := p.Backend.Get(ctx, ref.Key)
		if err != nil {
			return stats, fmt.Errorf("get chunk %d: %w", ref.Sequence, err)
		}
		var envelope bytes.Buffer
		if _, err := io.Copy(&envelope, stream); err != nil {
			stream.Close()
			return stats, fmt.Errorf("read chunk %d: %w", ref.Sequence, err)
		}
		if err := stream.Close(); err != nil {
			return stats, fmt.Errorf("close chunk %d: %w", ref.Sequence, err)
		}
		compressed, _, err := OpenEnvelope(p.Cipher, envelope.Bytes(), ref.Hash[:])
		if err != nil {
			return stats, fmt.Errorf("decrypt chunk %d: %w", ref.Sequence, err)
		}
		compressor, err := p.compressorFor(ref.Compression)
		if err != nil {
			return stats, fmt.Errorf("chunk %d compressor: %w", ref.Sequence, err)
		}
		reader, err := compressor.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return stats, fmt.Errorf("open chunk %d compression: %w", ref.Sequence, err)
		}
		hash, size, err := copyAndHash(w, reader)
		if closeErr := reader.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			return stats, fmt.Errorf("restore chunk %d: %w", ref.Sequence, err)
		}
		if hash != ref.Hash {
			return stats, fmt.Errorf("chunk %d hash mismatch", ref.Sequence)
		}
		if ref.Size >= 0 && size != int64(ref.Size) {
			return stats, fmt.Errorf("chunk %d size mismatch: got %d want %d", ref.Sequence, size, ref.Size)
		}
		stats.Chunks++
		stats.BytesIn += size
	}
	return stats, nil
}

func (p *Pipeline) workerCount() int {
	if p.Concurrency > 0 {
		return p.Concurrency
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		return 1
	}
	if workers > defaultMaxPipelineWorkers {
		return defaultMaxPipelineWorkers
	}
	return workers
}

func (p *Pipeline) channelCapacity(workers int) int {
	if p.ChannelCapacity > 0 {
		return p.ChannelCapacity
	}
	if workers < 1 {
		workers = 1
	}
	return workers * 2
}

func (p *Pipeline) compressorFor(algorithm kcompress.Algorithm) (kcompress.Compressor, error) {
	if algorithm == "" {
		if p.Compressor != nil {
			return p.Compressor, nil
		}
		return kcompress.New(kcompress.AlgorithmNone)
	}
	return kcompress.New(algorithm)
}

func (p *Pipeline) chunkFunc() ChunkFunc {
	if p.ChunkFunc != nil {
		return p.ChunkFunc
	}
	chunker := p.Chunker
	if chunker == nil {
		chunker = NewDefaultFastCDC()
	}
	return func(ctx context.Context, r io.Reader, out chan<- PipelineChunk) error {
		var seq int64
		return chunker.ForEach(r, func(chunk Chunk) error {
			item := PipelineChunk{
				Sequence: seq,
				Offset:   chunk.Offset,
				Size:     chunk.Size,
				Data:     chunk.Data,
			}
			if err := sendChunk(ctx, out, item); err != nil {
				return err
			}
			seq++
			return nil
		})
	}
}

func (p *Pipeline) hashFunc() HashFunc {
	if p.HashFunc != nil {
		return p.HashFunc
	}
	seen := make(map[Hash]struct{})
	var seenMu sync.Mutex
	return func(ctx context.Context, chunk PipelineChunk) (HashedChunk, error) {
		if err := ctx.Err(); err != nil {
			return HashedChunk{}, err
		}
		hash := HashBytes(chunk.Data)
		duplicate := false
		if p.Index != nil {
			duplicate = !p.Index.Add(hash)
		} else {
			seenMu.Lock()
			_, duplicate = seen[hash]
			if !duplicate {
				seen[hash] = struct{}{}
			}
			seenMu.Unlock()
		}
		return HashedChunk{
			PipelineChunk: chunk,
			Hash:          hash,
			Duplicate:     duplicate,
		}, nil
	}
}

func (p *Pipeline) encodeFunc() EncodeFunc {
	if p.EncodeFunc != nil {
		return p.EncodeFunc
	}
	return func(ctx context.Context, chunk HashedChunk) (EncodedChunk, error) {
		if err := ctx.Err(); err != nil {
			return EncodedChunk{}, err
		}
		compressor := p.Compressor
		if compressor == nil {
			var err error
			compressor, err = kcompress.New(kcompress.AlgorithmNone)
			if err != nil {
				return EncodedChunk{}, err
			}
		}
		if chunk.Duplicate {
			encoded := EncodedChunk{
				HashedChunk: chunk,
				Key:         contentKey(chunk.Hash),
				Compression: compressor.Algorithm(),
			}
			if p.Cipher != nil {
				encoded.Encryption = p.Cipher.Algorithm()
				encoded.KeyID = p.KeyID
			}
			return encoded, nil
		}
		if p.Cipher == nil {
			return EncodedChunk{}, fmt.Errorf("cipher is required")
		}
		if p.KeyID == "" {
			return EncodedChunk{}, fmt.Errorf("key id is required")
		}
		var compressed bytes.Buffer
		writer, err := compressor.NewWriter(&compressed)
		if err != nil {
			return EncodedChunk{}, err
		}
		if _, err := writer.Write(chunk.Data); err != nil {
			writer.Close()
			return EncodedChunk{}, err
		}
		if err := writer.Close(); err != nil {
			return EncodedChunk{}, err
		}
		payload, err := SealEnvelope(p.Cipher, p.KeyID, compressed.Bytes(), chunk.Hash[:])
		if err != nil {
			return EncodedChunk{}, err
		}
		return EncodedChunk{
			HashedChunk: chunk,
			Key:         contentKey(chunk.Hash),
			Compression: compressor.Algorithm(),
			Encryption:  p.Cipher.Algorithm(),
			KeyID:       p.KeyID,
			Payload:     payload,
		}, nil
	}
}

func (p *Pipeline) uploadFunc() UploadFunc {
	if p.UploadFunc != nil {
		return p.UploadFunc
	}
	return func(ctx context.Context, chunk EncodedChunk) (ChunkRef, error) {
		ref := ChunkRef{
			Sequence:    chunk.Sequence,
			Hash:        chunk.Hash,
			Key:         chunk.Key,
			Offset:      chunk.Offset,
			Size:        chunk.Size,
			Compression: chunk.Compression,
			Encryption:  chunk.Encryption,
			KeyID:       chunk.KeyID,
			Duplicate:   chunk.Duplicate,
		}
		if chunk.Duplicate {
			return ref, nil
		}
		if p.Backend == nil {
			return ChunkRef{}, fmt.Errorf("storage backend is required")
		}
		info, err := p.Backend.Put(ctx, chunk.Key, bytes.NewReader(chunk.Payload), int64(len(chunk.Payload)))
		if err != nil {
			return ChunkRef{}, err
		}
		ref.StoredSize = info.Size
		ref.ETag = info.ETag
		return ref, nil
	}
}

func sendChunk(ctx context.Context, out chan<- PipelineChunk, chunk PipelineChunk) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- chunk:
		return nil
	}
}

func sendHashed(ctx context.Context, out chan<- HashedChunk, chunk HashedChunk) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- chunk:
		return nil
	}
}

func sendEncoded(ctx context.Context, out chan<- EncodedChunk, chunk EncodedChunk) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- chunk:
		return nil
	}
}

func sendRef(ctx context.Context, out chan<- ChunkRef, ref ChunkRef) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- ref:
		return nil
	}
}

func contentKey(hash Hash) string {
	value := hash.String()
	return "data/" + value[:2] + "/" + value[2:4] + "/" + value
}

func copyAndHash(w io.Writer, r io.Reader) (Hash, int64, error) {
	hasher := blake3.New(HashSize, nil)
	written, err := io.Copy(io.MultiWriter(w, hasher), r)
	if err != nil {
		return Hash{}, written, err
	}
	var hash Hash
	copy(hash[:], hasher.Sum(nil))
	return hash, written, nil
}
