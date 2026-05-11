package server

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/kronos/kronos/internal/core"
)

// GRPCServer handles gRPC agent connections via bidirectional streaming.
type GRPCServer struct {
	registry  *AgentRegistry
	store     *JobStore
	orch      *Orchestrator
	conns     map[string]*agentConn
	mu        sync.RWMutex
	streamTTL time.Duration
}

type agentConn struct {
	agentID   string
	stream    chan []byte
	jobs      chan core.Job
	ctx       context.Context
	cancel    func()
	connected time.Time
}

func NewGRPCServer(registry *AgentRegistry, store *JobStore, orch *Orchestrator) *GRPCServer {
	return &GRPCServer{
		registry:  registry,
		store:     store,
		orch:      orch,
		conns:     make(map[string]*agentConn),
		streamTTL: 30 * time.Second,
	}
}

func (s *GRPCServer) Connect(stream grpcServerStream) error {
	ctx := stream.Context()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if err := s.handleMessage(ctx, stream, msg); err != nil {
			if err := stream.Send(s.errorPayload("", "HANDLER_ERROR", err.Error())); err != nil {
				return err
			}
		}
	}
}

func (s *GRPCServer) handleMessage(ctx context.Context, stream grpcServerStream, msg []byte) error {
	return nil
}

func (s *GRPCServer) registerAgent(agentID string, conn *agentConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[agentID] = conn
}

func (s *GRPCServer) unregisterAgent(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, agentID)
}

func (s *GRPCServer) dispatchJob(agentID string, job core.Job) error {
	s.mu.RLock()
	conn, ok := s.conns[agentID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	select {
	case conn.jobs <- job:
		return nil
	default:
		return fmt.Errorf("agent %s job channel full", agentID)
	}
}

type grpcServerStream interface {
	Context() context.Context
	Send(msg []byte) error
	Recv() ([]byte, error)
}

func (s *GRPCServer) errorPayload(requestID, code, message string) []byte {
	return []byte(fmt.Sprintf(`{"request_id":"%s","error":{"code":"%s","message":"%s"}}`, requestID, code, message))
}

// Heartbeat handles unary gRPC heartbeat requests.
func (s *GRPCServer) Heartbeat(ctx context.Context, req HeartbeatRequest) (*HeartbeatResponse, error) {
	if req.Heartbeat == nil {
		return nil, fmt.Errorf("heartbeat is required")
	}

	now := time.Now()
	if req.Heartbeat.Timestamp > 0 {
		now = time.Unix(req.Heartbeat.Timestamp, 0)
	}

	heartbeat := AgentHeartbeat{
		ID:       req.Heartbeat.AgentID,
		Version:  req.Heartbeat.Version,
		Address:  req.Heartbeat.Address,
		Capacity: int(req.Heartbeat.Capacity),
		Labels:   req.Heartbeat.Labels,
		Now:      now,
	}

	s.registry.Heartbeat(heartbeat)

	return &HeartbeatResponse{
		Accepted:   true,
		ServerTime: time.Now().Unix(),
	}, nil
}

// ClaimJob handles unary gRPC job claim requests.
func (s *GRPCServer) ClaimJob(ctx context.Context, req ClaimJobRequest) (*ClaimJobResponse, error) {
	if req.AgentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}

	jobs, err := s.store.List()
	if err != nil {
		return nil, err
	}

	var queued []core.Job
	for _, job := range jobs {
		if job.Status == core.JobStatusQueued {
			queued = append(queued, job)
		}
	}

	if len(queued) == 0 {
		return &ClaimJobResponse{Claimed: false}, nil
	}

	job := queued[0]
	job, err = s.orch.StartOnAgent(job.ID, req.AgentID)
	if err != nil {
		return nil, err
	}

	return &ClaimJobResponse{
		Claimed: true,
		Job: &JobPayload{
			RequestID: string(job.ID),
			JobID:     string(job.ID),
			JobType:   string(job.Type),
			TargetID:  string(job.TargetID),
			StorageID: string(job.StorageID),
		},
	}, nil
}

// FinishJob handles unary gRPC job completion requests.
func (s *GRPCServer) FinishJob(ctx context.Context, req FinishJobRequest) (*FinishJobResponse, error) {
	if req.JobID == "" {
		return nil, fmt.Errorf("job_id is required")
	}

	status := core.JobStatus(req.Status)
	if status == "" {
		status = core.JobStatusSucceeded
	}

	_, err := s.orch.Finish(core.ID(req.JobID), status, req.ErrorMessage)
	if err != nil {
		return nil, err
	}

	return &FinishJobResponse{Accepted: true}, nil
}

type HeartbeatRequest struct {
	Heartbeat *HeartbeatPayload
}

type HeartbeatPayload struct {
	AgentID   string
	Version   string
	Address   string
	Capacity  int32
	Labels    map[string]string
	Timestamp int64
}

type HeartbeatResponse struct {
	Accepted   bool
	ServerTime int64
}

type ClaimJobRequest struct {
	AgentID string
}

type ClaimJobResponse struct {
	Claimed bool
	Job     *JobPayload
}

type JobPayload struct {
	RequestID string
	JobID     string
	JobType   string
	TargetID  string
	StorageID string
}

type FinishJobRequest struct {
	JobID         string
	Status        string
	ErrorMessage  string
	ResultPayload []byte
}

type FinishJobResponse struct {
	Accepted bool
}

type ListTargetsRequest struct {
	AgentID       string
	IncludeSecrets bool
}

type ListTargetsResponse struct {
	Targets []*TargetPayload
}

type TargetPayload struct {
	Id         string
	Name       string
	Driver     string
	Connection map[string]string
	Options    map[string]string
}

type ListStoragesRequest struct {
	AgentID       string
	IncludeSecrets bool
}

type ListStoragesResponse struct {
	Storages []*StoragePayload
}

type StoragePayload struct {
	Id       string
	Name     string
	Backend  string
	Config   map[string]string
}

type ListBackupsRequest struct {
	AgentID string
}

type ListBackupsResponse struct {
	Backups []*BackupPayload
}

type BackupPayload struct {
	Id        string
	TargetID  string
	StorageID string
	CreatedAt int64
}
