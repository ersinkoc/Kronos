package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kronos/kronos/internal/obs"
)

func TestRunOverview(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/overview" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer overview-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get(obs.RequestIDHeader) != "req-overview-1" {
			t.Fatalf("%s = %q", obs.RequestIDHeader, r.Header.Get(obs.RequestIDHeader))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"agents":{"healthy":1},"jobs":{"active":2}}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"--server", server.URL, "--token", "overview-token", "--request-id", "req-overview-1", "overview"}); err != nil {
		t.Fatalf("overview error = %v", err)
	}
	if !strings.Contains(out.String(), `"healthy":1`) || !strings.Contains(out.String(), `"active":2`) {
		t.Fatalf("overview output = %q", out.String())
	}
}

func TestRunOverviewTable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/overview" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"generated_at":"2026-04-26T12:00:00Z",
			"agents":{"healthy":2,"degraded":1,"capacity":4},
			"inventory":{"targets":3,"storages":2,"schedules":5,"schedules_paused":1,"retention_policies":1,"notification_rules":2,"notification_rules_enabled":1,"users":4},
			"jobs":{"active":2,"by_status":{"failed":1,"running":2}},
			"backups":{"total":9,"protected":4,"bytes_total":2048,"by_type":{"full":3}},
			"health":{"status":"ok","checks":{"jobs":"ok"}},
			"attention":{"degraded_agents":1,"failed_jobs":1,"readiness_errors":0,"unprotected_backups":5,"disabled_notification_rules":1}
		}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"--server", server.URL, "--output", "table", "overview"}); err != nil {
		t.Fatalf("overview table error = %v", err)
	}
	text := out.String()
	for _, want := range []string{"METRIC", "VALUE", "health", "ok", "agents.healthy", "2", "attention.unprotected_backups", "5", "inventory.notification_rules_enabled", "1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("overview table missing %q in %q", want, text)
		}
	}
}

func TestRunOverviewRejectsArgs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"overview", "extra"}); err == nil {
		t.Fatal("overview extra arg error = nil, want error")
	}
}
