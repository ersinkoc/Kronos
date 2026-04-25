package chunk

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"sync"
)

const (
	defaultBloomBitsPerEntry = 10
	defaultBloomHashes       = 7
)

// Index is an in-memory deduplication index for chunk hashes.
type Index struct {
	mu      sync.RWMutex
	entries map[Hash]struct{}
	bloom   *BloomFilter
}

// NewIndex returns an index sized for expectedEntries.
func NewIndex(expectedEntries int) (*Index, error) {
	if expectedEntries <= 0 {
		expectedEntries = 1
	}
	bloom, err := NewBloomFilter(expectedEntries*defaultBloomBitsPerEntry, defaultBloomHashes)
	if err != nil {
		return nil, err
	}
	return &Index{
		entries: make(map[Hash]struct{}, expectedEntries),
		bloom:   bloom,
	}, nil
}

// Add inserts hash into the index. It returns true if hash was newly inserted.
func (i *Index) Add(hash Hash) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	if _, ok := i.entries[hash]; ok {
		return false
	}
	i.entries[hash] = struct{}{}
	i.bloom.Add(hash[:])
	return true
}

// Contains reports whether hash is known to the index.
func (i *Index) Contains(hash Hash) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if !i.bloom.MaybeContains(hash[:]) {
		return false
	}
	_, ok := i.entries[hash]
	return ok
}

// MaybeContains reports whether hash may be known, using only the Bloom filter.
func (i *Index) MaybeContains(hash Hash) bool {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.bloom.MaybeContains(hash[:])
}

// Len returns the exact number of hashes in the index.
func (i *Index) Len() int {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return len(i.entries)
}

// Stats returns index counters.
func (i *Index) Stats() IndexStats {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return IndexStats{
		Entries:   len(i.entries),
		BloomBits: i.bloom.BitCount(),
		Hashes:    i.bloom.Hashes(),
	}
}

// IndexStats exposes index sizing details.
type IndexStats struct {
	Entries   int
	BloomBits int
	Hashes    int
}

// BloomFilter is a compact probabilistic membership filter.
type BloomFilter struct {
	bits   []uint64
	nbits  uint64
	hashes uint64
}

// NewBloomFilter returns a Bloom filter with bitCount bits and hashCount probes.
func NewBloomFilter(bitCount int, hashCount int) (*BloomFilter, error) {
	if bitCount <= 0 {
		return nil, fmt.Errorf("bloom bit count must be greater than zero")
	}
	if hashCount <= 0 {
		return nil, fmt.Errorf("bloom hash count must be greater than zero")
	}
	words := (bitCount + 63) / 64
	return &BloomFilter{
		bits:   make([]uint64, words),
		nbits:  uint64(bitCount),
		hashes: uint64(hashCount),
	}, nil
}

// Add inserts data into the filter.
func (b *BloomFilter) Add(data []byte) {
	h1, h2 := bloomHashes(data)
	for i := uint64(0); i < b.hashes; i++ {
		idx := (h1 + i*h2) % b.nbits
		b.bits[idx/64] |= uint64(1) << (idx % 64)
	}
}

// MaybeContains reports whether data may be in the filter.
func (b *BloomFilter) MaybeContains(data []byte) bool {
	h1, h2 := bloomHashes(data)
	for i := uint64(0); i < b.hashes; i++ {
		idx := (h1 + i*h2) % b.nbits
		if b.bits[idx/64]&(uint64(1)<<(idx%64)) == 0 {
			return false
		}
	}
	return true
}

// BitCount returns the number of bits in the filter.
func (b *BloomFilter) BitCount() int {
	return int(b.nbits)
}

// Hashes returns the number of hash probes.
func (b *BloomFilter) Hashes() int {
	return int(b.hashes)
}

func bloomHashes(data []byte) (uint64, uint64) {
	first := fnv.New64a()
	first.Write(data)
	h1 := first.Sum64()

	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(data)))
	second := fnv.New64()
	second.Write(length[:])
	second.Write(data)
	h2 := second.Sum64()
	if h2 == 0 {
		h2 = 0x9e3779b97f4a7c15
	}
	return h1, h2
}
