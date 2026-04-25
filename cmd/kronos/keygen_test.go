package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunKeygenEmitsUsableKeyMaterial(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"--output", "pretty", "keygen", "--key-id", "prod-2026"}); err != nil {
		t.Fatalf("keygen error = %v", err)
	}
	var payload keygenOutput
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(keygen) error = %v output=%q", err, out.String())
	}
	publicKey, err := hex.DecodeString(payload.ManifestPublicKey)
	if err != nil {
		t.Fatalf("DecodeString(public key) error = %v", err)
	}
	privateKey, err := hex.DecodeString(payload.ManifestPrivateKey)
	if err != nil {
		t.Fatalf("DecodeString(private key) error = %v", err)
	}
	chunkKey, err := hex.DecodeString(payload.ChunkKey)
	if err != nil {
		t.Fatalf("DecodeString(chunk key) error = %v", err)
	}
	if len(publicKey) != 32 || len(privateKey) != 64 || len(chunkKey) != 32 {
		t.Fatalf("key lengths public=%d private=%d chunk=%d", len(publicKey), len(privateKey), len(chunkKey))
	}
	if payload.ChunkAlgorithm != "aes-256-gcm" || payload.KeyID != "prod-2026" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRunKeygenYAMLOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"--output", "yaml", "keygen", "--key-id", "prod-2026"}); err != nil {
		t.Fatalf("keygen yaml error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "manifest_public_key:") || !strings.Contains(text, "manifest_private_key:") || !strings.Contains(text, "key_id: prod-2026") {
		t.Fatalf("keygen yaml output = %q", text)
	}
}

func TestRunKeygenTableOutput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"--output", "table", "keygen", "--key-id", "prod-2026"}); err != nil {
		t.Fatalf("keygen table error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"KEY", "VALUE", "manifest_public_key", "manifest_private_key", "key_id", "prod-2026"} {
		if !strings.Contains(text, want) {
			t.Fatalf("keygen table output missing %q: %q", want, text)
		}
	}
}

func TestRunKeygenRequiresKeyID(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"keygen", "--key-id", ""}); err == nil {
		t.Fatal("keygen without key id error = nil, want error")
	}
}
