package server

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

var (
	jobsBucket                 = []byte("jobs")
	evidenceArtifactsBucket    = []byte("evidence_artifacts")
	evidenceArtifactsJobBucket = []byte("evidence_artifacts_by_job")
)

// JobStore persists server job state in Kronos' embedded KV store.
type JobStore struct {
	db *kvstore.DB
}

// EvidenceStore persists exportable evidence artifacts independently from jobs.
type EvidenceStore struct {
	db *kvstore.DB
}

// NewJobStore returns a job store backed by db.
func NewJobStore(db *kvstore.DB) (*JobStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &JobStore{db: db}, nil
}

// NewEvidenceStore returns an evidence artifact store backed by db.
func NewEvidenceStore(db *kvstore.DB) (*EvidenceStore, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	return &EvidenceStore{db: db}, nil
}

// Save inserts or replaces one job.
func (s *JobStore) Save(job core.Job) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("job store is closed")
	}
	if job.ID.IsZero() {
		return fmt.Errorf("job id is required")
	}
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(jobsBucket)
		if err != nil {
			return err
		}
		return bucket.Put([]byte(job.ID), data)
	})
}

// Get fetches one job by ID.
func (s *JobStore) Get(id core.ID) (core.Job, bool, error) {
	if s == nil || s.db == nil {
		return core.Job{}, false, fmt.Errorf("job store is closed")
	}
	if id.IsZero() {
		return core.Job{}, false, fmt.Errorf("job id is required")
	}
	var job core.Job
	var ok bool
	err := s.db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(jobsBucket)
		if err != nil {
			return err
		}
		data, exists, err := bucket.Get([]byte(id))
		if err != nil || !exists {
			ok = exists
			return err
		}
		ok = true
		return json.Unmarshal(data, &job)
	})
	return job, ok, err
}

// Delete removes one job by ID.
func (s *JobStore) Delete(id core.ID) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("job store is closed")
	}
	if id.IsZero() {
		return fmt.Errorf("job id is required")
	}
	return s.db.Update(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(jobsBucket)
		if err != nil {
			return err
		}
		return bucket.Delete([]byte(id))
	})
}

// List returns all jobs ordered by queued time, then ID.
func (s *JobStore) List() ([]core.Job, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("job store is closed")
	}
	var jobs []core.Job
	err := s.db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(jobsBucket)
		if err != nil {
			return err
		}
		it, err := bucket.Scan([]byte{1}, nil)
		if err != nil {
			return err
		}
		for it.Valid() {
			var job core.Job
			if err := json.Unmarshal(it.Value(), &job); err != nil {
				return err
			}
			jobs = append(jobs, job)
			it.Next()
		}
		return it.Err()
	})
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].QueuedAt.Equal(jobs[j].QueuedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].QueuedAt.Before(jobs[j].QueuedAt)
	})
	return jobs, err
}

// Save inserts or replaces one evidence artifact and indexes it by job ID.
func (s *EvidenceStore) Save(artifact core.EvidenceArtifact) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("evidence store is closed")
	}
	if artifact.ID.IsZero() {
		return fmt.Errorf("evidence artifact id is required")
	}
	if artifact.JobID.IsZero() {
		return fmt.Errorf("evidence artifact job id is required")
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *kvstore.Tx) error {
		artifacts, err := tx.Bucket(evidenceArtifactsBucket)
		if err != nil {
			return err
		}
		byJob, err := tx.Bucket(evidenceArtifactsJobBucket)
		if err != nil {
			return err
		}
		if err := artifacts.Put([]byte(artifact.ID), data); err != nil {
			return err
		}
		return byJob.Put([]byte(artifact.JobID), []byte(artifact.ID))
	})
}

// Get fetches one evidence artifact by artifact ID.
func (s *EvidenceStore) Get(id core.ID) (core.EvidenceArtifact, bool, error) {
	if s == nil || s.db == nil {
		return core.EvidenceArtifact{}, false, fmt.Errorf("evidence store is closed")
	}
	if id.IsZero() {
		return core.EvidenceArtifact{}, false, fmt.Errorf("evidence artifact id is required")
	}
	var artifact core.EvidenceArtifact
	var ok bool
	err := s.db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(evidenceArtifactsBucket)
		if err != nil {
			return err
		}
		data, exists, err := bucket.Get([]byte(id))
		if err != nil || !exists {
			ok = exists
			return err
		}
		ok = true
		return json.Unmarshal(data, &artifact)
	})
	return artifact, ok, err
}

// GetByJobID fetches the latest evidence artifact associated with a job ID.
func (s *EvidenceStore) GetByJobID(jobID core.ID) (core.EvidenceArtifact, bool, error) {
	if s == nil || s.db == nil {
		return core.EvidenceArtifact{}, false, fmt.Errorf("evidence store is closed")
	}
	if jobID.IsZero() {
		return core.EvidenceArtifact{}, false, fmt.Errorf("job id is required")
	}
	var artifactID core.ID
	var ok bool
	err := s.db.View(func(tx *kvstore.Tx) error {
		byJob, err := tx.Bucket(evidenceArtifactsJobBucket)
		if err != nil {
			return err
		}
		data, exists, err := byJob.Get([]byte(jobID))
		if err != nil || !exists {
			ok = exists
			return err
		}
		ok = true
		artifactID = core.ID(data)
		return nil
	})
	if err != nil || !ok {
		return core.EvidenceArtifact{}, ok, err
	}
	return s.Get(artifactID)
}

// FailActive marks jobs that were active during a server loss as failed.
func (s *JobStore) FailActive(now time.Time, reason string) (int, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if reason == "" {
		reason = "server_lost"
	}
	jobs, err := s.List()
	if err != nil {
		return 0, err
	}
	changed := 0
	for _, job := range jobs {
		if job.Status != core.JobStatusRunning && job.Status != core.JobStatusFinalizing {
			continue
		}
		job.Status = core.JobStatusFailed
		job.EndedAt = now
		job.Error = reason
		if err := s.Save(job); err != nil {
			return changed, err
		}
		changed++
	}
	return changed, nil
}
