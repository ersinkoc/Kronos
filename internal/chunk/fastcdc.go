package chunk

import (
	"fmt"
	"io"
)

const (
	// DefaultMinSize is Kronos' default minimum chunk size.
	DefaultMinSize = 512 * 1024
	// DefaultAverageSize is Kronos' default target average chunk size.
	DefaultAverageSize = 2 * 1024 * 1024
	// DefaultMaxSize is Kronos' default maximum chunk size.
	DefaultMaxSize = 8 * 1024 * 1024
)

// Chunk describes one content-defined byte range.
type Chunk struct {
	Offset int64
	Size   int
	Data   []byte
}

// FastCDC splits streams into deterministic content-defined chunks.
type FastCDC struct {
	minSize int
	avgSize int
	maxSize int
	mask    uint64
}

// NewFastCDC returns a FastCDC chunker with explicit size bounds.
func NewFastCDC(minSize int, avgSize int, maxSize int) (*FastCDC, error) {
	if minSize <= 0 {
		return nil, fmt.Errorf("min size must be greater than zero")
	}
	if avgSize < minSize {
		return nil, fmt.Errorf("average size must be >= min size")
	}
	if maxSize < avgSize {
		return nil, fmt.Errorf("max size must be >= average size")
	}
	return &FastCDC{
		minSize: minSize,
		avgSize: avgSize,
		maxSize: maxSize,
		mask:    maskForAverage(avgSize),
	}, nil
}

// NewDefaultFastCDC returns Kronos' default chunker.
func NewDefaultFastCDC() *FastCDC {
	return &FastCDC{
		minSize: DefaultMinSize,
		avgSize: DefaultAverageSize,
		maxSize: DefaultMaxSize,
		mask:    maskForAverage(DefaultAverageSize),
	}
}

// Split reads r and returns all chunks.
func (c *FastCDC) Split(r io.Reader) ([]Chunk, error) {
	var chunks []Chunk
	err := c.ForEach(r, func(chunk Chunk) error {
		chunks = append(chunks, chunk)
		return nil
	})
	return chunks, err
}

// ForEach streams r and calls fn once per chunk.
func (c *FastCDC) ForEach(r io.Reader, fn func(Chunk) error) error {
	if c == nil {
		return fmt.Errorf("nil FastCDC")
	}
	if r == nil {
		return fmt.Errorf("reader is required")
	}
	if fn == nil {
		return fmt.Errorf("chunk callback is required")
	}
	buf := make([]byte, 0, c.maxSize)
	scratch := make([]byte, 64*1024)
	var offset int64
	eof := false
	for {
		for !eof && len(buf) < c.maxSize {
			limit := len(scratch)
			if remaining := c.maxSize - len(buf); remaining < limit {
				limit = remaining
			}
			n, err := r.Read(scratch[:limit])
			if n > 0 {
				buf = append(buf, scratch[:n]...)
			}
			if err == io.EOF {
				eof = true
				break
			}
			if err != nil {
				return err
			}
			if n == 0 {
				break
			}
		}
		if len(buf) == 0 {
			return nil
		}
		size := c.boundary(buf)
		data := make([]byte, size)
		copy(data, buf[:size])
		if err := fn(Chunk{Offset: offset, Size: size, Data: data}); err != nil {
			return err
		}
		offset += int64(size)
		copy(buf, buf[size:])
		buf = buf[:len(buf)-size]
	}
}

func (c *FastCDC) boundary(data []byte) int {
	if len(data) <= c.minSize {
		return len(data)
	}
	limit := len(data)
	if limit > c.maxSize {
		limit = c.maxSize
	}
	var fp uint64
	for i := c.minSize; i < limit; i++ {
		fp = (fp << 1) + gearTable[data[i]]
		if i >= c.avgSize/2 && (fp&c.mask) == 0 {
			return i + 1
		}
	}
	return limit
}

func maskForAverage(avg int) uint64 {
	bits := 0
	for size := 1; size < avg; size <<= 1 {
		bits++
	}
	if bits <= 0 {
		return 0
	}
	return (uint64(1) << bits) - 1
}
