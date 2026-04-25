package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeySlotLifecycle(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	escrowPath := filepath.Join(dir, "escrow.json")
	rootKey := bytes.Repeat([]byte{0x42}, 32)
	t.Setenv("KRONOS_SLOT_PASS", "slot-passphrase")
	t.Setenv("KRONOS_BREAKGLASS_PASS", "breakglass-passphrase")

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{
		"key", "add-slot",
		"--file", path,
		"--id", "ops",
		"--root-key", hex.EncodeToString(rootKey),
		"--passphrase-env", "KRONOS_SLOT_PASS",
	}); err != nil {
		t.Fatalf("key add-slot error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) || !strings.Contains(out.String(), `"slot":"ops"`) {
		t.Fatalf("key add-slot output = %q", out.String())
	}

	out.Reset()
	if err := run(context.Background(), &out, []string{
		"key", "add-slot",
		"--file", path,
		"--id", "breakglass",
		"--unlock-slot", "ops",
		"--unlock-passphrase-env", "KRONOS_SLOT_PASS",
		"--passphrase-env", "KRONOS_BREAKGLASS_PASS",
	}); err != nil {
		t.Fatalf("key add-slot second error = %v", err)
	}
	if !strings.Contains(out.String(), `"slots":2`) {
		t.Fatalf("key add-slot second output = %q", out.String())
	}

	out.Reset()
	if err := run(context.Background(), &out, []string{"key", "list", "--file", path}); err != nil {
		t.Fatalf("key list error = %v", err)
	}
	if !strings.Contains(out.String(), `"id":"ops"`) || !strings.Contains(out.String(), `"id":"breakglass"`) || strings.Contains(out.String(), "slot-passphrase") {
		t.Fatalf("key list output = %q", out.String())
	}

	out.Reset()
	if err := run(context.Background(), &out, []string{"key", "escrow", "export", "--file", path, "--out", escrowPath}); err != nil {
		t.Fatalf("key escrow export error = %v", err)
	}
	if !strings.Contains(out.String(), `"slots":2`) {
		t.Fatalf("key escrow export output = %q", out.String())
	}
	if _, err := os.Stat(escrowPath); err != nil {
		t.Fatalf("Stat(escrow) error = %v", err)
	}

	out.Reset()
	if err := run(context.Background(), &out, []string{"key", "remove-slot", "--file", path, "--id", "ops"}); err != nil {
		t.Fatalf("key remove-slot error = %v", err)
	}
	if !strings.Contains(out.String(), `"slots":1`) {
		t.Fatalf("key remove-slot output = %q", out.String())
	}
	if err := run(context.Background(), &out, []string{"key", "remove-slot", "--file", path, "--id", "breakglass"}); err == nil {
		t.Fatal("key remove last slot error = nil, want error")
	}
}

func TestRunKeyRotate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.json")
	rotatedPath := filepath.Join(dir, "keys-rotated.json")
	rootKey := bytes.Repeat([]byte{0x11}, 32)
	t.Setenv("KRONOS_OLD_PASS", "old-passphrase")
	t.Setenv("KRONOS_NEW_PASS", "new-passphrase")

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{
		"key", "add-slot",
		"--file", path,
		"--id", "ops",
		"--root-key", hex.EncodeToString(rootKey),
		"--passphrase-env", "KRONOS_OLD_PASS",
	}); err != nil {
		t.Fatalf("key add-slot error = %v", err)
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{
		"key", "rotate",
		"--file", path,
		"--out", rotatedPath,
		"--id", "ops-rotated",
		"--unlock-slot", "ops",
		"--unlock-passphrase-env", "KRONOS_OLD_PASS",
		"--passphrase-env", "KRONOS_NEW_PASS",
	}); err != nil {
		t.Fatalf("key rotate error = %v", err)
	}
	if !strings.Contains(out.String(), `"ok":true`) || !strings.Contains(out.String(), `"old_root_key_fingerprint"`) || !strings.Contains(out.String(), `"new_root_key_fingerprint"`) {
		t.Fatalf("key rotate output = %q", out.String())
	}
	if _, err := os.Stat(rotatedPath); err != nil {
		t.Fatalf("Stat(rotated) error = %v", err)
	}
}

func TestTrimTrailingNewline(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"secret\n":   "secret",
		"secret\r\n": "secret",
		"secret":     "secret",
		"secret\n\n": "secret",
	} {
		if got := trimTrailingNewline(input); got != want {
			t.Fatalf("trimTrailingNewline(%q) = %q, want %q", input, got, want)
		}
	}
}
