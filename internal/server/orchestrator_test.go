package server

import (
	"errors"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	"github.com/kronos/kronos/internal/schedule"
)

func TestOrchestratorEnqueueStartFinish(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	clock := core.NewFakeClock(now)
	store := newTestJobStore(t)
	orchestrator, err := NewOrchestrator(store, clock)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	jobs, err := orchestrator.EnqueueDue([]schedule.DueJob{{
		ScheduleID:     "schedule-1",
		TargetID:       "target-1",
		StorageID:      "storage-1",
		Type:           core.BackupTypeFull,
		ParentBackupID: "backup-parent",
		QueuedAt:       now,
	}})
	if err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	if len(jobs) != 1 || jobs[0].Status != core.JobStatusQueued || jobs[0].ScheduleID != "schedule-1" || jobs[0].ParentBackupID != "backup-parent" {
		t.Fatalf("jobs = %#v", jobs)
	}

	clock.Advance(time.Minute)
	started, err := orchestrator.Start(jobs[0].ID)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if started.Status != core.JobStatusRunning || !started.StartedAt.Equal(clock.Now()) {
		t.Fatalf("started = %#v", started)
	}

	clock.Advance(time.Minute)
	finished, err := orchestrator.Finish(jobs[0].ID, core.JobStatusSucceeded, "")
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if finished.Status != core.JobStatusSucceeded || !finished.EndedAt.Equal(clock.Now()) {
		t.Fatalf("finished = %#v", finished)
	}
	if _, err := orchestrator.Finish(jobs[0].ID, core.JobStatusCanceled, "late cancel"); err == nil {
		t.Fatal("Finish(already terminal) error = nil, want error")
	}
}

func TestOrchestratorRejectsInvalidTransitions(t *testing.T) {
	t.Parallel()

	store := newTestJobStore(t)
	orchestrator, err := NewOrchestrator(store, core.NewFakeClock(time.Now().UTC()))
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	if _, err := orchestrator.Start("missing"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Start(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := orchestrator.EnqueueDue([]schedule.DueJob{{TargetID: "target"}}); err != nil {
		t.Fatalf("EnqueueDue() error = %v", err)
	}
	jobs, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if _, err := orchestrator.Finish(jobs[0].ID, core.JobStatusRunning, ""); err == nil {
		t.Fatal("Finish(running) error = nil, want terminal status error")
	}
}

func TestOrchestratorRetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	clock := core.NewFakeClock(now)
	store := newTestJobStore(t)
	orchestrator, err := NewOrchestrator(store, clock)
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}
	failed := core.Job{
		ID:        "job-failed",
		TargetID:  "target",
		StorageID: "storage",
		Type:      core.BackupTypeFull,
		AgentID:   "agent-1",
		Status:    core.JobStatusFailed,
		QueuedAt:  now.Add(-time.Hour),
		StartedAt: now.Add(-time.Hour),
		EndedAt:   now.Add(-time.Minute),
		Error:     "agent_lost",
	}
	if err := store.Save(failed); err != nil {
		t.Fatalf("Save(failed) error = %v", err)
	}
	clock.Advance(time.Minute)
	retried, err := orchestrator.Retry("job-failed")
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retried.Status != core.JobStatusQueued || retried.Error != "" || retried.AgentID != "" || !retried.StartedAt.IsZero() || !retried.EndedAt.IsZero() || !retried.QueuedAt.Equal(clock.Now()) {
		t.Fatalf("retried = %#v", retried)
	}
	if _, err := orchestrator.Retry("job-failed"); err == nil {
		t.Fatal("Retry(queued) error = nil, want error")
	}
}

func newTestJobStore(t *testing.T) *JobStore {
	t.Helper()
	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	return store
}
