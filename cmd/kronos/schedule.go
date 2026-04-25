package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/kronos/kronos/internal/core"
)

func runSchedule(ctx context.Context, out io.Writer, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("schedule subcommand is required")
	}
	switch args[0] {
	case "add":
		return runScheduleAdd(ctx, out, args[1:])
	case "inspect":
		return runScheduleInspect(ctx, out, args[1:])
	case "list":
		return runScheduleList(ctx, out, args[1:])
	case "pause":
		return runSchedulePause(ctx, out, args[1:], true)
	case "remove":
		return runScheduleRemove(ctx, out, args[1:])
	case "resume":
		return runSchedulePause(ctx, out, args[1:], false)
	case "tick":
		return runScheduleTick(ctx, out, args[1:])
	case "update":
		return runScheduleUpdate(ctx, out, args[1:])
	default:
		return fmt.Errorf("unknown schedule subcommand %q", args[0])
	}
}

func runScheduleList(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule list", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules", out)
}

func runScheduleInspect(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule inspect", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "schedule id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return getControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules/"+*id, out)
}

func runScheduleAdd(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule add", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "schedule id")
	name := fs.String("name", "", "schedule name")
	targetID := fs.String("target", "", "target id")
	storageID := fs.String("storage", "", "storage id")
	backupType := fs.String("type", string(core.BackupTypeFull), "backup type")
	expression := fs.String("cron", "", "cron expression or @between window")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *targetID == "" {
		return fmt.Errorf("--target is required")
	}
	if *storageID == "" {
		return fmt.Errorf("--storage is required")
	}
	if *expression == "" {
		return fmt.Errorf("--cron is required")
	}
	payload := core.Schedule{
		ID:         core.ID(*id),
		Name:       *name,
		TargetID:   core.ID(*targetID),
		StorageID:  core.ID(*storageID),
		BackupType: core.BackupType(*backupType),
		Expression: *expression,
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules", payload, out)
}

func runScheduleUpdate(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule update", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "schedule id")
	name := fs.String("name", "", "schedule name")
	targetID := fs.String("target", "", "target id")
	storageID := fs.String("storage", "", "storage id")
	backupType := fs.String("type", string(core.BackupTypeFull), "backup type")
	expression := fs.String("cron", "", "cron expression or @between window")
	retentionPolicy := fs.String("retention-policy", "", "retention policy id")
	paused := fs.Bool("paused", false, "create schedule in paused state")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	if *targetID == "" {
		return fmt.Errorf("--target is required")
	}
	if *storageID == "" {
		return fmt.Errorf("--storage is required")
	}
	if *expression == "" {
		return fmt.Errorf("--cron is required")
	}
	payload := core.Schedule{
		ID:              core.ID(*id),
		Name:            *name,
		TargetID:        core.ID(*targetID),
		StorageID:       core.ID(*storageID),
		BackupType:      core.BackupType(*backupType),
		Expression:      *expression,
		RetentionPolicy: core.ID(*retentionPolicy),
		Paused:          *paused,
	}
	return putControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules/"+*id, payload, out)
}

func runScheduleRemove(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule remove", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "schedule id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return deleteControl(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules/"+*id, out)
}

func runSchedulePause(ctx context.Context, out io.Writer, args []string, paused bool) error {
	name := "schedule pause"
	action := "pause"
	if !paused {
		name = "schedule resume"
		action = "resume"
	}
	fs := newFlagSet(name, out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	id := fs.String("id", "", "schedule id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("--id is required")
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/schedules/"+*id+"/"+action, nil, out)
}

func runScheduleTick(ctx context.Context, out io.Writer, args []string) error {
	fs := newFlagSet("schedule tick", out)
	serverAddr := fs.String("server", "127.0.0.1:8500", "server address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return postControlJSON(ctx, http.DefaultClient, *serverAddr, "/api/v1/scheduler/tick", nil, out)
}
