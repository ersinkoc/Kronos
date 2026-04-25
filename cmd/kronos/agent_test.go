package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	control "github.com/kronos/kronos/internal/server"
)

func TestHeartbeatEndpoint(t *testing.T) {
	t.Parallel()

	got, err := heartbeatEndpoint("127.0.0.1:8500")
	if err != nil {
		t.Fatalf("heartbeatEndpoint() error = %v", err)
	}
	if got != "http://127.0.0.1:8500/api/v1/agents/heartbeat" {
		t.Fatalf("endpoint = %q", got)
	}
	got, err = heartbeatEndpoint("https://example.com/base")
	if err != nil {
		t.Fatalf("heartbeatEndpoint(https) error = %v", err)
	}
	if got != "https://example.com/base/api/v1/agents/heartbeat" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestOSHostname(t *testing.T) {
	t.Parallel()

	host, err := osHostname()
	if err != nil {
		t.Fatalf("osHostname() error = %v", err)
	}
	if strings.TrimSpace(host) == "" {
		t.Fatal("osHostname() returned empty host")
	}
}

func TestRunAgentHeartbeatPostsUntilCanceled(t *testing.T) {
	t.Parallel()

	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/agents/heartbeat" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runAgentHeartbeat(ctx, server.Client(), server.URL, control.AgentHeartbeat{ID: "agent-1"}, time.Millisecond)
	}()
	for count.Load() < 2 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgentHeartbeat() error = %v, want context.Canceled", err)
	}
}

func TestRunAgentHeartbeatRejectsServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := runAgentHeartbeat(context.Background(), server.Client(), server.URL, control.AgentHeartbeat{ID: "agent-1"}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("runAgentHeartbeat() error = %v, want 503", err)
	}
}

func TestRunAgentHeartbeatSendsBearerToken(t *testing.T) {
	t.Parallel()

	var gotAuth atomic.Value
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		cancel()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := runAgentHeartbeatWithToken(ctx, server.Client(), server.URL, control.AgentHeartbeat{ID: "agent-1"}, time.Second, "agent-secret")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgentHeartbeatWithToken() error = %v, want context.Canceled", err)
	}
	if got, _ := gotAuth.Load().(string); got != "Bearer agent-secret" {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
}

func TestRunAgentSendsCapacity(t *testing.T) {
	t.Parallel()

	var heartbeat control.AgentHeartbeat
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body bytes.Buffer
		if _, err := body.ReadFrom(r.Body); err != nil {
			t.Fatalf("ReadFrom(heartbeat) error = %v", err)
		}
		if err := json.Unmarshal(body.Bytes(), &heartbeat); err != nil {
			t.Fatalf("Unmarshal(heartbeat) error = %v", err)
		}
		cancel()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := runAgent(ctx, io.Discard, []string{"--server", server.URL, "--id", "agent-1", "--capacity", "3", "--heartbeat-interval", "1s"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgent() error = %v, want context.Canceled", err)
	}
	if heartbeat.ID != "agent-1" || heartbeat.Capacity != 3 {
		t.Fatalf("heartbeat = %#v", heartbeat)
	}
}

func TestRunAgentListAndInspect(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agents":
			if r.Method != http.MethodGet {
				t.Fatalf("agent list method = %s", r.Method)
			}
			fmt.Fprint(w, `{"agents":[{"id":"agent-1","status":"healthy"}]}`)
		case "/api/v1/agents/agent-1":
			if r.Method != http.MethodGet {
				t.Fatalf("agent inspect method = %s", r.Method)
			}
			fmt.Fprint(w, `{"id":"agent-1","status":"healthy","capacity":2}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"agent", "list", "--server", server.URL}); err != nil {
		t.Fatalf("agent list error = %v", err)
	}
	if !strings.Contains(out.String(), `"agents":[{"id":"agent-1"`) {
		t.Fatalf("agent list output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"agent", "inspect", "--server", server.URL, "--id", "agent-1"}); err != nil {
		t.Fatalf("agent inspect error = %v", err)
	}
	if !strings.Contains(out.String(), `"capacity":2`) {
		t.Fatalf("agent inspect output = %q", out.String())
	}
}

func TestRunAgentInspectRequiresID(t *testing.T) {
	t.Parallel()

	if err := run(context.Background(), io.Discard, []string{"agent", "inspect"}); err == nil {
		t.Fatal("agent inspect without id error = nil, want error")
	}
}

func TestRunAgentUsesGlobalServer(t *testing.T) {
	t.Parallel()

	var heartbeat control.AgentHeartbeat
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
			t.Fatalf("Decode(heartbeat) error = %v", err)
		}
		cancel()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := run(ctx, io.Discard, []string{"--server", server.URL, "agent", "--id", "agent-global", "--heartbeat-interval", "1s"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("run(agent) error = %v, want context.Canceled", err)
	}
	if heartbeat.ID != "agent-global" {
		t.Fatalf("heartbeat = %#v", heartbeat)
	}
}

func TestRunAgentWorkerSyncsAndClaims(t *testing.T) {
	t.Parallel()

	_, privateKey, err := ed25519.GenerateKey(bytes.NewReader(bytes.Repeat([]byte{7}, 64)))
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	chunkKey := hex.EncodeToString(bytes.Repeat([]byte{9}, 32))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var gotAuth atomic.Value
	var gotAgent atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path == "/api/v1/jobs/claim" {
			gotAgent.Store(r.Header.Get("X-Kronos-Agent-ID"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agents/heartbeat":
			fmt.Fprint(w, `{"id":"agent-1","status":"online","capacity":2}`)
		case "/api/v1/targets":
			fmt.Fprint(w, `{"targets":[]}`)
		case "/api/v1/storages":
			fmt.Fprint(w, `{"storages":[]}`)
		case "/api/v1/backups":
			fmt.Fprint(w, `{"backups":[]}`)
		case "/api/v1/jobs/claim":
			cancel()
			fmt.Fprint(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err = runAgentWorkerWithToken(ctx, server.Client(), server.URL, control.AgentHeartbeat{ID: "agent-1", Capacity: 2}, time.Millisecond, "agent-secret", agentWorkerOptions{
		ManifestPrivateKeyHex: hex.EncodeToString(privateKey),
		ChunkKeyHex:           chunkKey,
		ChunkAlgorithm:        "aes-256-gcm",
		Compression:           "none",
		KeyID:                 "k1",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runAgentWorkerWithToken() error = %v, want context.Canceled", err)
	}
	if got, _ := gotAuth.Load().(string); got != "Bearer agent-secret" {
		t.Fatalf("Authorization = %q, want bearer token", got)
	}
	if got, _ := gotAgent.Load().(string); got != "agent-1" {
		t.Fatalf("X-Kronos-Agent-ID = %q, want agent-1", got)
	}
}
