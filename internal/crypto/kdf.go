package crypto

import (
	"fmt"

	"golang.org/x/crypto/argon2"
)

const derivedKeySize = 32

// KDFParams configures Argon2id key derivation.
type KDFParams struct {
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	KeyLen  uint32 `json:"key_len"`
}

// DefaultKDFParams returns Kronos production Argon2id defaults.
func DefaultKDFParams() KDFParams {
	return KDFParams{
		Time:    3,
		Memory:  64 * 1024,
		Threads: 4,
		KeyLen:  derivedKeySize,
	}
}

// DeriveKey derives a 32-byte key with Kronos production defaults.
func DeriveKey(passphrase, salt []byte) ([]byte, error) {
	return DeriveKeyWithParams(passphrase, salt, DefaultKDFParams())
}

// DeriveKeyWithParams derives a key using Argon2id and explicit params.
func DeriveKeyWithParams(passphrase, salt []byte, params KDFParams) ([]byte, error) {
	if len(passphrase) == 0 {
		return nil, fmt.Errorf("passphrase is required")
	}
	if len(salt) < 16 {
		return nil, fmt.Errorf("salt must be at least 16 bytes, got %d", len(salt))
	}
	if params.Time == 0 {
		return nil, fmt.Errorf("argon2id time cost must be greater than zero")
	}
	if params.Memory == 0 {
		return nil, fmt.Errorf("argon2id memory cost must be greater than zero")
	}
	if params.Threads == 0 {
		return nil, fmt.Errorf("argon2id threads must be greater than zero")
	}
	if params.KeyLen == 0 {
		params.KeyLen = derivedKeySize
	}
	return argon2.IDKey(passphrase, salt, params.Time, params.Memory, params.Threads, params.KeyLen), nil
}
