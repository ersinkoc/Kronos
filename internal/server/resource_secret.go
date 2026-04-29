package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	kcrypto "github.com/kronos/kronos/internal/crypto"
)

const encryptedOptionMarker = "kronos-state-secret:v1"

type encryptedOptionValue struct {
	Marker     string `json:"_kronos_secret"`
	Algorithm  string `json:"algorithm"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// StateSecretProtector encrypts sensitive resource option values before they
// are written to the state database.
type StateSecretProtector struct {
	cipher kcrypto.Cipher
}

// NewStateSecretProtector derives a state secret protector from the configured
// server master passphrase.
func NewStateSecretProtector(passphrase string) (*StateSecretProtector, error) {
	passphrase = strings.TrimSpace(passphrase)
	if passphrase == "" {
		return nil, fmt.Errorf("state secret passphrase is required")
	}
	salt := sha256.Sum256([]byte("kronos-state-secret-protector-v1"))
	key, err := kcrypto.DeriveKey([]byte(passphrase), salt[:])
	if err != nil {
		return nil, err
	}
	cipher, err := kcrypto.NewAES256GCM(key)
	if err != nil {
		return nil, err
	}
	return &StateSecretProtector{cipher: cipher}, nil
}

func (p *StateSecretProtector) protectOptions(options map[string]any) (map[string]any, error) {
	if len(options) == 0 {
		return options, nil
	}
	out := make(map[string]any, len(options))
	for key, value := range options {
		if !sensitiveOptionKey(key) {
			out[key] = value
			continue
		}
		if isEncryptedOptionValue(value) {
			out[key] = value
			continue
		}
		if p == nil {
			out[key] = value
			continue
		}
		protected, err := p.protectValue(key, value)
		if err != nil {
			return nil, err
		}
		out[key] = protected
	}
	return out, nil
}

func (p *StateSecretProtector) revealOptions(options map[string]any) (map[string]any, error) {
	if len(options) == 0 {
		return options, nil
	}
	out := make(map[string]any, len(options))
	for key, value := range options {
		if !isEncryptedOptionValue(value) {
			out[key] = value
			continue
		}
		if p == nil {
			return nil, fmt.Errorf("state secret protector is required to read encrypted option %q", key)
		}
		plaintext, err := p.revealValue(key, value)
		if err != nil {
			return nil, err
		}
		out[key] = plaintext
	}
	return out, nil
}

func (p *StateSecretProtector) protectValue(key string, value any) (encryptedOptionValue, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return encryptedOptionValue{}, fmt.Errorf("marshal secret option %q: %w", key, err)
	}
	nonce, err := p.cipher.NewNonce()
	if err != nil {
		return encryptedOptionValue{}, err
	}
	ciphertext := p.cipher.Seal(nonce, plaintext, []byte(key))
	return encryptedOptionValue{
		Marker:     encryptedOptionMarker,
		Algorithm:  p.cipher.Algorithm(),
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

func (p *StateSecretProtector) revealValue(key string, value any) (any, error) {
	encrypted, err := encryptedOptionFromAny(value)
	if err != nil {
		return nil, err
	}
	if encrypted.Algorithm != kcrypto.AlgorithmAES256GCM {
		return nil, fmt.Errorf("unsupported encrypted option algorithm %q", encrypted.Algorithm)
	}
	nonce, err := base64.RawStdEncoding.DecodeString(encrypted.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted option nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decode encrypted option ciphertext: %w", err)
	}
	plaintext, err := p.cipher.Open(nonce, ciphertext, []byte(key))
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(plaintext, &out); err != nil {
		return nil, fmt.Errorf("unmarshal secret option %q: %w", key, err)
	}
	return out, nil
}

func isEncryptedOptionValue(value any) bool {
	encrypted, err := encryptedOptionFromAny(value)
	return err == nil && encrypted.Marker == encryptedOptionMarker
}

func encryptedOptionFromAny(value any) (encryptedOptionValue, error) {
	switch typed := value.(type) {
	case encryptedOptionValue:
		return typed, nil
	case map[string]any:
		data, err := json.Marshal(typed)
		if err != nil {
			return encryptedOptionValue{}, err
		}
		var encrypted encryptedOptionValue
		if err := json.Unmarshal(data, &encrypted); err != nil {
			return encryptedOptionValue{}, err
		}
		if encrypted.Marker != encryptedOptionMarker {
			return encryptedOptionValue{}, fmt.Errorf("not encrypted option")
		}
		return encrypted, nil
	default:
		return encryptedOptionValue{}, fmt.Errorf("not encrypted option")
	}
}

func sensitiveOptionKey(key string) bool {
	key = strings.ToLower(key)
	for _, marker := range []string{"password", "secret", "token", "passphrase", "credential", "private_key", "access_key", "session_key", "encryption_key", "api_key", "apikey"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}
