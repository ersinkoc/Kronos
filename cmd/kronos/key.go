package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	kcrypto "github.com/kronos/kronos/internal/crypto"
)

func runKey(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("key subcommand is required")
	}
	switch args[0] {
	case "add-slot":
		return runKeyAddSlot(ctx, out, args[1:])
	case "escrow":
		return runKeyEscrow(ctx, out, args[1:])
	case "list":
		return runKeyList(ctx, out, args[1:])
	case "remove-slot":
		return runKeyRemoveSlot(ctx, out, args[1:])
	case "rotate":
		return runKeyRotate(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown key subcommand %q", args[0])
	}
}

func runKeyAddSlot(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("key add-slot", out)
	filePath := fs.String("file", "", "key slot file path")
	id := fs.String("id", "", "new key slot id")
	rootKeyHex := fs.String("root-key", "", "hex-encoded 32-byte root key for a new slot file")
	generateRoot := fs.Bool("generate-root-key", false, "generate a new root key when creating a key slot file")
	passphrase := fs.String("passphrase", "", "new slot passphrase")
	passphraseEnv := fs.String("passphrase-env", "", "environment variable containing new slot passphrase")
	passphraseFile := fs.String("passphrase-file", "", "file containing new slot passphrase")
	unlockSlot := fs.String("unlock-slot", "", "existing slot id used to unlock the root key")
	unlockPassphrase := fs.String("unlock-passphrase", "", "existing slot passphrase")
	unlockPassphraseEnv := fs.String("unlock-passphrase-env", "", "environment variable containing existing slot passphrase")
	unlockPassphraseFile := fs.String("unlock-passphrase-file", "", "file containing existing slot passphrase")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return fmt.Errorf("--file is required")
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	newPassphrase, err := passphraseValue(*passphrase, *passphraseEnv, *passphraseFile, "new slot passphrase")
	if err != nil {
		return err
	}
	file, exists, err := loadKeySlotFile(*filePath)
	if err != nil {
		return err
	}
	rootKey, err := rootKeyForAddSlot(file, exists, *rootKeyHex, *generateRoot, *unlockSlot, *unlockPassphrase, *unlockPassphraseEnv, *unlockPassphraseFile)
	if err != nil {
		return err
	}
	if err := file.AddSlot(rootKey, *id, newPassphrase, time.Now().UTC()); err != nil {
		return err
	}
	if err := saveKeySlotFile(*filePath, file); err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":                   true,
		"file":                 *filePath,
		"slot":                 *id,
		"slots":                len(file.Slots),
		"root_key_fingerprint": kcrypto.RootKeyFingerprint(rootKey),
	})
}

func runKeyList(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("key list", out)
	filePath := fs.String("file", "", "key slot file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return fmt.Errorf("--file is required")
	}
	file, _, err := loadKeySlotFile(*filePath)
	if err != nil {
		return err
	}
	slots := make([]map[string]any, 0, len(file.Slots))
	for _, slot := range file.Slots {
		slots = append(slots, map[string]any{
			"id":         slot.ID,
			"created_at": slot.CreatedAt,
			"kdf": map[string]any{
				"time":    slot.KDF.Time,
				"memory":  slot.KDF.Memory,
				"threads": slot.KDF.Threads,
				"key_len": slot.KDF.KeyLen,
			},
		})
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"file":      *filePath,
		"version":   file.Version,
		"algorithm": file.Algorithm,
		"slots":     slots,
	})
}

func runKeyRemoveSlot(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("key remove-slot", out)
	filePath := fs.String("file", "", "key slot file path")
	id := fs.String("id", "", "key slot id")
	yes := fs.Bool("yes", false, "allow removing the last slot")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return fmt.Errorf("--file is required")
	}
	file, _, err := loadKeySlotFile(*filePath)
	if err != nil {
		return err
	}
	if err := file.RemoveSlot(*id, *yes); err != nil {
		return err
	}
	if err := saveKeySlotFile(*filePath, file); err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":    true,
		"file":  *filePath,
		"slot":  *id,
		"slots": len(file.Slots),
	})
}

