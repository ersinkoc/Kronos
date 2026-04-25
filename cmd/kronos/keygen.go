package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
)

type keygenOutput struct {
	ManifestPublicKey  string `json:"manifest_public_key"`
	ManifestPrivateKey string `json:"manifest_private_key"`
	ChunkKey           string `json:"chunk_key"`
	ChunkAlgorithm     string `json:"chunk_algorithm"`
	KeyID              string `json:"key_id"`
}

func runKeygen(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("keygen", out)
	keyID := fs.String("key-id", "default", "key id recorded in manifests")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyID == "" {
		return fmt.Errorf("--key-id is required")
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate manifest signing key: %w", err)
	}
	var chunkKey [32]byte
	if _, err := io.ReadFull(rand.Reader, chunkKey[:]); err != nil {
		return fmt.Errorf("generate chunk key: %w", err)
	}
	payload := keygenOutput{
		ManifestPublicKey:  hex.EncodeToString(publicKey),
		ManifestPrivateKey: hex.EncodeToString(privateKey),
		ChunkKey:           hex.EncodeToString(chunkKey[:]),
		ChunkAlgorithm:     "aes-256-gcm",
		KeyID:              *keyID,
	}
	return writeCommandJSON(ctx, out, payload)
}
