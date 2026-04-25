package server

import (
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestTargetStoreSaveGetListDeleteReopen(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/state.db"
	db, err := kvstore.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store, err := NewTargetStore(db)
	if err != nil {
		t.Fatalf("NewTargetStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, target := range []core.Target{
		{ID: "target-b", Name: "bravo", Driver: core.TargetDriverRedis, Endpoint: "127.0.0.1:6379", CreatedAt: now, UpdatedAt: now},
		{ID: "target-a", Name: "alpha", Driver: core.TargetDriverPostgres, Endpoint: "127.0.0.1:5432", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Save(target); err != nil {
			t.Fatalf("Save(%s) error = %v", target.ID, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := kvstore.Open(path)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer reopened.Close()
	store, err = NewTargetStore(reopened)
	if err != nil {
		t.Fatalf("NewTargetStore(reopen) error = %v", err)
	}
	got, ok, err := store.Get("target-a")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.Name != "alpha" {
		t.Fatalf("Get(target-a) = %#v, %v", got, ok)
	}
	targets, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(targets) != 2 || targets[0].ID != "target-a" || targets[1].ID != "target-b" {
		t.Fatalf("List() = %#v", targets)
	}
	if err := store.Delete("target-a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("target-a"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestScheduleStoreSaveGetListDelete(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewScheduleStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, schedule := range []core.Schedule{
		{ID: "schedule-b", Name: "bravo", TargetID: "target", StorageID: "storage", BackupType: core.BackupTypeFull, Expression: "@between 02:00-04:00 UTC random", CreatedAt: now, UpdatedAt: now},
		{ID: "schedule-a", Name: "alpha", TargetID: "target", StorageID: "storage", BackupType: core.BackupTypeFull, Expression: "0 2 * * *", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Save(schedule); err != nil {
			t.Fatalf("Save(%s) error = %v", schedule.ID, err)
		}
	}
	got, ok, err := store.Get("schedule-a")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.Name != "alpha" {
		t.Fatalf("Get(schedule-a) = %#v, %v", got, ok)
	}
	schedules, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(schedules) != 2 || schedules[0].ID != "schedule-a" || schedules[1].ID != "schedule-b" {
		t.Fatalf("List() = %#v", schedules)
	}
	if err := store.Delete("schedule-a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("schedule-a"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestStorageStoreSaveGetListDelete(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewStorageStore(db)
	if err != nil {
		t.Fatalf("NewStorageStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, storage := range []core.Storage{
		{ID: "storage-b", Name: "bravo", Kind: core.StorageKindS3, URI: "s3://bucket", CreatedAt: now, UpdatedAt: now},
		{ID: "storage-a", Name: "alpha", Kind: core.StorageKindLocal, URI: "file:///repo", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Save(storage); err != nil {
			t.Fatalf("Save(%s) error = %v", storage.ID, err)
		}
	}
	got, ok, err := store.Get("storage-a")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.Name != "alpha" {
		t.Fatalf("Get(storage-a) = %#v, %v", got, ok)
	}
	storages, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(storages) != 2 || storages[0].ID != "storage-a" || storages[1].ID != "storage-b" {
		t.Fatalf("List() = %#v", storages)
	}
	if err := store.Delete("storage-a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("storage-a"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestBackupStoreSaveListProtect(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewBackupStore(db)
	if err != nil {
		t.Fatalf("NewBackupStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, backup := range []core.Backup{
		{ID: "backup-old", TargetID: "target", StorageID: "storage", JobID: "job-1", Type: core.BackupTypeFull, ManifestID: "manifest-1", StartedAt: now.Add(-3 * time.Hour), EndedAt: now.Add(-2 * time.Hour), SizeBytes: 10, ChunkCount: 1},
		{ID: "backup-new", TargetID: "target", StorageID: "storage", JobID: "job-2", Type: core.BackupTypeFull, ManifestID: "manifest-2", StartedAt: now.Add(-time.Hour), EndedAt: now, SizeBytes: 20, ChunkCount: 2},
	} {
		if err := store.Save(backup); err != nil {
			t.Fatalf("Save(%s) error = %v", backup.ID, err)
		}
	}
	backups, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(backups) != 2 || backups[0].ID != "backup-new" || backups[1].ID != "backup-old" {
		t.Fatalf("List() = %#v", backups)
	}
	protected, err := store.Protect("backup-old", true)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if !protected.Protected {
		t.Fatalf("protected backup = %#v", protected)
	}
	got, ok, err := store.Get("backup-old")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || !got.Protected {
		t.Fatalf("Get(backup-old) = %#v, %v", got, ok)
	}
	if err := store.Delete("backup-old"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("backup-old"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestResourceStoresValidateRequiredFields(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	targets, err := NewTargetStore(db)
	if err != nil {
		t.Fatalf("NewTargetStore() error = %v", err)
	}
	if err := targets.Save(core.Target{}); err == nil {
		t.Fatal("Save(empty target) error = nil, want error")
	}
	storages, err := NewStorageStore(db)
	if err != nil {
		t.Fatalf("NewStorageStore() error = %v", err)
	}
	if err := storages.Save(core.Storage{}); err == nil {
		t.Fatal("Save(empty storage) error = nil, want error")
	}
	schedules, err := NewScheduleStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStore() error = %v", err)
	}
	err = schedules.Save(core.Schedule{ID: "schedule", Name: "bad", TargetID: "target", StorageID: "storage", BackupType: core.BackupTypeFull, Expression: "bad cron"})
	if err == nil {
		t.Fatal("Save(bad schedule) error = nil, want error")
	}
	backups, err := NewBackupStore(db)
	if err != nil {
		t.Fatalf("NewBackupStore() error = %v", err)
	}
	if err := backups.Save(core.Backup{}); err == nil {
		t.Fatal("Save(empty backup) error = nil, want error")
	}
}
