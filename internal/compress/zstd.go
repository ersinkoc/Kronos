package compress

import (
	"io"

	"github.com/klauspost/compress/zstd"
)

const (
	// DefaultZstdLevel is Kronos' default zstd level.
	DefaultZstdLevel = 3
)

// ZstdCompressor wraps klauspost/compress zstd.
type ZstdCompressor struct {
	Level int
}

// Algorithm returns zstd.
func (ZstdCompressor) Algorithm() Algorithm {
	return AlgorithmZstd
}

// NewWriter returns a zstd encoder.
func (c ZstdCompressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	level := c.Level
	if level == 0 {
		level = DefaultZstdLevel
	}
	encoder, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)))
	if err != nil {
		return nil, err
	}
	return encoder, nil
}

// NewReader returns a zstd decoder.
func (ZstdCompressor) NewReader(r io.Reader) (io.ReadCloser, error) {
	decoder, err := zstd.NewReader(r)
	if err != nil {
		return nil, err
	}
	return decoder.IOReadCloser(), nil
}
