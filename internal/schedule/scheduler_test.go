package schedule

import (
	"fmt"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
)

func TestTickQueuesDueSchedule(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	now := created.Add(time.Hour)
	state := ScheduleState{
		Schedule: core.Schedule{
			ID:         "schedule-1",
			TargetID:   "target-1",
			StorageID:  "storage-1",
			BackupType: core.BackupTypeFull,
			Expression: "0 * * * *",
			CreatedAt:  created,
		},
	}

	jobs, updated, err := Tick([]ScheduleState{state}, now)
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1", len(jobs))
	}
	if jobs[0].ScheduleID != "schedule-1" || jobs[0].DueAt != now {
		t.Fatalf("job = %#v", jobs[0])
	}
	wantNext := now.Add(time.Hour)
	if !updated[0].LastRun.Equal(now) || !updated[0].NextRun.Equal(wantNext) {
		t.Fatalf("updated = %#v, want last=%s next=%s", updated[0], now, wantNext)
	}
}

func TestTickCatchUpPolicies(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	now := created.Add(3 * time.Hour)
	base := ScheduleState{
		Schedule: core.Schedule{
			ID:         "schedule-1",
			TargetID:   "target-1",
			StorageID:  "storage-1",
			BackupType: core.BackupTypeFull,
			Expression: "0 * * * *",
			CreatedAt:  created,
		},
	}

	queueState := base
	queueState.CatchUpPolicy = CatchUpQueue
	jobs, _, err := Tick([]ScheduleState{queueState}, now)
	if err != nil {
		t.Fatalf("Tick(queue) error = %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("queue jobs = %d, want 3", len(jobs))
	}

	runOnceState := base
	runOnceState.CatchUpPolicy = CatchUpRunOnce
	jobs, _, err = Tick([]ScheduleState{runOnceState}, now)
	if err != nil {
		t.Fatalf("Tick(run_once) error = %v", err)
	}
	if len(jobs) != 1 || !jobs[0].DueAt.Equal(now) {
		t.Fatalf("run_once jobs = %#v, want one job at now", jobs)
	}

	skipState := base
	skipState.CatchUpPolicy = CatchUpSkip
	jobs, _, err = Tick([]ScheduleState{skipState}, now)
	if err != nil {
		t.Fatalf("Tick(skip) error = %v", err)
	}
	if len(jobs) != 1 || !jobs[0].DueAt.Equal(now) {
		t.Fatalf("skip jobs = %#v, want latest due job", jobs)
	}
}

func TestTickSkipsPausedAndFutureSchedules(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 10, 30, 0, 0, time.UTC)
	states := []ScheduleState{
		{Schedule: core.Schedule{ID: "paused", Expression: "0 * * * *", CreatedAt: now.Add(-time.Hour), Paused: true}},
		{Schedule: core.Schedule{ID: "future", Expression: "0 * * * *", CreatedAt: now}},
	}
	jobs, updated, err := Tick(states, now)
	if err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs = %#v, want none", jobs)
	}
	if !updated[1].NextRun.Equal(time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("future next = %s", updated[1].NextRun)
	}
}

func TestTickSupportsBetweenWindow(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 4, 25, 1, 0, 0, 0, time.UTC)
	now := time.Date(2026, 4, 25, 3, 0, 0, 0, time.UTC)
	state := ScheduleState{
		Schedule: core.Schedule{
			ID:         "window",
			TargetID:   "target-1",
			StorageID:  "storage-1",
			BackupType: core.BackupTypeFull,
			Expression: "@between 02:00-04:00 UTC",
			CreatedAt:  created,
		},
	}

	jobs, updated, err := Tick([]ScheduleState{state}, now)
	if err != nil {
		t.Fatalf("Tick(window) error = %v", err)
	}
	if len(jobs) != 1 || !jobs[0].DueAt.Equal(time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("jobs = %#v, want one window job", jobs)
	}
	if !updated[0].NextRun.Equal(time.Date(2026, 4, 26, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("next window = %s", updated[0].NextRun)
	}
}

func TestTickRejectsExcessCatchUp(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	state := ScheduleState{
		Schedule:      core.Schedule{ID: "too-many", Expression: "* * * * * *", CreatedAt: created},
		CatchUpPolicy: CatchUpQueue,
		MaxCatchUp:    2,
	}
	if _, _, err := Tick([]ScheduleState{state}, created.Add(3*time.Second)); err == nil {
		t.Fatal("Tick() error = nil, want max catch-up error")
	}
}

func TestTargetQueueSerializesPerTarget(t *testing.T) {
	t.Parallel()

	queue := NewTargetQueue()
	jobs := []DueJob{
		{ScheduleID: "a", TargetID: "target-1"},
		{ScheduleID: "b", TargetID: "target-1"},
		{ScheduleID: "c", TargetID: "target-2"},
	}
	ready := queue.Enqueue(jobs...)
	if len(ready) != 2 {
		t.Fatalf("ready = %#v, want first target-1 and target-2", ready)
	}
	if queue.Pending("target-1") != 1 {
		t.Fatalf("target-1 pending = %d, want 1", queue.Pending("target-1"))
	}
	next, ok := queue.Complete("target-1")
	if !ok || next.ScheduleID != "b" {
		t.Fatalf("Complete(target-1) = %#v %v, want job b", next, ok)
	}
	if _, ok := queue.Complete("target-1"); ok {
		t.Fatal("Complete(target-1) returned extra job")
	}
}

func BenchmarkTickTenThousandSchedules(b *testing.B) {
	created := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	states := make([]ScheduleState, 10000)
	for i := range states {
		states[i] = ScheduleState{
			Schedule: core.Schedule{
				ID:         core.ID(fmt.Sprintf("schedule-%05d", i)),
				TargetID:   core.ID(fmt.Sprintf("target-%05d", i)),
				StorageID:  "storage",
				BackupType: core.BackupTypeFull,
				Expression: "0 * * * *",
				CreatedAt:  created,
			},
		}
	}
	now := created.Add(time.Hour)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := Tick(states, now); err != nil {
			b.Fatal(err)
		}
	}
}
