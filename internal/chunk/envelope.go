package chunk

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	kcrypto "github.com/kronos/kronos/internal/crypto"
)

const (
	envelopeMagic = "KCHK"

	// EnvelopeVersion is the current encrypted chunk envelope version.
	EnvelopeVersion byte = 1
)

const (
	algorithmAES256GCM byte = 2
	algorithmChaCha20  byte = 3
)

// EnvelopeHeader is the plaintext header for an encrypted chunk.
type EnvelopeHeader struct {
	Version   byte
	Algorithm byte
	KeyID     string
	Nonce     []byte
}

// SealEnvelope encrypts plaintext and serializes an encrypted chunk envelope.
func SealEnvelope(cipher kcrypto.Cipher, keyID string, plaintext []byte, additionalData []byte) ([]byte, error) {
	if cipher == nil {
		return nil, fmt.Errorf("cipher is required")
	}
	if keyID == "" {
		return nil, fmt.Errorf("key id is required")
	}
	algorithm, err := envelopeAlgorithm(cipher.Algorithm())
	if err != nil {
		return nil, err
	}
	nonce, err := cipher.NewNonce()
	if err != nil {
		return nil, err
	}
	header := EnvelopeHeader{
		Version:   EnvelopeVersion,
		Algorithm: algorithm,
		KeyID:     keyID,
		Nonce:     nonce,
	}
	headerBytes, err := marshalHeader(header)
	if err != nil {
		return nil, err
	}
	authData := makeAdditionalData(headerBytes, additionalData)
	ciphertext := cipher.Seal(nonce, plaintext, authData)
	out := make([]byte, 0, len(headerBytes)+len(ciphertext))
	out = append(out, headerBytes...)
	out = append(out, ciphertext...)
	return out, nil
}

// OpenEnvelope parses envelope, authenticates it, and returns plaintext.
func OpenEnvelope(cipher kcrypto.Cipher, envelope []byte, additionalData []byte) ([]byte, EnvelopeHeader, error) {
	if cipher == nil {
		return nil, EnvelopeHeader{}, fmt.Errorf("cipher is required")
	}
	header, headerLen, err := ParseEnvelopeHeader(envelope)
	if err != nil {
		return nil, EnvelopeHeader{}, err
	}
	algorithm, err := envelopeAlgorithm(cipher.Algorithm())
	if err != nil {
		return nil, EnvelopeHeader{}, err
	}
	if header.Algorithm != algorithm {
		return nil, EnvelopeHeader{}, fmt.Errorf("envelope algorithm %d does not match cipher %s", header.Algorithm, cipher.Algorithm())
	}
	authData := makeAdditionalData(envelope[:headerLen], additionalData)
	plaintext, err := cipher.Open(header.Nonce, envelope[headerLen:], authData)
	if err != nil {
		return nil, EnvelopeHeader{}, err
	}
	return plaintext, header, nil
}

// ParseEnvelopeHeader parses the plaintext envelope header.
func ParseEnvelopeHeader(envelope []byte) (EnvelopeHeader, int, error) {
	reader := bytes.NewReader(envelope)
	magic := make([]byte, len(envelopeMagic))
	if _, err := io.ReadFull(reader, magic); err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope magic: %w", err)
	}
	if string(magic) != envelopeMagic {
		return EnvelopeHeader{}, 0, fmt.Errorf("invalid envelope magic")
	}
	version, err := reader.ReadByte()
	if err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope version: %w", err)
	}
	if version != EnvelopeVersion {
		return EnvelopeHeader{}, 0, fmt.Errorf("unsupported envelope version %d", version)
	}
	algorithm, err := reader.ReadByte()
	if err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope algorithm: %w", err)
	}
	nonceLen, err := reader.ReadByte()
	if err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope nonce length: %w", err)
	}
	var keyLen uint16
	if err := binary.Read(reader, binary.BigEndian, &keyLen); err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope key id length: %w", err)
	}
	if nonceLen == 0 {
		return EnvelopeHeader{}, 0, fmt.Errorf("envelope nonce is empty")
	}
	if keyLen == 0 {
		return EnvelopeHeader{}, 0, fmt.Errorf("envelope key id is empty")
	}
	nonce := make([]byte, int(nonceLen))
	if _, err := io.ReadFull(reader, nonce); err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope nonce: %w", err)
	}
	key := make([]byte, int(keyLen))
	if _, err := io.ReadFull(reader, key); err != nil {
		return EnvelopeHeader{}, 0, fmt.Errorf("read envelope key id: %w", err)
	}
	return EnvelopeHeader{
		Version:   version,
		Algorithm: algorithm,
		KeyID:     string(key),
		Nonce:     nonce,
	}, len(envelopeMagic) + 1 + 1 + 1 + 2 + int(nonceLen) + int(keyLen), nil
}

func marshalHeader(header EnvelopeHeader) ([]byte, error) {
	if header.Version != EnvelopeVersion {
		return nil, fmt.Errorf("unsupported envelope version %d", header.Version)
	}
	if header.Algorithm != algorithmAES256GCM && header.Algorithm != algorithmChaCha20 {
		return nil, fmt.Errorf("unsupported envelope algorithm %d", header.Algorithm)
	}
	if len(header.Nonce) == 0 || len(header.Nonce) > 255 {
		return nil, fmt.Errorf("nonce length must be between 1 and 255 bytes")
	}
	if len(header.KeyID) == 0 || len(header.KeyID) > 65535 {
		return nil, fmt.Errorf("key id length must be between 1 and 65535 bytes")
	}

	var out bytes.Buffer
	out.WriteString(envelopeMagic)
	out.WriteByte(header.Version)
	out.WriteByte(header.Algorithm)
	out.WriteByte(byte(len(header.Nonce)))
	binary.Write(&out, binary.BigEndian, uint16(len(header.KeyID)))
	out.Write(header.Nonce)
	out.WriteString(header.KeyID)
	return out.Bytes(), nil
}

func envelopeAlgorithm(name string) (byte, error) {
	switch name {
	case kcrypto.AlgorithmAES256GCM:
		return algorithmAES256GCM, nil
	case kcrypto.AlgorithmChaCha20Poly1305:
		return algorithmChaCha20, nil
	default:
		return 0, fmt.Errorf("unsupported cipher algorithm %q", name)
	}
}

func makeAdditionalData(header []byte, additionalData []byte) []byte {
	out := make([]byte, 0, len(header)+len(additionalData))
	out = append(out, header...)
	out = append(out, additionalData...)
	return out
}
