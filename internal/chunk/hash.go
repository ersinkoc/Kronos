package chunk

import (
	"encoding/hex"
	"fmt"
	"io"

	"lukechampine.com/blake3"
)

const (
	// HashSize is the byte size of Kronos chunk IDs.
	HashSize = 32
)

// Hash is a BLAKE3-256 digest.
type Hash [HashSize]byte

// HashBytes returns the BLAKE3-256 digest of data.
func HashBytes(data []byte) Hash {
	return blake3.Sum256(data)
}

// HashReader streams r into a BLAKE3 hasher and returns the digest.
func HashReader(r io.Reader) (Hash, error) {
	if r == nil {
		return Hash{}, fmt.Errorf("reader is required")
	}
	hasher := blake3.New(HashSize, nil)
	if _, err := io.Copy(hasher, r); err != nil {
		return Hash{}, err
	}
	var out Hash
	copy(out[:], hasher.Sum(nil))
	return out, nil
}

// String returns h as lowercase hexadecimal.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// ParseHash parses a lowercase or uppercase hexadecimal BLAKE3-256 digest.
func ParseHash(value string) (Hash, error) {
	data, err := hex.DecodeString(value)
	if err != nil {
		return Hash{}, err
	}
	if len(data) != HashSize {
		return Hash{}, fmt.Errorf("hash must be %d bytes, got %d", HashSize, len(data))
	}
	var out Hash
	copy(out[:], data)
	return out, nil
}
