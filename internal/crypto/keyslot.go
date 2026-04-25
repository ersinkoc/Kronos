package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

const (
	KeySlotFileVersion = 1
	keySlotSaltSize    = 32
	keySlotRootKeySize = 32
)

// KeySlotFile stores passphrase-protected unlock slots for one repository root key.
type KeySlotFile struct {
	Version   int       `json:"version"`
	Algorithm string    `json:"algorithm"`
	Slots     []KeySlot `json:"slots"`
}

// KeySlot stores one encrypted copy of the repository root key.
type KeySlot struct {
	ID         string    `json:"id"`
	KDF        KDFParams `json:"kdf"`
	Salt       string    `json:"salt"`
	Nonce      string    `json:"nonce"`
	Ciphertext string    `json:"ciphertext"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewKeySlotFile creates a key slot file with one passphrase-protected slot.
func NewKeySlotFile(rootKey []byte, id string, passphrase []byte, now time.Time) (KeySlotFile, error) {
	file := KeySlotFile{Version: KeySlotFileVersion, Algorithm: AlgorithmAES256GCM}
	if err := file.AddSlot(rootKey, id, passphrase, now); err != nil {
		return KeySlotFile{}, err
	}
	return file, nil
}

// ParseKeySlotFile decodes and validates a key slot file.
func ParseKeySlotFile(data []byte) (KeySlotFile, error) {
	var file KeySlotFile
	if err := json.Unmarshal(data, &file); err != nil {
		return KeySlotFile{}, err
	}
	if file.Version != KeySlotFileVersion {
		return KeySlotFile{}, fmt.Errorf("unsupported key slot file version %d", file.Version)
	}
	if file.Algorithm != AlgorithmAES256GCM {
		return KeySlotFile{}, fmt.Errorf("unsupported key slot algorithm %q", file.Algorithm)
	}
	seen := map[string]struct{}{}
	for _, slot := range file.Slots {
		if slot.ID == "" {
			return KeySlotFile{}, fmt.Errorf("key slot id is required")
		}
		if _, ok := seen[slot.ID]; ok {
			return KeySlotFile{}, fmt.Errorf("duplicate key slot %q", slot.ID)
		}
		seen[slot.ID] = struct{}{}
	}
	return file, nil
}

// Marshal returns a stable JSON representation of the key slot file.
func (f KeySlotFile) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// AddSlot encrypts rootKey into a new slot.
func (f *KeySlotFile) AddSlot(rootKey []byte, id string, passphrase []byte, now time.Time) error {
	if f == nil {
		return fmt.Errorf("key slot file is required")
	}
	if f.Version == 0 {
		f.Version = KeySlotFileVersion
	}
	if f.Algorithm == "" {
		f.Algorithm = AlgorithmAES256GCM
	}
	if f.Version != KeySlotFileVersion {
		return fmt.Errorf("unsupported key slot file version %d", f.Version)
	}
	if f.Algorithm != AlgorithmAES256GCM {
		return fmt.Errorf("unsupported key slot algorithm %q", f.Algorithm)
	}
	if len(rootKey) != keySlotRootKeySize {
		return fmt.Errorf("root key must be %d bytes, got %d", keySlotRootKeySize, len(rootKey))
	}
	if id == "" {
		return fmt.Errorf("--id is required")
	}
	for _, slot := range f.Slots {
		if slot.ID == id {
			return fmt.Errorf("key slot %q already exists", id)
		}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	params := DefaultKDFParams()
	salt, err := randomBytes(keySlotSaltSize)
	if err != nil {
		return err
	}
	derived, err := DeriveKeyWithParams(passphrase, salt, params)
	if err != nil {
		return err
	}
	cipher, err := NewAES256GCM(derived)
	if err != nil {
		return err
	}
	nonce, err := cipher.NewNonce()
	if err != nil {
		return err
	}
	slot := KeySlot{
		ID:         id,
		KDF:        params,
		Salt:       hex.EncodeToString(salt),
		Nonce:      hex.EncodeToString(nonce),
		Ciphertext: hex.EncodeToString(cipher.Seal(nonce, rootKey, keySlotAAD(id))),
		CreatedAt:  now.UTC(),
	}
	f.Slots = append(f.Slots, slot)
	return nil
}

// Unlock decrypts the root key from a slot.
func (f KeySlotFile) Unlock(id string, passphrase []byte) ([]byte, error) {
	for _, slot := range f.Slots {
		if slot.ID != id {
			continue
		}
		salt, err := hex.DecodeString(slot.Salt)
		if err != nil {
			return nil, fmt.Errorf("decode slot salt: %w", err)
		}
		nonce, err := hex.DecodeString(slot.Nonce)
		if err != nil {
			return nil, fmt.Errorf("decode slot nonce: %w", err)
		}
		ciphertext, err := hex.DecodeString(slot.Ciphertext)
		if err != nil {
			return nil, fmt.Errorf("decode slot ciphertext: %w", err)
		}
		derived, err := DeriveKeyWithParams(passphrase, salt, slot.KDF)
		if err != nil {
			return nil, err
		}
		cipher, err := NewAES256GCM(derived)
		if err != nil {
			return nil, err
		}
		rootKey, err := cipher.Open(nonce, ciphertext, keySlotAAD(slot.ID))
		if err != nil {
			return nil, err
		}
		if len(rootKey) != keySlotRootKeySize {
			return nil, fmt.Errorf("slot %q unlocked invalid root key length %d", slot.ID, len(rootKey))
		}
		return rootKey, nil
	}
	return nil, fmt.Errorf("key slot %q not found", id)
}

// RemoveSlot removes a slot by id.
func (f *KeySlotFile) RemoveSlot(id string, allowEmpty bool) error {
	if id == "" {
		return fmt.Errorf("--id is required")
	}
	for i, slot := range f.Slots {
		if slot.ID != id {
			continue
		}
		if len(f.Slots) == 1 && !allowEmpty {
			return fmt.Errorf("refusing to remove the last key slot without --yes")
		}
		f.Slots = append(f.Slots[:i], f.Slots[i+1:]...)
		return nil
	}
	return fmt.Errorf("key slot %q not found", id)
}

// RootKeyFingerprint returns a short stable fingerprint for logs and CLI output.
func RootKeyFingerprint(rootKey []byte) string {
	sum := sha256.Sum256(rootKey)
	return hex.EncodeToString(sum[:8])
}

func keySlotAAD(id string) []byte {
	return []byte("kronos-key-slot-v1:" + id)
}

func randomBytes(size int) ([]byte, error) {
	out := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, out); err != nil {
		return nil, fmt.Errorf("generate random bytes: %w", err)
	}
	return out, nil
}
