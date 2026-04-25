package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/kronos/kronos/internal/repository"
	"github.com/kronos/kronos/internal/storage/local"
)

func runGC(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("gc", out)
	localRoot := fs.String("storage-local", "", "local storage repository root")
	publicKeyHex := fs.String("public-key", "", "hex-encoded Ed25519 public key")
	dryRun := fs.Bool("dry-run", false, "report orphan chunks without deleting them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *localRoot == "" {
		return fmt.Errorf("--storage-local is required")
	}
	if *publicKeyHex == "" {
		return fmt.Errorf("--public-key is required")
	}
	publicKey, err := decodePublicKey(*publicKeyHex)
	if err != nil {
		return err
	}
	backend, err := local.New("local", *localRoot)
	if err != nil {
		return err
	}
	report, err := repository.GarbageCollect(ctx, backend, publicKey, repository.GCOptions{DryRun: *dryRun})
	if err != nil {
		return err
	}
	if *dryRun {
		return writeCommandJSON(ctx, out, map[string]any{
			"ok":                true,
			"dry_run":           true,
			"manifests":         report.Manifests,
			"referenced_chunks": report.ReferencedChunks,
			"scanned_chunks":    report.ScannedChunks,
			"orphan_chunks":     len(report.OrphanChunks),
			"deleted_chunks":    0,
			"deleted_bytes":     0,
		})
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":                true,
		"dry_run":           false,
		"manifests":         report.Manifests,
		"referenced_chunks": report.ReferencedChunks,
		"scanned_chunks":    report.ScannedChunks,
		"orphan_chunks":     len(report.OrphanChunks),
		"deleted_chunks":    report.DeletedChunks,
		"deleted_bytes":     report.DeletedBytes,
	})
}

func decodePublicKey(value string) (ed25519.PublicKey, error) {
	keyBytes, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(keyBytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(keyBytes), nil
}
