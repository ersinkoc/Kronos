package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
)

func TestRunRetentionPlan(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	input := retentionPlanInput{
		Now: now,
		Policy: core.RetentionPolicy{Rules: []core.RetentionRule{{
			Kind:   "count",
			Params: map[string]any{"n": 1},
		}}},
		Backups: []core.Backup{
			{ID: "old", Type: core.BackupTypeFull, EndedAt: now.Add(-2 * time.Hour), SizeBytes: 1},
			{ID: "new", Type: core.BackupTypeFull, EndedAt: now.Add(-time.Hour), SizeBytes: 1},
		},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "retention.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "plan", "--input", path}); err != nil {
		t.Fatalf("retention plan error = %v", err)
	}
	var result retentionPlanOutput
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal(retention plan) error = %v output=%q", err, out.String())
	}
	if len(result.Items) != 2 || result.Items[0].ID != "new" || !result.Items[0].Keep || result.Items[1].ID != "old" || result.Items[1].Keep {
		t.Fatalf("retention plan output = %#v", result)
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"--output", "yaml", "retention", "plan", "--input", path}); err != nil {
		t.Fatalf("retention plan yaml error = %v", err)
	}
	text := out.String()
	if !strings.Contains(text, "items:") || !strings.Contains(text, "id: new") || !strings.Contains(text, "keep: true") {
		t.Fatalf("yaml output = %s", text)
	}
}

func TestRunRetentionPlanRejectsUnknownSubcommand(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "missing"}); err == nil {
		t.Fatal("retention missing succeeded, want error")
	}
}

func TestRunRetentionApply(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/retention/apply" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		defer r.Body.Close()
		var body bytes.Buffer
		if _, err := body.ReadFrom(r.Body); err != nil {
			t.Fatalf("ReadFrom(request) error = %v", err)
		}
		text := body.String()
		if !strings.Contains(text, `"kind":"count"`) || !strings.Contains(text, `"dry_run":true`) {
			t.Fatalf("retention apply request = %q", text)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"deleted":["old"],"dry_run":true}`)
	}))
	defer server.Close()

	input := retentionServerRequest{Policy: core.RetentionPolicy{Rules: []core.RetentionRule{{Kind: "count", Params: map[string]any{"n": 1}}}}}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "retention.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "apply", "--server", server.URL, "--input", path, "--dry-run"}); err != nil {
		t.Fatalf("retention apply error = %v", err)
	}
	if !strings.Contains(out.String(), `"deleted":["old"]`) || !strings.Contains(out.String(), `"dry_run":true`) {
		t.Fatalf("retention apply output = %q", out.String())
	}
}

func TestRunRetentionApplyPreservesDryRunFromInput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var body bytes.Buffer
		if _, err := body.ReadFrom(r.Body); err != nil {
			t.Fatalf("ReadFrom(request) error = %v", err)
		}
		if !strings.Contains(body.String(), `"dry_run":true`) {
			t.Fatalf("retention apply request = %q, want dry_run true", body.String())
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"deleted":[],"dry_run":true}`)
	}))
	defer server.Close()

	input := retentionServerRequest{
		DryRun: true,
		Policy: core.RetentionPolicy{Rules: []core.RetentionRule{{
			Kind:   "count",
			Params: map[string]any{"n": 1},
		}}},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "retention.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "apply", "--server", server.URL, "--input", path}); err != nil {
		t.Fatalf("retention apply error = %v", err)
	}
}

func TestRunRetentionApplyRequiresInput(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "apply"}); err == nil {
		t.Fatal("retention apply without input error = nil, want error")
	}
}

func TestRunRetentionPolicyAddListRemove(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/retention/policies":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"policies":[{"id":"policy-1","name":"daily"}]}`)
			case http.MethodPost:
				defer r.Body.Close()
				var body bytes.Buffer
				if _, err := body.ReadFrom(r.Body); err != nil {
					t.Fatalf("ReadFrom(request) error = %v", err)
				}
				text := body.String()
				if !strings.Contains(text, `"id":"policy-1"`) || !strings.Contains(text, `"kind":"count"`) {
					t.Fatalf("retention policy add request = %q", text)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"policy-1","name":"daily"}`)
			default:
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
			}
		case "/api/v1/retention/policies/policy-1":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"policy-1","name":"daily","rules":[{"kind":"count","params":{"n":7}}]}`)
			case http.MethodPut:
				defer r.Body.Close()
				var body bytes.Buffer
				if _, err := body.ReadFrom(r.Body); err != nil {
					t.Fatalf("ReadFrom(update request) error = %v", err)
				}
				text := body.String()
				if !strings.Contains(text, `"name":"weekly"`) || !strings.Contains(text, `"kind":"time"`) {
					t.Fatalf("retention policy update request = %q", text)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"policy-1","name":"weekly","rules":[{"kind":"time","params":{"duration":"168h"}}]}`)
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("retention policy method = %s", r.Method)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	policy := core.RetentionPolicy{ID: "policy-1", Name: "daily", Rules: []core.RetentionRule{{Kind: "count", Params: map[string]any{"n": 7}}}}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "policy", "add", "--server", server.URL, "--input", path}); err != nil {
		t.Fatalf("retention policy add error = %v", err)
	}
	if !strings.Contains(out.String(), `"id":"policy-1"`) {
		t.Fatalf("retention policy add output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"retention", "policy", "list", "--server", server.URL}); err != nil {
		t.Fatalf("retention policy list error = %v", err)
	}
	if !strings.Contains(out.String(), `"policies":[{"id":"policy-1"`) {
		t.Fatalf("retention policy list output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"retention", "policy", "inspect", "--server", server.URL, "--id", "policy-1"}); err != nil {
		t.Fatalf("retention policy inspect error = %v", err)
	}
	if !strings.Contains(out.String(), `"rules":[{"kind":"count"`) {
		t.Fatalf("retention policy inspect output = %q", out.String())
	}
	out.Reset()
	updatedPolicy := core.RetentionPolicy{ID: "ignored", Name: "weekly", Rules: []core.RetentionRule{{Kind: "time", Params: map[string]any{"duration": "168h"}}}}
	updatedData, err := json.Marshal(updatedPolicy)
	if err != nil {
		t.Fatalf("Marshal(updatedPolicy) error = %v", err)
	}
	updatedPath := filepath.Join(t.TempDir(), "policy-updated.json")
	if err := os.WriteFile(updatedPath, updatedData, 0o600); err != nil {
		t.Fatalf("WriteFile(updatedPath) error = %v", err)
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "update", "--server", server.URL, "--id", "policy-1", "--input", updatedPath}); err != nil {
		t.Fatalf("retention policy update error = %v", err)
	}
	if !strings.Contains(out.String(), `"name":"weekly"`) {
		t.Fatalf("retention policy update output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"retention", "policy", "remove", "--server", server.URL, "--id", "policy-1"}); err != nil {
		t.Fatalf("retention policy remove error = %v", err)
	}
	if out.String() != "" {
		t.Fatalf("retention policy remove output = %q, want empty", out.String())
	}
}

func TestRunRetentionPolicyRequiresFields(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"retention", "policy"}); err == nil {
		t.Fatal("retention policy without subcommand error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "add"}); err == nil {
		t.Fatal("retention policy add without input error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "inspect"}); err == nil {
		t.Fatal("retention policy inspect without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "remove"}); err == nil {
		t.Fatal("retention policy remove without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "update", "--input", "policy.json"}); err == nil {
		t.Fatal("retention policy update without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"retention", "policy", "update", "--id", "policy-1"}); err == nil {
		t.Fatal("retention policy update without input error = nil, want error")
	}
}