func runKeyRotate(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("key rotate", out)
	filePath := fs.String("file", "", "source key slot file path")
	outPath := fs.String("out", "", "rotated key slot file path")
	id := fs.String("id", "", "slot id for the new root key")
	passphrase := fs.String("passphrase", "", "new slot passphrase")
	passphraseEnv := fs.String("passphrase-env", "", "environment variable containing new slot passphrase")
	passphraseFile := fs.String("passphrase-file", "", "file containing new slot passphrase")
	unlockSlot := fs.String("unlock-slot", "", "existing slot id used to unlock the old root key")
	unlockPassphrase := fs.String("unlock-passphrase", "", "existing slot passphrase")
	unlockPassphraseEnv := fs.String("unlock-passphrase-env", "", "environment variable containing existing slot passphrase")
	unlockPassphraseFile := fs.String("unlock-passphrase-file", "", "file containing existing slot passphrase")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return fmt.Errorf("--file is required")
	}
	if *outPath == "" {
		return fmt.Errorf("--out is required")
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	file, _, err := loadKeySlotFile(*filePath)
	if err != nil {
		return err
	}
	oldPassphrase, err := passphraseValue(*unlockPassphrase, *unlockPassphraseEnv, *unlockPassphraseFile, "existing slot passphrase")
	if err != nil {
		return err
	}
	oldRoot, err := file.Unlock(*unlockSlot, oldPassphrase)
	if err != nil {
		return err
	}
	newPassphrase, err := passphraseValue(*passphrase, *passphraseEnv, *passphraseFile, "new slot passphrase")
	if err != nil {
		return err
	}
	newRoot, err := randomRootKey()
	if err != nil {
		return err
	}
	rotated, err := kcrypto.NewKeySlotFile(newRoot, *id, newPassphrase, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := saveKeySlotFile(*outPath, rotated); err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":                       true,
		"file":                     *filePath,
		"out":                      *outPath,
		"slot":                     *id,
		"old_root_key_fingerprint": kcrypto.RootKeyFingerprint(oldRoot),
		"new_root_key_fingerprint": kcrypto.RootKeyFingerprint(newRoot),
	})
}

func runKeyEscrow(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("key escrow subcommand is required")
	}
	switch args[0] {
	case "export":
		return runKeyEscrowExport(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown key escrow subcommand %q", args[0])
	}
}

func runKeyEscrowExport(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("key escrow export", out)
	filePath := fs.String("file", "", "key slot file path")
	outPath := fs.String("out", "", "escrow export path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *filePath == "" {
		return fmt.Errorf("--file is required")
	}
	if *outPath == "" {
		return fmt.Errorf("--out is required")
	}
	file, _, err := loadKeySlotFile(*filePath)
	if err != nil {
		return err
	}
	if err := saveKeySlotFile(*outPath, file); err != nil {
		return err
	}
	return writeCommandJSON(ctx, out, map[string]any{
		"ok":    true,
		"file":  *filePath,
		"out":   *outPath,
		"slots": len(file.Slots),
	})
}

func rootKeyForAddSlot(file kcrypto.KeySlotFile, exists bool, rootKeyHex string, generateRoot bool, unlockSlot string, unlockPassphrase string, unlockPassphraseEnv string, unlockPassphraseFile string) ([]byte, error) {
	if rootKeyHex != "" && generateRoot {
		return nil, fmt.Errorf("use either --root-key or --generate-root-key, not both")
	}
	if rootKeyHex != "" {
		rootKey, err := hex.DecodeString(rootKeyHex)
		if err != nil {
			return nil, fmt.Errorf("decode --root-key: %w", err)
		}
		if len(rootKey) != 32 {
			return nil, fmt.Errorf("--root-key must decode to 32 bytes")
		}
		return rootKey, nil
	}
	if generateRoot {
		return randomRootKey()
	}
	if !exists {
		return nil, fmt.Errorf("new key slot files require --root-key or --generate-root-key")
	}
	passphrase, err := passphraseValue(unlockPassphrase, unlockPassphraseEnv, unlockPassphraseFile, "existing slot passphrase")
	if err != nil {
		return nil, err
	}
	return file.Unlock(unlockSlot, passphrase)
}

func loadKeySlotFile(path string) (kcrypto.KeySlotFile, bool, error) {
	data, err := readFileBounded(path, 16*1024*1024)
	if err != nil {
		if os.IsNotExist(err) {
			return kcrypto.KeySlotFile{Version: kcrypto.KeySlotFileVersion, Algorithm: kcrypto.AlgorithmAES256GCM}, false, nil
		}
		return kcrypto.KeySlotFile{}, false, err
	}
	file, err := kcrypto.ParseKeySlotFile(data)
	if err != nil {
		return kcrypto.KeySlotFile{}, false, err
	}
	return file, true, nil
}

func saveKeySlotFile(path string, file kcrypto.KeySlotFile) error {
	data, err := file.Marshal()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func passphraseValue(value string, envName string, filePath string, label string) ([]byte, error) {
	sources := 0
	if value != "" {
		sources++
	}
	if envName != "" {
		sources++
	}
	if filePath != "" {
		sources++
	}
	if sources != 1 {
		return nil, fmt.Errorf("exactly one %s source is required", label)
	}
	switch {
	case value != "":
		return []byte(value), nil
	case envName != "":
		value := os.Getenv(envName)
		if value == "" {
			return nil, fmt.Errorf("environment variable %s is empty", envName)
		}
		return []byte(value), nil
	default:
		data, err := readFileBounded(filePath, 1024*1024)
		if err != nil {
			return nil, err
		}
		return []byte(trimTrailingNewline(string(data))), nil
	}
}

func randomRootKey() ([]byte, error) {
	rootKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, rootKey); err != nil {
		return nil, fmt.Errorf("generate root key: %w", err)
	}
	return rootKey, nil
}

func trimTrailingNewline(value string) string {
	return strings.TrimRight(value, "\r\n")
}
