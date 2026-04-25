package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// ID is a UUIDv7 identifier encoded in canonical text form.
type ID string

// NewID returns a UUIDv7 using random entropy and the supplied clock.
func NewID(clock Clock) (ID, error) {
	if clock == nil {
		clock = RealClock{}
	}
	return newID(clock.Now(), rand.Reader)
}

func newID(now time.Time, entropy io.Reader) (ID, error) {
	var b [16]byte
	if _, err := io.ReadFull(entropy, b[:]); err != nil {
		return "", fmt.Errorf("generate id entropy: %w", err)
	}

	unixMillis := uint64(now.UnixMilli())
	b[0] = byte(unixMillis >> 40)
	b[1] = byte(unixMillis >> 32)
	b[2] = byte(unixMillis >> 24)
	b[3] = byte(unixMillis >> 16)
	b[4] = byte(unixMillis >> 8)
	b[5] = byte(unixMillis)

	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return ID(formatUUID(b)), nil
}

func formatUUID(b [16]byte) string {
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:])
}

// String returns the canonical text form of the ID.
func (id ID) String() string {
	return string(id)
}

// IsZero reports whether id is empty.
func (id ID) IsZero() bool {
	return strings.TrimSpace(string(id)) == ""
}
