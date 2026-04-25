package chunk

import (
	"fmt"
	"sync"
	"testing"
)

func TestIndexAddContains(t *testing.T) {
	t.Parallel()

	index, err := NewIndex(100)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	hash := HashBytes([]byte("chunk-1"))

	if !index.Add(hash) {
		t.Fatal("Add(first) = false, want true")
	}
	if index.Add(hash) {
		t.Fatal("Add(duplicate) = true, want false")
	}
	if !index.Contains(hash) {
		t.Fatal("Contains(inserted) = false, want true")
	}
	if !index.MaybeContains(hash) {
		t.Fatal("MaybeContains(inserted) = false, want true")
	}
	if index.Contains(HashBytes([]byte("missing"))) {
		t.Fatal("Contains(missing) = true, want false")
	}
	if index.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", index.Len())
	}
}

func TestBloomFilterNoFalseNegatives(t *testing.T) {
	t.Parallel()

	filter, err := NewBloomFilter(10_000, 7)
	if err != nil {
		t.Fatalf("NewBloomFilter() error = %v", err)
	}
	var hashes []Hash
	for i := 0; i < 500; i++ {
		hash := HashBytes([]byte(fmt.Sprintf("chunk-%d", i)))
		hashes = append(hashes, hash)
		filter.Add(hash[:])
	}
	for i, hash := range hashes {
		if !filter.MaybeContains(hash[:]) {
			t.Fatalf("MaybeContains(inserted %d) = false", i)
		}
	}
}

func TestIndexConcurrentAccess(t *testing.T) {
	t.Parallel()

	index, err := NewIndex(1000)
	if err != nil {
		t.Fatalf("NewIndex() error = %v", err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				hash := HashBytes([]byte(fmt.Sprintf("worker-%d-%d", worker, i)))
				index.Add(hash)
				if !index.Contains(hash) {
					t.Errorf("Contains(%s) = false", hash)
				}
			}
		}()
	}
	wg.Wait()

	if index.Len() != 8*250 {
		t.Fatalf("Len() = %d, want %d", index.Len(), 8*250)
	}
	stats := index.Stats()
	if stats.Entries != index.Len() || stats.BloomBits == 0 || stats.Hashes == 0 {
		t.Fatalf("Stats() = %#v", stats)
	}
}

func TestBloomFilterRejectsInvalidSizing(t *testing.T) {
	t.Parallel()

	if _, err := NewBloomFilter(0, 1); err == nil {
		t.Fatal("NewBloomFilter(0 bits) error = nil, want error")
	}
	if _, err := NewBloomFilter(1, 0); err == nil {
		t.Fatal("NewBloomFilter(0 hashes) error = nil, want error")
	}
}

func BenchmarkIndexContains(b *testing.B) {
	index, err := NewIndex(1_000_000)
	if err != nil {
		b.Fatalf("NewIndex() error = %v", err)
	}
	hashes := make([]Hash, 0, 1_000_000)
	for i := 0; i < 1_000_000; i++ {
		hash := HashBytes([]byte(fmt.Sprintf("chunk-%d", i)))
		hashes = append(hashes, hash)
		index.Add(hash)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !index.Contains(hashes[i%len(hashes)]) {
			b.Fatal("missing inserted hash")
		}
	}
}
