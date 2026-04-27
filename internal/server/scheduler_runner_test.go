package server

import (
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	sched "github.com/kronos/kronos/internal/schedule"
)

func TestSchedulerRunnerTickEnqueuesDueJobsAndPersistsState(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	schedules, err := NewScheduleStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStore() error = %v", err)
	}
	states, err := NewScheduleStateStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStateStore() error = %v", err)
	}
	jobs, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	now := created.Add(time.Hour)
	if err := schedules.Save(core.Schedule{
		ID:         "schedule-1",
		Name:       "hourly",
		TargetID:   "target-1",
		StorageID:  "storage-1",
		BackupType: core.BackupTypeFull,
		Expression: "0 * * * *",
		CreatedAt:  created,
		UpdatedAt:  created,
	}); err != nil {
		t.Fatalf("Save(schedule) error = %v", err)
	}
	clock := core.NewFakeClock(now)
	runner, err := NewSchedulerRunner(schedules, states, jobs, clock)
	if err != nil {
		t.Fatalf("NewSchedulerRunner() error = %v", err)
	}
	createdJobs, err := runner.Tick()
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(createdJobs) != 1 || createdJobs[0].ScheduleID != "schedule-1" || createdJobs[0].Status != core.JobStatusQueued {
		t.Fatalf("created jobs = %#v", createdJobs)
	}
	persisted, ok, err := states.Get("schedule-1")
	if err != nil || !ok {
		t.Fatalf("Get(state) ok=%v err=%v", ok, err)
	}
	if !persisted.LastRun.Equal(now) || !persisted.NextRun.Equal(now.Add(time.Hour)) {
		t.Fatalf("state = %#v", persisted)
	}
	list, err := jobs.List()
	if err != nil {
		t.Fatalf("List(jobs) error = %v", err)
	}
	if len(list) != 1 || list[0].TargetID != "target-1" || list[0].StorageID != "storage-1" {
		t.Fatalf("jobs = %#v", list)
	}

	createdJobs, err = runner.Tick()
	if err != nil {
		t.Fatalf("Tick(second) error = %v", err)
	}
	if len(createdJobs) != 0 {
		t.Fatalf("second tick jobs = %#v, want none", createdJobs)
	}
}

func TestSchedulerRunnerSkipsPausedSchedules(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	schedules, err := NewScheduleStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStore() error = %v", err)
	}
	states, err := NewScheduleStateStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStateStore() error = %v", err)
	}
	jobs, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	if err := schedules.Save(core.Schedule{
		ID: "schedule-paused", Name: "paused", TargetID: "target", StorageID: "storage",
		BackupType: core.BackupTypeFull, Expression: "0 * * * *", CreatedAt: now.Add(-time.Hour), UpdatedAt: now, Paused: true,
	}); err != nil {
		t.Fatalf("Save(schedule) error = %v", err)
	}
	runner, err := NewSchedulerRunner(schedules, states, jobs, core.NewFakeClock(now))
	if err != nil {
		t.Fatalf("NewSchedulerRunner() error = %v", err)
	}
	createdJobs, err := runner.Tick()
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(createdJobs) != 0 {
		t.Fatalf("created jobs = %#v, want none", createdJobs)
	}
}

