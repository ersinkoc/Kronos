package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// AlgorithmAES256GCM identifies AES-256-GCM chunk encryption.
	AlgorithmAES256GCM = "aes-256-gcm"
	// AlgorithmChaCha20Poly1305 identifies ChaCha20-Poly1305 chunk encryption.
	AlgorithmChaCha20Poly1305 = "chacha20-poly1305"
)

// Cipher wraps an AEAD algorithm behind a stable Kronos interface.
type Cipher interface {
	Algorithm() string
	NonceSize() int
	Overhead() int
	Seal(nonce, plaintext, additionalData []byte) []byte
	Open(nonce, ciphertext, additionalData []byte) ([]byte, error)
	NewNonce() ([]byte, error)
}

type aeadCipher struct {
	algorithm string
	aead      cipher.AEAD
}

// NewAES256GCM returns an AES-256-GCM cipher.
func NewAES256GCM(key []byte) (Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("aes-256-gcm key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create aes-gcm: %w", err)
	}
	return aeadCipher{algorithm: AlgorithmAES256GCM, aead: aead}, nil
}

// NewChaCha20Poly1305 returns a ChaCha20-Poly1305 cipher.
func NewChaCha20Poly1305(key []byte) (Cipher, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("chacha20-poly1305 key must be 32 bytes, got %d", len(key))
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("create chacha20-poly1305: %w", err)
	}
	return aeadCipher{algorithm: AlgorithmChaCha20Poly1305, aead: aead}, nil
}

func (c aeadCipher) Algorithm() string {
	return c.algorithm
}

func (c aeadCipher) NonceSize() int {
	return c.aead.NonceSize()
}

func (c aeadCipher) Overhead() int {
	return c.aead.Overhead()
}

func (c aeadCipher) Seal(nonce, plaintext, additionalData []byte) []byte {
	return c.aead.Seal(nil, nonce, plaintext, additionalData)
}

func (c aeadCipher) Open(nonce, ciphertext, additionalData []byte) ([]byte, error) {
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s payload: %w", c.algorithm, err)
	}
	return plaintext, nil
}

func (c aeadCipher) NewNonce() ([]byte, error) {
	nonce := make([]byte, c.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate %s nonce: %w", c.algorithm, err)
	}
	return nonce, nil
}
