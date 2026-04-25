package chunk

import (
	"bytes"
	"testing"

	kcrypto "github.com/kronos/kronos/internal/crypto"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()

	ciphers := []struct {
		name string
		new  func([]byte) (kcrypto.Cipher, error)
	}{
		{name: kcrypto.AlgorithmAES256GCM, new: kcrypto.NewAES256GCM},
		{name: kcrypto.AlgorithmChaCha20Poly1305, new: kcrypto.NewChaCha20Poly1305},
	}

	for _, tc := range ciphers {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cipher, err := tc.new(bytes.Repeat([]byte{0x42}, 32))
			if err != nil {
				t.Fatalf("new cipher error = %v", err)
			}
			plaintext := []byte("Time devours. Kronos preserves.")
			aad := []byte("backup:123")
			envelope, err := SealEnvelope(cipher, "root-key", plaintext, aad)
			if err != nil {
				t.Fatalf("SealEnvelope() error = %v", err)
			}
			got, header, err := OpenEnvelope(cipher, envelope, aad)
			if err != nil {
				t.Fatalf("OpenEnvelope() error = %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Fatalf("OpenEnvelope() = %q, want %q", got, plaintext)
			}
			if header.KeyID != "root-key" || header.Version != EnvelopeVersion {
				t.Fatalf("header = %#v", header)
			}
		})
	}
}

func TestEnvelopeRejectsBadVersion(t *testing.T) {
	t.Parallel()

	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	envelope, err := SealEnvelope(cipher, "root-key", []byte("payload"), nil)
	if err != nil {
		t.Fatalf("SealEnvelope() error = %v", err)
	}
	envelope[len(envelopeMagic)] = 99
	if _, _, err := OpenEnvelope(cipher, envelope, nil); err == nil {
		t.Fatal("OpenEnvelope(bad version) error = nil, want error")
	}
}

func TestEnvelopeRejectsTampering(t *testing.T) {
	t.Parallel()

	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	envelope, err := SealEnvelope(cipher, "root-key", []byte("payload"), []byte("aad"))
	if err != nil {
		t.Fatalf("SealEnvelope() error = %v", err)
	}
	envelope[len(envelope)-1] ^= 0xff
	if _, _, err := OpenEnvelope(cipher, envelope, []byte("aad")); err == nil {
		t.Fatal("OpenEnvelope(tampered) error = nil, want error")
	}
}

func TestEnvelopeRejectsWrongAAD(t *testing.T) {
	t.Parallel()

	cipher, err := kcrypto.NewAES256GCM(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	envelope, err := SealEnvelope(cipher, "root-key", []byte("payload"), []byte("aad"))
	if err != nil {
		t.Fatalf("SealEnvelope() error = %v", err)
	}
	if _, _, err := OpenEnvelope(cipher, envelope, []byte("wrong")); err == nil {
		t.Fatal("OpenEnvelope(wrong aad) error = nil, want error")
	}
}

func TestParseEnvelopeHeader(t *testing.T) {
	t.Parallel()

	header := EnvelopeHeader{
		Version:   EnvelopeVersion,
		Algorithm: algorithmAES256GCM,
		KeyID:     "key-1",
		Nonce:     []byte("123456789012"),
	}
	data, err := marshalHeader(header)
	if err != nil {
		t.Fatalf("marshalHeader() error = %v", err)
	}
	got, n, err := ParseEnvelopeHeader(append(data, []byte("ciphertext")...))
	if err != nil {
		t.Fatalf("ParseEnvelopeHeader() error = %v", err)
	}
	if n != len(data) || got.KeyID != header.KeyID || !bytes.Equal(got.Nonce, header.Nonce) {
		t.Fatalf("ParseEnvelopeHeader() header=%#v n=%d", got, n)
	}
}
