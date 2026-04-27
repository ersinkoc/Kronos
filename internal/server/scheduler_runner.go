package server

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	sched "github.com/kronos/kronos/internal/schedule"
)

var scheduleStatesBucket = []byte("schedule_states")

// ScheduleStateStore persists mutable scheduler cursors separately from schedule definitions.
type ScheduleStateStore struct {
	db *kvstore.DB
}

// NewScheduleStateStore returns a schedule state store backed by db.
func NewScheduleStateStore(db *kvstore.DB) (*ScheduleStateStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &ScheduleStateStore{db: db}, nil
}

// Get fetches one persisted schedule cursor.
func (s *ScheduleStateStore) Get(id core.ID) (sched.ScheduleState, bool, error) {
	var state sched.ScheduleState
	ok, err := getJSON(s.db, scheduleStatesBucket, []byte(id), &state)
	return state, ok, err
}

// List returns persisted schedule cursors ordered by schedule ID.
func (s *ScheduleStateStore) List() ([]sched.ScheduleState, error) {
	var states []sched.ScheduleState
	if err := listJSON(s.db, scheduleStatesBucket, func(data []byte) error {
		var state sched.ScheduleState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		states = append(states, state)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(states, func(i, j int) bool {
		return states[i].Schedule.ID < states[j].Schedule.ID
	})
	return states, nil
}

// Save inserts or replaces one schedule cursor.
func (s *ScheduleStateStore) Save(state sched.ScheduleState) error {
	if state.Schedule.ID.IsZero() {
		return fmt.Errorf("schedule id is required")
	}
	return saveJSON(s.db, scheduleStatesBucket, []byte(state.Schedule.ID), state)
}

// Delete removes one schedule cursor.
func (s *ScheduleStateStore) Delete(id core.ID) error {
	return deleteKey(s.db, scheduleStatesBucket, []byte(id))
}

// SchedulerRunner evaluates persisted schedules and enqueues due backup jobs.
type SchedulerRunner struct {
	Schedules *ScheduleStore
	States    *ScheduleStateStore
	Jobs      *JobStore
	Backups   *BackupStore
	Clock     core.Clock
}

// NewSchedulerRunner returns a scheduler runner.
func NewSchedulerRunner(schedules *ScheduleStore, states *ScheduleStateStore, jobs *JobStore, clock core.Clock) (*SchedulerRunner, error) {
	if schedules == nil {
		return nil, fmt.Errorf("schedule store is required")
	}
	if states == nil {
		return nil, fmt.Errorf("schedule state store is required")
	}
	if jobs == nil {
		return nil, fmt.Errorf("job store is required")
	}
	if clock == nil {
		clock = core.RealClock{}
	}
	return &SchedulerRunner{Schedules: schedules, States: states, Jobs: jobs, Clock: clock}, nil
}

// Tick evaluates all schedules once and persists updated cursors.
func (r *SchedulerRunner) Tick() ([]core.Job, error) {
	schedules, err := r.Schedules.List()
	if err != nil {
		return nil, err
	}
	states := make([]sched.ScheduleState, 0, len(schedules))
	for _, schedule := range schedules {
		state, ok, err := r.States.Get(schedule.ID)
		if err != nil {
			return nil, err
		}
		if !ok {
			state = sched.ScheduleState{Schedule: schedule}
		}
		state.Schedule = schedule
		states = append(states, state)
	}
	due, updated, err := sched.Tick(states, r.Clock.Now().UTC())
	if err != nil {
		return nil, err
	}
	for _, state := range updated {
		if err := r.States.Save(state); err != nil {
			return nil, err
		}
	}
	if len(due) == 0 {
		return nil, nil
	}
	if r.Backups != nil {
		due, err = r.attachParentBackups(due)
		if err != nil {
			return nil, err
		}
	}
	orchestrator, err := NewOrchestrator(r.Jobs, r.Clock)
	if err != nil {
		return nil, err
	}
	return orchestrator.EnqueueDue(due)
}

func (r *SchedulerRunner) attachParentBackups(due []sched.DueJob) ([]sched.DueJob, error) {
	backups, err := r.Backups.List()
	if err != nil {
		return nil, err
	}
	out := append([]sched.DueJob(nil), due...)
	for i := range out {
		if !scheduledBackupNeedsParent(out[i].Type) {
			continue
		}
		parent, ok := selectParentBackup(backups, out[i])
		if !ok {
			out[i].Type = core.BackupTypeFull
			out[i].ParentBackupID = ""
			continue
		}
		out[i].ParentBackupID = parent.ID
	}
	return out, nil
}

func scheduledBackupNeedsParent(backupType core.BackupType) bool {
	return backupType == core.BackupTypeIncremental || backupType == core.BackupTypeDifferential
}

func selectParentBackup(backups []core.Backup, due sched.DueJob) (core.Backup, bool) {
	for _, backup := range backups {
		if backup.TargetID != due.TargetID || backup.StorageID != due.StorageID || backup.ManifestID.IsZero() {
			continue
		}
		switch due.Type {
		case core.BackupTypeDifferential:
			if backup.Type != core.BackupTypeFull {
				continue
			}
		case core.BackupTypeIncremental:
			if backup.Type != core.BackupTypeFull && backup.Type != core.BackupTypeIncremental && backup.Type != core.BackupTypeDifferential {
				continue
			}
		}
		return backup, true
	}
	return core.Backup{}, false
}
