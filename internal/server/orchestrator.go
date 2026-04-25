package server

import (
	"fmt"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/schedule"
)

// Orchestrator persists due work and prepares it for agent dispatch.
type Orchestrator struct {
	store *JobStore
	clock core.Clock
}

// NewOrchestrator returns an orchestrator backed by store.
func NewOrchestrator(store *JobStore, clock core.Clock) (*Orchestrator, error) {
	if store == nil {
		return nil, fmt.Errorf("job store is required")
	}
	if clock == nil {
		clock = core.RealClock{}
	}
	return &Orchestrator{store: store, clock: clock}, nil
}

// EnqueueDue persists scheduler output as queued jobs.
func (o *Orchestrator) EnqueueDue(due []schedule.DueJob) ([]core.Job, error) {
	if o == nil || o.store == nil {
		return nil, fmt.Errorf("orchestrator is closed")
	}
	jobs := make([]core.Job, 0, len(due))
	for _, item := range due {
		id, err := core.NewID(o.clock)
		if err != nil {
			return nil, err
		}
		queuedAt := item.QueuedAt
		if queuedAt.IsZero() {
			queuedAt = o.clock.Now().UTC()
		}
		job := core.Job{
			ID:             id,
			Operation:      core.JobOperationBackup,
			ScheduleID:     item.ScheduleID,
			TargetID:       item.TargetID,
			StorageID:      item.StorageID,
			Type:           item.Type,
			ParentBackupID: item.ParentBackupID,
			Status:         core.JobStatusQueued,
			QueuedAt:       queuedAt,
		}
		if err := o.store.Save(job); err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// Start marks a queued job as running.
func (o *Orchestrator) Start(id core.ID) (core.Job, error) {
	return o.StartOnAgent(id, "")
}

// StartOnAgent marks a queued job as running and records the claiming agent.
func (o *Orchestrator) StartOnAgent(id core.ID, agentID string) (core.Job, error) {
	job, ok, err := o.store.Get(id)
	if err != nil {
		return core.Job{}, err
	}
	if !ok {
		return core.Job{}, core.WrapKind(core.ErrorKindNotFound, "start job", fmt.Errorf("job %q not found", id))
	}
	if job.Status != core.JobStatusQueued {
		return core.Job{}, fmt.Errorf("job %q is %s, want queued", id, job.Status)
	}
	job.Status = core.JobStatusRunning
	job.StartedAt = o.clock.Now().UTC()
	job.AgentID = agentID
	if err := o.store.Save(job); err != nil {
		return core.Job{}, err
	}
	return job, nil
}

// Finish records a terminal job state.
func (o *Orchestrator) Finish(id core.ID, status core.JobStatus, message string) (core.Job, error) {
	if status != core.JobStatusSucceeded && status != core.JobStatusFailed && status != core.JobStatusCanceled {
		return core.Job{}, fmt.Errorf("status %s is not terminal", status)
	}
	job, ok, err := o.store.Get(id)
	if err != nil {
		return core.Job{}, err
	}
	if !ok {
		return core.Job{}, core.WrapKind(core.ErrorKindNotFound, "finish job", fmt.Errorf("job %q not found", id))
	}
	if terminalJobStatus(job.Status) {
		return core.Job{}, fmt.Errorf("job %q is already %s", id, job.Status)
	}
	job.Status = status
	job.EndedAt = o.clock.Now().UTC()
	job.Error = message
	if err := o.store.Save(job); err != nil {
		return core.Job{}, err
	}
	return job, nil
}

// Retry returns a failed or canceled job to the queue with its original payload.
func (o *Orchestrator) Retry(id core.ID) (core.Job, error) {
	job, ok, err := o.store.Get(id)
	if err != nil {
		return core.Job{}, err
	}
	if !ok {
		return core.Job{}, core.WrapKind(core.ErrorKindNotFound, "retry job", fmt.Errorf("job %q not found", id))
	}
	if job.Status != core.JobStatusFailed && job.Status != core.JobStatusCanceled {
		return core.Job{}, fmt.Errorf("job %q is %s, want failed or canceled", id, job.Status)
	}
	job.Status = core.JobStatusQueued
	job.QueuedAt = o.clock.Now().UTC()
	job.StartedAt = time.Time{}
	job.EndedAt = time.Time{}
	job.Error = ""
	job.AgentID = ""
	if err := o.store.Save(job); err != nil {
		return core.Job{}, err
	}
	return job, nil
}

func terminalJobStatus(status core.JobStatus) bool {
	return status == core.JobStatusSucceeded || status == core.JobStatusFailed || status == core.JobStatusCanceled
}
