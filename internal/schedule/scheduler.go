package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/kronos/kronos/internal/core"
)

// CatchUpPolicy controls how missed schedule occurrences are materialized.
type CatchUpPolicy string

const (
	// CatchUpSkip advances past missed occurrences and emits only the latest due run.
	CatchUpSkip CatchUpPolicy = "skip"
	// CatchUpQueue emits every missed occurrence up to MaxCatchUp.
	CatchUpQueue CatchUpPolicy = "queue"
	// CatchUpRunOnce collapses all missed occurrences into one run at now.
	CatchUpRunOnce CatchUpPolicy = "run_once"
)

// ScheduleState is mutable scheduler state for one schedule.
type ScheduleState struct {
	Schedule      core.Schedule
	LastRun       time.Time
	NextRun       time.Time
	CatchUpPolicy CatchUpPolicy
	MaxCatchUp    int
}

// DueJob is a scheduled backup occurrence ready to enqueue.
type DueJob struct {
	ScheduleID     core.ID
	TargetID       core.ID
	StorageID      core.ID
	Type           core.BackupType
	ParentBackupID core.ID
	DueAt          time.Time
	QueuedAt       time.Time
}

// Tick evaluates schedules at now and returns due jobs plus updated states.
func Tick(states []ScheduleState, now time.Time) ([]DueJob, []ScheduleState, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updated := make([]ScheduleState, len(states))
	var jobs []DueJob
	for i, state := range states {
		nextState, due, err := tickOne(state, now)
		if err != nil {
			return nil, nil, err
		}
		updated[i] = nextState
		jobs = append(jobs, due...)
	}
	return jobs, updated, nil
}

func tickOne(state ScheduleState, now time.Time) (ScheduleState, []DueJob, error) {
	if state.Schedule.Paused {
		return state, nil, nil
	}
	nextFn, err := nextFunc(state.Schedule)
	if err != nil {
		return state, nil, fmt.Errorf("schedule %q: %w", state.Schedule.ID, err)
	}
	next := state.NextRun
	if next.IsZero() {
		seed := state.LastRun
		if seed.IsZero() {
			seed = state.Schedule.CreatedAt
		}
		if seed.IsZero() {
			seed = now
		}
		next, err = nextFn(seed)
		if err != nil {
			return state, nil, err
		}
	}
	if next.After(now) {
		state.NextRun = next
		return state, nil, nil
	}

	dueTimes := make([]time.Time, 0, 1)
	limit := state.MaxCatchUp
	if limit <= 0 {
		limit = 1000
	}
	for !next.After(now) {
		dueTimes = append(dueTimes, next)
		if len(dueTimes) > limit {
			return state, nil, fmt.Errorf("schedule %q exceeded max catch-up runs %d", state.Schedule.ID, limit)
		}
		next, err = nextFn(next)
		if err != nil {
			return state, nil, err
		}
	}

	state.LastRun = dueTimes[len(dueTimes)-1]
	state.NextRun = next
	return state, dueJobsForPolicy(state, dueTimes, now), nil
}

func nextFunc(schedule core.Schedule) (func(time.Time) (time.Time, error), error) {
	if strings.HasPrefix(strings.TrimSpace(schedule.Expression), "@between") {
		window, err := ParseWindow(schedule.Expression)
		if err != nil {
			return nil, err
		}
		stableKey := schedule.ID.String()
		if stableKey == "" {
			stableKey = schedule.Name
		}
		return func(after time.Time) (time.Time, error) {
			return window.Next(after, stableKey)
		}, nil
	}
	cron, err := ParseCron(schedule.Expression)
	if err != nil {
		return nil, err
	}
	return cron.Next, nil
}

func dueJobsForPolicy(state ScheduleState, dueTimes []time.Time, now time.Time) []DueJob {
	if len(dueTimes) == 0 {
		return nil
	}
	policy := state.CatchUpPolicy
	if policy == "" {
		policy = CatchUpSkip
	}
	switch policy {
	case CatchUpQueue:
		jobs := make([]DueJob, 0, len(dueTimes))
		for _, dueAt := range dueTimes {
			jobs = append(jobs, dueJob(state.Schedule, dueAt, now))
		}
		return jobs
	case CatchUpRunOnce:
		return []DueJob{dueJob(state.Schedule, now, now)}
	case CatchUpSkip:
		return []DueJob{dueJob(state.Schedule, dueTimes[len(dueTimes)-1], now)}
	default:
		return []DueJob{dueJob(state.Schedule, dueTimes[len(dueTimes)-1], now)}
	}
}

func dueJob(schedule core.Schedule, dueAt time.Time, queuedAt time.Time) DueJob {
	return DueJob{
		ScheduleID: schedule.ID,
		TargetID:   schedule.TargetID,
		StorageID:  schedule.StorageID,
		Type:       schedule.BackupType,
		DueAt:      dueAt,
		QueuedAt:   queuedAt,
	}
}
