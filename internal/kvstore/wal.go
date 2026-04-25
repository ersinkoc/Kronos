package kvstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var rollbackWALMagic = []byte("KRWAL001")

func writeRollbackWAL(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("database path is required")
	}
	walPath := rollbackWALPath(dbPath)
	tmpPath := walPath + ".tmp"

	source, err := os.Open(dbPath)
	if err != nil {
		return err
	}
	defer source.Close()

	wal, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		wal.Close()
		if cleanup {
			os.Remove(tmpPath)
		}
	}()

	if _, err := wal.Write(rollbackWALMagic); err != nil {
		return err
	}
	if _, err := io.Copy(wal, source); err != nil {
		return err
	}
	if err := wal.Sync(); err != nil {
		return err
	}
	if err := wal.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, walPath); err != nil {
		return err
	}
	cleanup = false
	return syncParentDir(dbPath)
}

func recoverRollbackWAL(dbPath string) error {
	walPath := rollbackWALPath(dbPath)
	wal, err := os.Open(walPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer wal.Close()

	magic := make([]byte, len(rollbackWALMagic))
	if _, err := io.ReadFull(wal, magic); err != nil {
		return fmt.Errorf("read rollback wal magic: %w", err)
	}
	if !bytes.Equal(magic, rollbackWALMagic) {
		return fmt.Errorf("invalid rollback wal magic")
	}

	tmpPath := dbPath + ".recovering"
	recovered, err := os.OpenFile(tmpPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		recovered.Close()
		if cleanup {
			os.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(recovered, wal); err != nil {
		return err
	}
	if err := recovered.Sync(); err != nil {
		return err
	}
	if err := recovered.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, dbPath); err != nil {
		return err
	}
	if err := os.Remove(walPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	cleanup = false
	return syncParentDir(dbPath)
}

func removeRollbackWAL(dbPath string) error {
	err := os.Remove(rollbackWALPath(dbPath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncParentDir(dbPath)
}

func rollbackWALPath(dbPath string) string {
	return dbPath + ".wal"
}

func syncParentDir(path string) error {
	dir := filepath.Dir(path)
	file, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer file.Close()
	return file.Sync()
}
