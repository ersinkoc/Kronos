package server

import (
	"sort"
	"sync"
	"time"
)

// AgentStatus describes current server-side agent liveness.
type AgentStatus string

const (
	// AgentHealthy means the agent heartbeat is fresh.
	AgentHealthy AgentStatus = "healthy"
	// AgentDegraded means the agent missed the healthy heartbeat window.
	AgentDegraded AgentStatus = "degraded"
)

// AgentHeartbeat is sent by an agent to announce liveness and capacity.
type AgentHeartbeat struct {
	ID       string            `json:"id"`
	Version  string            `json:"version,omitempty"`
	Address  string            `json:"address,omitempty"`
	Capacity int               `json:"capacity,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Now      time.Time         `json:"now,omitempty"`
}

// AgentSnapshot is returned to API callers.
type AgentSnapshot struct {
	ID            string            `json:"id"`
	Version       string            `json:"version,omitempty"`
	Address       string            `json:"address,omitempty"`
	Capacity      int               `json:"capacity,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	Status        AgentStatus       `json:"status"`
}

// AgentRegistry stores the latest heartbeat from each agent.
type AgentRegistry struct {
	mu            sync.RWMutex
	now           func() time.Time
	degradedAfter time.Duration
	agents        map[string]AgentSnapshot
}

// NewAgentRegistry returns an empty agent registry.
func NewAgentRegistry(now func() time.Time, degradedAfter time.Duration) *AgentRegistry {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if degradedAfter <= 0 {
		degradedAfter = 30 * time.Second
	}
	return &AgentRegistry{
		now:           now,
		degradedAfter: degradedAfter,
		agents:        make(map[string]AgentSnapshot),
	}
}

// Heartbeat records one agent heartbeat and returns the fresh snapshot.
func (r *AgentRegistry) Heartbeat(h AgentHeartbeat) AgentSnapshot {
	if r.agents == nil {
		r.agents = make(map[string]AgentSnapshot)
	}
	now := h.Now
	if now.IsZero() {
		now = r.now()
	}
	snapshot := AgentSnapshot{
		ID:            h.ID,
		Version:       h.Version,
		Address:       h.Address,
		Capacity:      h.Capacity,
		Labels:        cloneLabels(h.Labels),
		LastHeartbeat: now.UTC(),
		Status:        AgentHealthy,
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[h.ID] = snapshot
	return snapshot
}

// List returns known agents ordered by ID with computed status.
func (r *AgentRegistry) List() []AgentSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now()
	agents := make([]AgentSnapshot, 0, len(r.agents))
	for _, agent := range r.agents {
		agent.Status = r.statusAt(agent.LastHeartbeat, now)
		agent.Labels = cloneLabels(agent.Labels)
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})
	return agents
}

// Get returns one known agent snapshot with computed status.
func (r *AgentRegistry) Get(id string) (AgentSnapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agents[id]
	if !ok {
		return AgentSnapshot{}, false
	}
	agent.Status = r.statusAt(agent.LastHeartbeat, r.now())
	agent.Labels = cloneLabels(agent.Labels)
	return agent, true
}

// Prune removes agents older than maxAge and returns the number removed.
func (r *AgentRegistry) Prune(maxAge time.Duration) int {
	if maxAge <= 0 {
		return 0
	}
	cutoff := r.now().Add(-maxAge)
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for id, agent := range r.agents {
		if agent.LastHeartbeat.Before(cutoff) {
			delete(r.agents, id)
			removed++
		}
	}
	return removed
}

func (r *AgentRegistry) statusAt(lastHeartbeat time.Time, now time.Time) AgentStatus {
	if now.Sub(lastHeartbeat) > r.degradedAfter {
		return AgentDegraded
	}
	return AgentHealthy
}

func cloneLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}
