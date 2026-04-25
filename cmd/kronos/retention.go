package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/retention"
)

type retentionPlanInput struct {
	Policy  core.RetentionPolicy `json:"policy"`
	Backups []core.Backup        `json:"backups"`
	Now     time.Time            `json:"now,omitempty"`
}

type retentionServerRequest struct {
	Policy core.RetentionPolicy `json:"policy"`
	Now    time.Time            `json:"now,omitempty"`
	DryRun bool                 `json:"dry_run,omitempty"`
}

type retentionPlanOutput struct {
	Items []retentionPlanItem `json:"items"`
}

type retentionPlanItem struct {
	ID      core.ID  `json:"id"`
	Keep    bool     `json:"keep"`
	Reasons []string `json:"reasons,omitempty"`
}

func runRetention(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("retention subcommand is required")
	}
	switch args[0] {
	case "apply":
		return runRetentionApply(ctx, out, args[1:])
	case "policy":
		return runRetentionPolicy(ctx, out, args[1:])
	case "plan":
		return runRetentionPlan(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown retention subcommand %q", args[0])
	}
}

func runRetentionPlan(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention plan", out)
	inputPath := fs.String("input", "", "JSON file containing policy, backups, and optional now")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" {
		return fmt.Errorf("--input is required")
	}
	data, err := readFileBounded(*inputPath, 64*1024*1024)
	if err != nil {
		return err
	}
	var input retentionPlanInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	plan, err := retention.Resolve(input.Backups, input.Policy, input.Now)
	if err != nil {
		return err
	}
	result := retentionPlanOutput{Items: make([]retentionPlanItem, 0, len(plan.Items))}
	for _, item := range plan.Items {
		result.Items = append(result.Items, retentionPlanItem{
			ID:      item.Backup.ID,
			Keep:    item.Keep,
			Reasons: item.Reasons,
		})
	}
	return writeCommandJSON(ctx, out, result)
}

func runRetentionApply(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention apply", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	inputPath := fs.String("input", "", "JSON file containing policy and optional now")
	dryRun := fs.Bool("dry-run", false, "preview deleted backups without modifying server state")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" {
		return fmt.Errorf("--input is required")
	}
	data, err := readFileBounded(*inputPath, 64*1024*1024)
	if err != nil {
		return err
	}
	var input retentionServerRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	if *dryRun {
		input.DryRun = true
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/apply", input, out)
}

func runRetentionPolicy(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("retention policy subcommand is required")
	}
	switch args[0] {
	case "add":
		return runRetentionPolicyAdd(ctx, out, args[1:])
	case "inspect":
		return runRetentionPolicyInspect(ctx, out, args[1:])
	case "list":
		return runRetentionPolicyList(ctx, out, args[1:])
	case "remove":
		return runRetentionPolicyRemove(ctx, out, args[1:])
	case "update":
		return runRetentionPolicyUpdate(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown retention policy subcommand %q", args[0])
	}
}

func runRetentionPolicyAdd(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention policy add", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	inputPath := fs.String("input", "", "JSON file containing a retention policy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inputPath == "" {
		return fmt.Errorf("--input is required")
	}
	data, err := readFileBounded(*inputPath, 64*1024*1024)
	if err != nil {
		return err
	}
	var policy core.RetentionPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return err
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/policies", policy, out)
}

func runRetentionPolicyList(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention policy list", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/policies", out)
}

func runRetentionPolicyInspect(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention policy inspect", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "retention policy id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/policies/"+*id, out)
}

func runRetentionPolicyRemove(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention policy remove", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "retention policy id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return deleteControl(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/policies/"+*id, out)
}

func runRetentionPolicyUpdate(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("retention policy update", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "retention policy id")
	inputPath := fs.String("input", "", "JSON file containing a retention policy")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	if *inputPath == "" {
		return fmt.Errorf("--input is required")
	}
	data, err := readFileBounded(*inputPath, 64*1024*1024)
	if err != nil {
		return err
	}
	var policy core.RetentionPolicy
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return err
	}
	return putControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/retention/policies/"+*id, policy, out)
}
