package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestAES256GCMKnownAnswer(t *testing.T) {
	t.Parallel()

	cipher, err := NewAES256GCM(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewAES256GCM() error = %v", err)
	}
	got := cipher.Seal(make([]byte, 12), nil, nil)
	want := mustDecodeHex(t, "530f8afbc74536b9a963b4f1c4cb738b")
	if !bytes.Equal(got, want) {
		t.Fatalf("Seal() = %x, want %x", got, want)
	}
}

func TestChaCha20Poly1305KnownAnswer(t *testing.T) {
	t.Parallel()

	key := mustDecodeHex(t, "808182838485868788898a8b8c8d8e8f909192939495969798999a9b9c9d9e9f")
	nonce := mustDecodeHex(t, "070000004041424344454647")
	aad := mustDecodeHex(t, "50515253c0c1c2c3c4c5c6c7")
	plaintext := []byte("Ladies and Gentlemen of the class of '99: If I could offer you only one tip for the future, sunscreen would be it.")
	want := mustDecodeHex(t,
		"d31a8d34648e60db7b86afbc53ef7ec2"+
			"a4aded51296e08fea9e2b5a736ee62d6"+
			"3dbea45e8ca9671282fafb69da92728b"+
			"1a71de0a9e060b2905d6a5b67ecd3b36"+
			"92ddbd7f2d778b8c9803aee328091b58"+
			"fab324e4fad675945585808b4831d7bc"+
			"3ff4def08e4b7a9de576d26586cec64b"+
			"61161ae10b594f09e26a7e902ecbd0600691")

	cipher, err := NewChaCha20Poly1305(key)
	if err != nil {
		t.Fatalf("NewChaCha20Poly1305() error = %v", err)
	}
	got := cipher.Seal(nonce, plaintext, aad)
	if !bytes.Equal(got, want) {
		t.Fatalf("Seal() = %x, want %x", got, want)
	}
}

func TestCipherRoundTrip(t *testing.T) {
	t.Parallel()

	constructors := []struct {
		name string
		fn   func([]byte) (Cipher, error)
	}{
		{name: AlgorithmAES256GCM, fn: NewAES256GCM},
		{name: AlgorithmChaCha20Poly1305, fn: NewChaCha20Poly1305},
	}

	for _, tc := range constructors {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cipher, err := tc.fn(bytes.Repeat([]byte{0x42}, 32))
			if err != nil {
				t.Fatalf("constructor error = %v", err)
			}
			if got := cipher.Algorithm(); got != tc.name {
				t.Fatalf("Algorithm() = %s, want %s", got, tc.name)
			}
			if cipher.Overhead() <= 0 {
				t.Fatalf("Overhead() = %d, want positive", cipher.Overhead())
			}
			nonce, err := cipher.NewNonce()
			if err != nil {
				t.Fatalf("NewNonce() error = %v", err)
			}
			plaintext := []byte("Time devours. Kronos preserves.")
			aad := []byte("manifest:1")

			ciphertext := cipher.Seal(nonce, plaintext, aad)
			got, err := cipher.Open(nonce, ciphertext, aad)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Fatalf("Open() = %q, want %q", got, plaintext)
			}
		})
	}
}

func TestCipherRejectsBadKeySizes(t *testing.T) {
	t.Parallel()

	if _, err := NewAES256GCM(make([]byte, 31)); err == nil {
		t.Fatal("NewAES256GCM(short key) error = nil, want error")
	}
	if _, err := NewChaCha20Poly1305(make([]byte, 31)); err == nil {
		t.Fatal("NewChaCha20Poly1305(short key) error = nil, want error")
	}
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()

	out, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("DecodeString(%q) error = %v", value, err)
	}
	return out
}
