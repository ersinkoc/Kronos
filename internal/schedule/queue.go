package schedule

import "github.com/kronos/kronos/internal/core"

// TargetQueue serializes jobs per target while allowing different targets to run.
type TargetQueue struct {
	running map[core.ID]bool
	queued  map[core.ID][]DueJob
}

// NewTargetQueue returns an empty per-target queue.
func NewTargetQueue() *TargetQueue {
	return &TargetQueue{
		running: make(map[core.ID]bool),
		queued:  make(map[core.ID][]DueJob),
	}
}

// Enqueue records jobs and returns the jobs that can start immediately.
func (q *TargetQueue) Enqueue(jobs ...DueJob) []DueJob {
	if q.running == nil {
		q.running = make(map[core.ID]bool)
	}
	if q.queued == nil {
		q.queued = make(map[core.ID][]DueJob)
	}
	ready := make([]DueJob, 0, len(jobs))
	for _, job := range jobs {
		if !q.running[job.TargetID] {
			q.running[job.TargetID] = true
			ready = append(ready, job)
			continue
		}
		q.queued[job.TargetID] = append(q.queued[job.TargetID], job)
	}
	return ready
}

// Complete marks one target's running job done and returns the next queued job.
func (q *TargetQueue) Complete(targetID core.ID) (DueJob, bool) {
	if q.running == nil {
		return DueJob{}, false
	}
	pending := q.queued[targetID]
	if len(pending) == 0 {
		delete(q.running, targetID)
		delete(q.queued, targetID)
		return DueJob{}, false
	}
	next := pending[0]
	if len(pending) == 1 {
		delete(q.queued, targetID)
	} else {
		q.queued[targetID] = pending[1:]
	}
	q.running[targetID] = true
	return next, true
}

// Pending reports the number of queued jobs waiting behind the active job.
func (q *TargetQueue) Pending(targetID core.ID) int {
	return len(q.queued[targetID])
}
