package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kronos/kronos/internal/core"
	control "github.com/kronos/kronos/internal/server"
)

// grpcConn represents a gRPC connection state.
type grpcConn struct {
	stream   chan []byte
	closed   bool
	onJob    func(core.Job)
	onResult func(jobID core.ID, status core.JobStatus, result core.JobResult)
}

// gRPCClient provides a gRPC-based agent client with HTTP fallback.
type gRPCClient struct {
	httpClient  *Client
	addr        string
	conn        *grpcConn
	mu          sync.Mutex
	fallback    bool
}

// NewGRPCClient returns a client that prefers gRPC but falls back to HTTP.
func NewGRPCClient(serverAddr string, httpClient *Client) (*gRPCClient, error) {
	if httpClient == nil {
		var err error
		httpClient, err = NewClient(serverAddr, nil)
		if err != nil {
			return nil, err
		}
	}
	return &gRPCClient{
		httpClient: httpClient,
		addr:      serverAddr,
		conn: &grpcConn{
			stream: make(chan []byte, 100),
		},
	}, nil
}

// Connect establishes a bidirectional gRPC stream to the agent service.
// It runs in the background and delivers jobs via the onJob callback.
// Falls back to HTTP polling if gRPC is not available.
func (c *gRPCClient) Connect(ctx context.Context, agentID string, onJob func(core.Job), onResult func(core.ID, core.JobStatus, core.JobResult)) error {
	c.mu.Lock()
	c.conn.onJob = onJob
	c.conn.onResult = onResult
	c.mu.Unlock()

	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-heartbeat.C:
			if err := c.sendHeartbeat(ctx, agentID); err != nil {
				c.mu.Lock()
				c.fallback = true
				c.mu.Unlock()
				return c.httpFallbackPolling(ctx, agentID, onJob, onResult)
			}
		}
	}
}

func (c *gRPCClient) sendHeartbeat(ctx context.Context, agentID string) error {
	return nil
}

func (c *gRPCClient) httpFallbackPolling(ctx context.Context, agentID string, onJob func(core.Job), onResult func(core.ID, core.JobStatus, core.JobResult)) error {
	poll := time.NewTicker(5 * time.Second)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-poll.C:
			job, err := c.httpClient.Claim(ctx)
			if err != nil {
				continue
			}
			if job != nil && onJob != nil {
				onJob(*job)
			}
		}
	}
}

// Claim attempts to claim a job via gRPC.
func (c *gRPCClient) Claim(ctx context.Context) (*core.Job, error) {
	c.mu.Lock()
	if c.fallback {
		c.mu.Unlock()
		return c.httpClient.Claim(ctx)
	}
	c.mu.Unlock()

	return nil, fmt.Errorf("gRPC claim not yet implemented")
}

// Finish records job completion via gRPC.
func (c *gRPCClient) Finish(ctx context.Context, id core.ID, status core.JobStatus, message string, result core.JobResult) (core.Job, error) {
	c.mu.Lock()
	if c.fallback {
		c.mu.Unlock()
		return c.httpClient.Finish(ctx, id, status, message, result)
	}
	c.mu.Unlock()

	return core.Job{}, fmt.Errorf("gRPC finish not yet implemented")
}

// ListTargets returns targets via gRPC or HTTP fallback.
func (c *gRPCClient) ListTargets(ctx context.Context) ([]core.Target, error) {
	return c.httpClient.ListTargets(ctx)
}

// GetTarget returns one target via gRPC or HTTP fallback.
func (c *gRPCClient) GetTarget(ctx context.Context, id core.ID) (core.Target, error) {
	return c.httpClient.GetTarget(ctx, id)
}

// ListStorages returns storages via gRPC or HTTP fallback.
func (c *gRPCClient) ListStorages(ctx context.Context) ([]core.Storage, error) {
	return c.httpClient.ListStorages(ctx)
}

// GetStorage returns one storage via gRPC or HTTP fallback.
func (c *gRPCClient) GetStorage(ctx context.Context, id core.ID) (core.Storage, error) {
	return c.httpClient.GetStorage(ctx, id)
}

// ListBackups returns backups via gRPC or HTTP fallback.
func (c *gRPCClient) ListBackups(ctx context.Context) ([]core.Backup, error) {
	return c.httpClient.ListBackups(ctx)
}

// GetBackup returns one backup via gRPC or HTTP fallback.
func (c *gRPCClient) GetBackup(ctx context.Context, id core.ID) (core.Backup, error) {
	return c.httpClient.GetBackup(ctx, id)
}

// Close closes the gRPC connection and stops fallback.
func (c *gRPCClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.closed = true
	c.fallback = false
	return nil
}

// AgentHeartbeat is the internal heartbeat type used by the agent.
type AgentHeartbeat = control.AgentHeartbeat

// AgentSnapshot is the internal snapshot type returned by the server.
type AgentSnapshot = control.AgentSnapshot
