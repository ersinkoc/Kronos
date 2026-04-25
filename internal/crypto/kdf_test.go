package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestDeriveKeyWithParamsKnownAnswer(t *testing.T) {
	t.Parallel()

	params := KDFParams{Time: 2, Memory: 64 * 1024, Threads: 1, KeyLen: 32}
	got, err := DeriveKeyWithParams([]byte("password"), []byte("somesalt-16bytes"), params)
	if err != nil {
		t.Fatalf("DeriveKeyWithParams() error = %v", err)
	}
	want := mustDecodeHex(t, "a094bd231a33de394ece80b157789a2a5e3720ce7c55cc406ff329b7eb40b0c6")
	if !bytes.Equal(got, want) {
		t.Fatalf("DeriveKeyWithParams() = %x, want %x", got, want)
	}
}

func TestDeriveKeyDefaults(t *testing.T) {
	t.Parallel()

	params := DefaultKDFParams()
	if params.Time != 3 || params.Memory != 64*1024 || params.Threads != 4 || params.KeyLen != 32 {
		t.Fatalf("DefaultKDFParams() = %#v", params)
	}
}

func TestDeriveKeyValidation(t *testing.T) {
	t.Parallel()

	_, err := DeriveKey(nil, []byte("somesalt-16bytes"))
	if err == nil {
		t.Fatal("DeriveKey(empty passphrase) error = nil, want error")
	}

	_, err = DeriveKey([]byte("password"), []byte("short"))
	if err == nil {
		t.Fatal("DeriveKey(short salt) error = nil, want error")
	}
}

func TestDerivedKeyIsAESKeySized(t *testing.T) {
	t.Parallel()

	params := KDFParams{Time: 1, Memory: 1024, Threads: 1, KeyLen: 32}
	key, err := DeriveKeyWithParams([]byte("password"), []byte("somesalt-16bytes"), params)
	if err != nil {
		t.Fatalf("DeriveKeyWithParams() error = %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("len(key) = %d, want 32", len(key))
	}
	if _, err := hex.DecodeString(hex.EncodeToString(key)); err != nil {
		t.Fatalf("derived key should be hex encodable: %v", err)
	}
}
