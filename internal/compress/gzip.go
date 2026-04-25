package compress

import (
	"compress/gzip"
	"io"
)

const (
	// DefaultGzipLevel is the gzip compatibility fallback level.
	DefaultGzipLevel = gzip.DefaultCompression
)

// GzipCompressor wraps compress/gzip.
type GzipCompressor struct {
	Level int
}

// Algorithm returns gzip.
func (GzipCompressor) Algorithm() Algorithm {
	return AlgorithmGzip
}

// NewWriter returns a gzip writer.
func (c GzipCompressor) NewWriter(w io.Writer) (io.WriteCloser, error) {
	level := c.Level
	if level == 0 {
		level = DefaultGzipLevel
	}
	return gzip.NewWriterLevel(w, level)
}

// NewReader returns a gzip reader.
func (GzipCompressor) NewReader(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}
