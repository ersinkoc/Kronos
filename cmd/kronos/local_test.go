package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunLocalStartsServerAndStateDB(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}
	dataDir := filepath.Join(t.TempDir(), "local-state")
	done := make(chan error, 1)
	go func() {
		done <- runLocal(ctx, out, []string{"--listen", "127.0.0.1:0", "--data-dir", dataDir})
	}()
	for !strings.Contains(out.String(), "kronos-server listening=") {
		time.Sleep(time.Millisecond)
	}
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runLocal() error = %v, want context.Canceled", err)
	}
	db, _, err := openServerState(dataDir)
	if err != nil {
		t.Fatalf("openServerState(local data dir) error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close(local state db) error = %v", err)
	}
}

func TestRunLocalCanStartEmbeddedWorker(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{5}, 64)))
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}
	dataDir := filepath.Join(t.TempDir(), "local-worker-state")
	done := make(chan error, 1)
	go func() {
		done <- runLocal(ctx, out, []string{
			"--listen", "127.0.0.1:0",
			"--data-dir", dataDir,
			"--work",
			"--id", "local-agent",
			"--manifest-private-key", hex.EncodeToString(privateKey),
			"--chunk-key", hex.EncodeToString(bytes.Repeat([]byte{9}, 32)),
			"--heartbeat-interval", "1ms",
		})
	}()
	for !strings.Contains(out.String(), "kronos-local worker=local-agent") {
		time.Sleep(time.Millisecond)
	}
	cancel()
	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runLocal(--work) error = %v, want context.Canceled", err)
	}
}

func TestRunLocalLoadsConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dataDir := filepath.Join(dir, "state")
	path := filepath.Join(dir, "kronos.yaml")
	data := []byte(`
server:
  listen: "127.0.0.1:0"
  data_dir: "` + dataDir + `"
projects:
  - name: default
    storages:
      - name: local
        backend: local
        path: "/tmp/repo"
    targets:
      - name: redis
        driver: redis
        connection:
          host: "127.0.0.1"
          port: 6379
    schedules:
      - name: redis-nightly
        target: redis
        type: full
        cron: "0 2 * * *"
        storage: local
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}
	done := make(chan error, 1)
	go func() {
		done <- runLocal(ctx, out, []string{"--config", path})
	}()
	for !strings.Contains(out.String(), "projects=1") {
		time.Sleep(time.Millisecond)
	}
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runLocal(--config) error = %v, want context.Canceled", err)
	}
	db, _, err := openServerState(dataDir)
	if err != nil {
		t.Fatalf("openServerState(config data dir) error = %v", err)
	}
	defer db.Close()
	stores, err := newAPIStores(db)
	if err != nil {
		t.Fatalf("newAPIStores() error = %v", err)
	}
	if _, ok, err := stores.schedules.Get("default/redis-nightly"); err != nil || !ok {
		t.Fatalf("Get(schedule) ok=%v err=%v", ok, err)
	}
}
