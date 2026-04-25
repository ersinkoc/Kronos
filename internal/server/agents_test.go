package server

import (
	"testing"
	"time"
)

func TestAgentRegistryHeartbeatListAndStatus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	registry := NewAgentRegistry(func() time.Time { return now }, 30*time.Second)
	registry.Heartbeat(AgentHeartbeat{
		ID:       "agent-b",
		Version:  "dev",
		Capacity: 2,
		Labels:   map[string]string{"zone": "b"},
	})
	registry.Heartbeat(AgentHeartbeat{
		ID:  "agent-a",
		Now: now.Add(-time.Minute),
	})

	agents := registry.List()
	if len(agents) != 2 {
		t.Fatalf("len(agents) = %d, want 2", len(agents))
	}
	if agents[0].ID != "agent-a" || agents[0].Status != AgentDegraded {
		t.Fatalf("agents[0] = %#v, want degraded agent-a", agents[0])
	}
	if agents[1].ID != "agent-b" || agents[1].Status != AgentHealthy || agents[1].Capacity != 2 {
		t.Fatalf("agents[1] = %#v, want healthy agent-b", agents[1])
	}
	agents[1].Labels["zone"] = "mutated"
	again := registry.List()
	if again[1].Labels["zone"] != "b" {
		t.Fatalf("registry labels mutated through snapshot: %#v", again[1].Labels)
	}

	agent, ok := registry.Get("agent-b")
	if !ok || agent.ID != "agent-b" || agent.Status != AgentHealthy {
		t.Fatalf("Get(agent-b) = %#v ok=%v, want healthy agent-b", agent, ok)
	}
	agent.Labels["zone"] = "mutated"
	agent, _ = registry.Get("agent-b")
	if agent.Labels["zone"] != "b" {
		t.Fatalf("registry labels mutated through get snapshot: %#v", agent.Labels)
	}
}

func TestAgentRegistryPrune(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	registry := NewAgentRegistry(func() time.Time { return now }, 30*time.Second)
	registry.Heartbeat(AgentHeartbeat{ID: "fresh", Now: now})
	registry.Heartbeat(AgentHeartbeat{ID: "stale", Now: now.Add(-10 * time.Minute)})

	if removed := registry.Prune(5 * time.Minute); removed != 1 {
		t.Fatalf("Prune() = %d, want 1", removed)
	}
	agents := registry.List()
	if len(agents) != 1 || agents[0].ID != "fresh" {
		t.Fatalf("agents = %#v, want only fresh", agents)
	}
}