func TestScheduleStateStoreDeleteAndRunnerValidation(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	states, err := NewScheduleStateStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStateStore() error = %v", err)
	}
	if err := states.Save(sched.ScheduleState{Schedule: core.Schedule{ID: "schedule-1"}}); err != nil {
		t.Fatalf("Save(state) error = %v", err)
	}
	if err := states.Save(sched.ScheduleState{Schedule: core.Schedule{ID: "schedule-0"}}); err != nil {
		t.Fatalf("Save(second state) error = %v", err)
	}
	list, err := states.List()
	if err != nil {
		t.Fatalf("List(states) error = %v", err)
	}
	if len(list) != 2 || list[0].Schedule.ID != "schedule-0" || list[1].Schedule.ID != "schedule-1" {
		t.Fatalf("states = %#v", list)
	}
	if err := states.Delete("schedule-1"); err != nil {
		t.Fatalf("Delete(state) error = %v", err)
	}
	if _, ok, err := states.Get("schedule-1"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want false nil", ok, err)
	}
	if err := states.Save(sched.ScheduleState{}); err == nil {
		t.Fatal("Save(empty schedule id) error = nil, want error")
	}
	if _, err := NewScheduleStateStore(nil); err == nil {
		t.Fatal("NewScheduleStateStore(nil) error = nil, want error")
	}
	if _, err := NewSchedulerRunner(nil, states, nil, nil); err == nil {
		t.Fatal("NewSchedulerRunner(nil schedules) error = nil, want error")
	}
}

func TestSchedulerRunnerAttachesParentBackups(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	schedules, err := NewScheduleStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStore() error = %v", err)
	}
	states, err := NewScheduleStateStore(db)
	if err != nil {
		t.Fatalf("NewScheduleStateStore() error = %v", err)
	}
	jobs, err := NewJobStore(db)
	if err != nil {
		t.Fatalf("NewJobStore() error = %v", err)
	}
	backups, err := NewBackupStore(db)
	if err != nil {
		t.Fatalf("NewBackupStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	for _, schedule := range []core.Schedule{
		{ID: "schedule-incr", Name: "incr", TargetID: "target", StorageID: "storage", BackupType: core.BackupTypeIncremental, Expression: "0 * * * *", CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
		{ID: "schedule-diff", Name: "diff", TargetID: "target", StorageID: "storage", BackupType: core.BackupTypeDifferential, Expression: "0 * * * *", CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
		{ID: "schedule-seed", Name: "seed", TargetID: "empty-target", StorageID: "storage", BackupType: core.BackupTypeIncremental, Expression: "0 * * * *", CreatedAt: now.Add(-time.Hour), UpdatedAt: now},
	} {
		if err := schedules.Save(schedule); err != nil {
			t.Fatalf("Save(schedule %s) error = %v", schedule.ID, err)
		}
	}
	for _, backup := range []core.Backup{
		{ID: "backup-full", TargetID: "target", StorageID: "storage", JobID: "job-full", Type: core.BackupTypeFull, ManifestID: "manifest-full", StartedAt: now.Add(-3 * time.Hour), EndedAt: now.Add(-3 * time.Hour)},
		{ID: "backup-incr", TargetID: "target", StorageID: "storage", JobID: "job-incr", Type: core.BackupTypeIncremental, ParentID: "backup-full", ManifestID: "manifest-incr", StartedAt: now.Add(-time.Hour), EndedAt: now.Add(-time.Hour)},
	} {
		if err := backups.Save(backup); err != nil {
			t.Fatalf("Save(backup %s) error = %v", backup.ID, err)
		}
	}
	runner, err := NewSchedulerRunner(schedules, states, jobs, core.NewFakeClock(now))
	if err != nil {
		t.Fatalf("NewSchedulerRunner() error = %v", err)
	}
	runner.Backups = backups
	createdJobs, err := runner.Tick()
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	bySchedule := make(map[core.ID]core.Job, len(createdJobs))
	for _, job := range createdJobs {
		bySchedule[job.ScheduleID] = job
	}
	if got := bySchedule["schedule-incr"]; got.Type != core.BackupTypeIncremental || got.ParentBackupID != "backup-incr" {
		t.Fatalf("incremental job = %#v", got)
	}
	if got := bySchedule["schedule-diff"]; got.Type != core.BackupTypeDifferential || got.ParentBackupID != "backup-full" {
		t.Fatalf("differential job = %#v", got)
	}
	if got := bySchedule["schedule-seed"]; got.Type != core.BackupTypeFull || !got.ParentBackupID.IsZero() {
		t.Fatalf("seed job = %#v", got)
	}
}
