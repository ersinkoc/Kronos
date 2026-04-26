package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type overviewCLIResponse struct {
	GeneratedAt string `json:"generated_at"`
	Agents      struct {
		Healthy  int `json:"healthy"`
		Degraded int `json:"degraded"`
		Capacity int `json:"capacity"`
	} `json:"agents"`
	Inventory map[string]int `json:"inventory"`
	Jobs      struct {
		Active   int            `json:"active"`
		ByStatus map[string]int `json:"by_status"`
	} `json:"jobs"`
	Backups struct {
		Total                    int            `json:"total"`
		Protected                int            `json:"protected"`
		BytesTotal               int64          `json:"bytes_total"`
		LatestCompletedTimestamp int64          `json:"latest_completed_timestamp"`
		ByType                   map[string]int `json:"by_type"`
	} `json:"backups"`
	Health struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
		Error  string            `json:"error"`
	} `json:"health"`
	Attention map[string]int `json:"attention"`
}

func runOverview(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("overview", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("overview does not accept positional arguments")
	}
	body, err := fetchOverview(ctx, http.DefaultClient, *serverAddr)
	if err != nil {
		return err
	}
	if controlOutput(ctx) != "table" {
		data, err := formatStructuredJSONBytes(ctx, body)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	}
	var overview overviewCLIResponse
	if err := json.Unmarshal(body, &overview); err != nil {
		return err
	}
	_, err = out.Write(formatOverviewTable(overview))
	return err
}

func fetchOverview(ctx context.Context, client *http.Client, serverAddr string) ([]byte, error) {
	server := controlServerAddr(ctx, serverAddr)
	endpoint, err := controlEndpoint(server, "/api/v1/overview")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	setControlAuth(ctx, req)
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body bytes.Buffer
	if _, err := body.ReadFrom(io.LimitReader(resp.Body, 16*1024*1024)); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed: %s%s: %s", req.Method, req.URL.Path, resp.Status, responseRequestID(resp), strings.TrimSpace(body.String()))
	}
	return body.Bytes(), nil
}

func formatOverviewTable(overview overviewCLIResponse) []byte {
	rows := [][]string{
		{"generated_at", overview.GeneratedAt},
		{"health", overview.Health.Status},
		{"agents.healthy", fmt.Sprint(overview.Agents.Healthy)},
		{"agents.degraded", fmt.Sprint(overview.Agents.Degraded)},
		{"agents.capacity", fmt.Sprint(overview.Agents.Capacity)},
		{"jobs.active", fmt.Sprint(overview.Jobs.Active)},
		{"jobs.failed", fmt.Sprint(overview.Jobs.ByStatus["failed"])},
		{"backups.total", fmt.Sprint(overview.Backups.Total)},
		{"backups.protected", fmt.Sprint(overview.Backups.Protected)},
		{"backups.bytes_total", fmt.Sprint(overview.Backups.BytesTotal)},
	}
	for _, key := range []string{"degraded_agents", "failed_jobs", "readiness_errors", "unprotected_backups", "disabled_notification_rules"} {
		rows = append(rows, []string{"attention." + key, fmt.Sprint(overview.Attention[key])})
	}
	for _, key := range []string{"targets", "storages", "schedules", "schedules_paused", "retention_policies", "notification_rules", "notification_rules_enabled", "users"} {
		rows = append(rows, []string{"inventory." + key, fmt.Sprint(overview.Inventory[key])})
	}
	return renderTable([]string{"metric", "value"}, rows)
}
