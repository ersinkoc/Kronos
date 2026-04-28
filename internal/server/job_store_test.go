package server

import (
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestJobStoreSaveGetListReopen(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/state.db"
	db, err := kvstore.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	jobs := []core.Job{
		{ID: "job-2", TargetID: "target", StorageID: "storage", Type: core.BackupTypeFull, Status: core.JobStatusQueued, QueuedAt: now.Add(time.Minute)},
		{ID: "job-1", TargetID: "target", StorageID: "storage", Type: core.BackupTypeFull, Status: core.JobStatusRunning, QueuedAt: now},
	}
	for _, job := range jobs {
		if err := store.Save(job); err != nil {
			t.Fatalf("Save(%s) error = %v", job.ID, err)
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
	store, err = NewJobStore(reopened)
	if err != nil {
		t.Fatalf("NewJobStore(reopen) error = %v", err)
	}
	got, ok, err := store.Get("job-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.Status != core.JobStatusRunning {
		t.Fatalf("Get(job-1) = %#v, %v", got, ok)
	}
	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 || list[0].ID != "job-1" || list[1].ID != "job-2" {
		t.Fatalf("List() = %#v, want queued-time order", list)
	}
}

func TestJobStoreDelete(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	if err := store.Save(core.Job{ID: "job-1", Status: core.JobStatusQueued}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := store.Delete("job-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("job-1"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want false nil", ok, err)
	}
}

func TestEvidenceStoreSaveGetByJobIDReopen(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/state.db"
	db, err := kvstore.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store, err := NewEvidenceStore(db)
	if err != nil {
		t.Fatalf("NewEvidenceStore() error = %v", err)
	}
	artifact := core.EvidenceArtifact{
		ID:        "artifact-1",
		JobID:     "job-1",
		Kind:      "restore",
		SHA256:    "abc123",
		CreatedAt: time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC),
		Restore: &core.RestoreEvidence{
			Status: core.JobStatusSucceeded,
		},
	}
	if err := store.Save(artifact); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := kvstore.Open(path)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer reopened.Close()
	store, err = NewEvidenceStore(reopened)
	if err != nil {
		t.Fatalf("NewEvidenceStore(reopen) error = %v", err)
	}
	got, ok, err := store.GetByJobID("job-1")
	if err != nil {
		t.Fatalf("GetByJobID() error = %v", err)
	}
	if !ok || got.ID != "artifact-1" || got.SHA256 != "abc123" || got.Restore == nil {
		t.Fatalf("GetByJobID() = %#v, %v", got, ok)
	}
}

func TestJobStoreFailActive(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, job := range []core.Job{
		{ID: "queued", Status: core.JobStatusQueued, QueuedAt: now},
		{ID: "running", Status: core.JobStatusRunning, QueuedAt: now},
		{ID: "finalizing", Status: core.JobStatusFinalizing, QueuedAt: now},
		{ID: "done", Status: core.JobStatusSucceeded, QueuedAt: now},
	} {
		if err := store.Save(job); err != nil {
			t.Fatalf("Save(%s) error = %v", job.ID, err)
		}
	}

	changed, err := store.FailActive(now.Add(time.Minute), "server_lost")
	if err != nil {
		t.Fatalf("FailActive() error = %v", err)
	}
	if changed != 2 {
		t.Fatalf("FailActive() = %d, want 2", changed)
	}
	for _, id := range []core.ID{"running", "finalizing"} {
		job, ok, err := store.Get(id)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", id, err)
		}
		if !ok || job.Status != core.JobStatusFailed || job.Error != "server_lost" || job.EndedAt.IsZero() {
			t.Fatalf("job %s = %#v", id, job)
		}
	}
	queued, _, err := store.Get("queued")
	if err != nil {
		t.Fatalf("Get(queued) error = %v", err)
	}
	if queued.Status != core.JobStatusQueued {
		t.Fatalf("queued status = %s, want queued", queued.Status)
	}
}

func TestJobStoreRejectsEmptyID(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	if err := store.Save(core.Job{}); err == nil {
		t.Fatal("Save(empty id) error = nil, want error")
	}
}
