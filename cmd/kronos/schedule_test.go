package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunScheduleAddListRemoveTick(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/schedules":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"schedules":[{"id":"schedule-1","name":"nightly"}]}`)
			case http.MethodPost:
				defer r.Body.Close()
				var body bytes.Buffer
				if _, err := body.ReadFrom(r.Body); err != nil {
					t.Fatalf("ReadFrom(request) error = %v", err)
				}
				text := body.String()
				for _, want := range []string{`"id":"schedule-1"`, `"target_id":"target-1"`, `"storage_id":"storage-1"`, `"expression":"0 2 * * *"`} {
					if !strings.Contains(text, want) {
						t.Fatalf("schedule add request missing %s in %q", want, text)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"schedule-1","name":"nightly"}`)
			default:
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
			}
		case "/api/v1/schedules/schedule-1":
			switch r.Method {
			case http.MethodGet:
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"schedule-1","name":"nightly","backup_type":"full"}`)
			case http.MethodPut:
				defer r.Body.Close()
				var body bytes.Buffer
				if _, err := body.ReadFrom(r.Body); err != nil {
					t.Fatalf("ReadFrom(update request) error = %v", err)
				}
				text := body.String()
				for _, want := range []string{`"name":"hourly"`, `"target_id":"target-2"`, `"storage_id":"storage-2"`, `"backup_type":"incr"`, `"retention_policy_id":"policy-1"`, `"paused":true`} {
					if !strings.Contains(text, want) {
						t.Fatalf("schedule update request missing %s in %q", want, text)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"schedule-1","name":"hourly","backup_type":"incr","paused":true}`)
			case http.MethodDelete:
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("schedule method = %s", r.Method)
			}
		case "/api/v1/schedules/schedule-1/pause":
			if r.Method != http.MethodPost {
				t.Fatalf("schedule pause method = %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"schedule-1","paused":true}`)
		case "/api/v1/schedules/schedule-1/resume":
			if r.Method != http.MethodPost {
				t.Fatalf("schedule resume method = %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":"schedule-1","paused":false}`)
		case "/api/v1/scheduler/tick":
			if r.Method != http.MethodPost {
				t.Fatalf("schedule tick method = %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"jobs":[{"id":"job-1","status":"queued"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{
		"schedule", "add", "--server", server.URL, "--id", "schedule-1", "--name", "nightly", "--target", "target-1", "--storage", "storage-1", "--cron", "0 2 * * *",
	}); err != nil {
		t.Fatalf("schedule add error = %v", err)
	}
	if !strings.Contains(out.String(), `"id":"schedule-1"`) {
		t.Fatalf("schedule add output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "list", "--server", server.URL}); err != nil {
		t.Fatalf("schedule list error = %v", err)
	}
	if !strings.Contains(out.String(), `"schedules":[{"id":"schedule-1"`) {
		t.Fatalf("schedule list output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "inspect", "--server", server.URL, "--id", "schedule-1"}); err != nil {
		t.Fatalf("schedule inspect error = %v", err)
	}
	if !strings.Contains(out.String(), `"backup_type":"full"`) {
		t.Fatalf("schedule inspect output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "update", "--server", server.URL, "--id", "schedule-1", "--name", "hourly", "--target", "target-2", "--storage", "storage-2", "--type", "incr", "--cron", "0 * * * *", "--retention-policy", "policy-1", "--paused"}); err != nil {
		t.Fatalf("schedule update error = %v", err)
	}
	if !strings.Contains(out.String(), `"name":"hourly"`) || !strings.Contains(out.String(), `"paused":true`) {
		t.Fatalf("schedule update output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "pause", "--server", server.URL, "--id", "schedule-1"}); err != nil {
		t.Fatalf("schedule pause error = %v", err)
	}
	if !strings.Contains(out.String(), `"paused":true`) {
		t.Fatalf("schedule pause output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "resume", "--server", server.URL, "--id", "schedule-1"}); err != nil {
		t.Fatalf("schedule resume error = %v", err)
	}
	if !strings.Contains(out.String(), `"paused":false`) {
		t.Fatalf("schedule resume output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "tick", "--server", server.URL}); err != nil {
		t.Fatalf("schedule tick error = %v", err)
	}
	if !strings.Contains(out.String(), `"jobs":[{"id":"job-1"`) {
		t.Fatalf("schedule tick output = %q", out.String())
	}
	out.Reset()
	if err := run(context.Background(), &out, []string{"schedule", "remove", "--server", server.URL, "--id", "schedule-1"}); err != nil {
		t.Fatalf("schedule remove error = %v", err)
	}
	if out.String() != "" {
		t.Fatalf("schedule remove output = %q, want empty", out.String())
	}
}

func TestRunScheduleAddRequiresFields(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := run(context.Background(), &out, []string{"schedule", "add", "--target", "target-1", "--storage", "storage-1", "--cron", "0 2 * * *"}); err == nil {
		t.Fatal("schedule add without name error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"schedule", "add", "--name", "nightly", "--storage", "storage-1", "--cron", "0 2 * * *"}); err == nil {
		t.Fatal("schedule add without target error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"schedule", "remove"}); err == nil {
		t.Fatal("schedule remove without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"schedule", "inspect"}); err == nil {
		t.Fatal("schedule inspect without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"schedule", "update", "--name", "nightly", "--target", "target-1", "--storage", "storage-1", "--cron", "0 2 * * *"}); err == nil {
		t.Fatal("schedule update without id error = nil, want error")
	}
	if err := run(context.Background(), &out, []string{"schedule", "pause"}); err == nil {
		t.Fatal("schedule pause without id error = nil, want error")
	}
}
