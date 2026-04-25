package compress

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
)

func TestCompressorsRoundTrip(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte("Time devours. Kronos preserves.\n"), 4096)
	for _, algorithm := range []Algorithm{AlgorithmNone, AlgorithmGzip, AlgorithmZstd} {
		algorithm := algorithm
		t.Run(string(algorithm), func(t *testing.T) {
			t.Parallel()

			compressor, err := New(algorithm)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			var encoded bytes.Buffer
			writer, err := compressor.NewWriter(&encoded)
			if err != nil {
				t.Fatalf("NewWriter() error = %v", err)
			}
			if _, err := writer.Write(payload); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			reader, err := compressor.NewReader(bytes.NewReader(encoded.Bytes()))
			if err != nil {
				t.Fatalf("NewReader() error = %v", err)
			}
			defer reader.Close()
			var decoded bytes.Buffer
			if _, err := io.Copy(&decoded, reader); err != nil {
				t.Fatalf("Copy(decoded) error = %v", err)
			}
			if !bytes.Equal(decoded.Bytes(), payload) {
				t.Fatal("decoded payload mismatch")
			}
		})
	}
}

func TestSelectAlgorithm(t *testing.T) {
	t.Parallel()

	compressible := bytes.Repeat([]byte{0}, 8192)
	if got := SelectAlgorithm(compressible); got != AlgorithmZstd {
		t.Fatalf("SelectAlgorithm(compressible) = %s, want zstd", got)
	}

	random := make([]byte, 8192)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	if got := SelectAlgorithm(random); got != AlgorithmNone {
		t.Fatalf("SelectAlgorithm(random) = %s, want none", got)
	}
}

func TestPrepareAutoPreservesStream(t *testing.T) {
	t.Parallel()

	payload := bytes.Repeat([]byte("abc123"), 300000)
	algorithm, reader, err := PrepareAuto(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("PrepareAuto() error = %v", err)
	}
	if algorithm != AlgorithmZstd {
		t.Fatalf("PrepareAuto() algorithm = %s, want zstd", algorithm)
	}
	var got bytes.Buffer
	if _, err := io.Copy(&got, reader); err != nil {
		t.Fatalf("Copy() error = %v", err)
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Fatal("PrepareAuto() reader did not preserve input")
	}
}

func TestCompressorAlgorithmsAndRejectsUnsupported(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []Algorithm{AlgorithmNone, AlgorithmGzip, AlgorithmZstd} {
		compressor, err := New(algorithm)
		if err != nil {
			t.Fatalf("New(%s) error = %v", algorithm, err)
		}
		if got := compressor.Algorithm(); got != algorithm {
			t.Fatalf("Algorithm() = %s, want %s", got, algorithm)
		}
	}
	if _, err := New("brotli"); err == nil {
		t.Fatal("New(unsupported) error = nil, want error")
	}
	if _, err := (GzipCompressor{}).NewReader(bytes.NewReader([]byte("not gzip"))); err == nil {
		t.Fatal("Gzip NewReader(invalid) error = nil, want error")
	}
	zstdReader, err := (ZstdCompressor{}).NewReader(bytes.NewReader([]byte("not zstd")))
	if err != nil {
		return
	}
	defer zstdReader.Close()
	if _, err := io.Copy(io.Discard, zstdReader); err == nil {
		t.Fatal("Zstd reader over invalid stream error = nil, want error")
	}
}
