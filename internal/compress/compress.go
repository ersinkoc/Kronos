package compress

import (
	"fmt"
	"io"
)

// Algorithm names a supported compression algorithm.
type Algorithm string

const (
	// AlgorithmNone stores data without compression.
	AlgorithmNone Algorithm = "none"
	// AlgorithmGzip uses the Go standard library gzip implementation.
	AlgorithmGzip Algorithm = "gzip"
	// AlgorithmZstd uses klauspost/compress zstd.
	AlgorithmZstd Algorithm = "zstd"
)

// Compressor creates streaming encoders and decoders.
type Compressor interface {
	Algorithm() Algorithm
	NewWriter(io.Writer) (io.WriteCloser, error)
	NewReader(io.Reader) (io.ReadCloser, error)
}

// New returns a compressor for algorithm.
func New(algorithm Algorithm) (Compressor, error) {
	switch algorithm {
	case AlgorithmNone:
		return noneCompressor{}, nil
	case AlgorithmGzip:
		return GzipCompressor{Level: DefaultGzipLevel}, nil
	case AlgorithmZstd:
		return ZstdCompressor{Level: DefaultZstdLevel}, nil
	default:
		return nil, fmt.Errorf("unsupported compression algorithm %q", algorithm)
	}
}

type noneCompressor struct{}

func (noneCompressor) Algorithm() Algorithm {
	return AlgorithmNone
}

func (noneCompressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	return nopWriteCloser{Writer: w}, nil
}

func (noneCompressor) NewReader(r io.Reader) (io.ReadCloser, error) {
	return io.NopCloser(r), nil
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}
