package chunk

import (
	"bytes"
	"testing"
)

func TestFastCDCDeterministicBoundaries(t *testing.T) {
	t.Parallel()

	data := deterministicData(512 * 1024)
	chunker, err := NewFastCDC(2*1024, 8*1024, 32*1024)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}

	first, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Split(first) error = %v", err)
	}
	second, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Split(second) error = %v", err)
	}

	if len(first) != len(second) {
		t.Fatalf("chunk count first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i].Offset != second[i].Offset || first[i].Size != second[i].Size {
			t.Fatalf("chunk %d first=%#v second=%#v", i, first[i], second[i])
		}
	}
}

func TestFastCDCRoundTrip(t *testing.T) {
	t.Parallel()

	data := deterministicData(300 * 1024)
	chunker, err := NewFastCDC(1024, 4096, 16*1024)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}
	chunks, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	var got bytes.Buffer
	for _, chunk := range chunks {
		if chunk.Size != len(chunk.Data) {
			t.Fatalf("chunk size = %d len(data) = %d", chunk.Size, len(chunk.Data))
		}
		got.Write(chunk.Data)
	}
	if !bytes.Equal(got.Bytes(), data) {
		t.Fatal("reassembled chunks do not match input")
	}
}

func TestFastCDCForEachMatchesSplit(t *testing.T) {
	t.Parallel()

	data := deterministicData(512 * 1024)
	chunker, err := NewFastCDC(2*1024, 8*1024, 32*1024)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}
	want, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	var got []Chunk
	if err := chunker.ForEach(bytes.NewReader(data), func(chunk Chunk) error {
		got = append(got, chunk)
		return nil
	}); err != nil {
		t.Fatalf("ForEach() error = %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ForEach chunks = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Offset != want[i].Offset || got[i].Size != want[i].Size || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Fatalf("chunk %d mismatch", i)
		}
	}
}

func TestNewDefaultFastCDC(t *testing.T) {
	t.Parallel()

	chunker := NewDefaultFastCDC()
	if chunker.minSize != DefaultMinSize || chunker.avgSize != DefaultAverageSize || chunker.maxSize != DefaultMaxSize {
		t.Fatalf("default chunker = %#v", chunker)
	}
	chunks, err := chunker.Split(bytes.NewReader([]byte("small payload")))
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}
	if len(chunks) != 1 || string(chunks[0].Data) != "small payload" {
		t.Fatalf("chunks = %#v", chunks)
	}
}

func TestFastCDCRespectsBounds(t *testing.T) {
	t.Parallel()

	data := deterministicData(512 * 1024)
	minSize := 4 * 1024
	maxSize := 32 * 1024
	chunker, err := NewFastCDC(minSize, 8*1024, maxSize)
	if err != nil {
		t.Fatalf("NewFastCDC() error = %v", err)
	}
	chunks, err := chunker.Split(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Split() error = %v", err)
	}

	for i, chunk := range chunks {
		last := i == len(chunks)-1
		if !last && chunk.Size < minSize {
			t.Fatalf("chunk %d size = %d, below min %d", i, chunk.Size, minSize)
		}
		if chunk.Size > maxSize {
			t.Fatalf("chunk %d size = %d, above max %d", i, chunk.Size, maxSize)
		}
	}
}

func TestFastCDCRejectsInvalidBounds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		min  int
		avg  int
		max  int
	}{
		{name: "zero min", min: 0, avg: 1, max: 2},
		{name: "avg below min", min: 2, avg: 1, max: 3},
		{name: "max below avg", min: 1, avg: 3, max: 2},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewFastCDC(tc.min, tc.avg, tc.max); err == nil {
				t.Fatal("NewFastCDC() error = nil, want error")
			}
		})
	}
}

func deterministicData(size int) []byte {
	data := make([]byte, size)
	var x uint32 = 0x12345678
	for i := range data {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		data[i] = byte(x)
	}
	return data
}
