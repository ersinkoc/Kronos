package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

func (d *Driver) psyncStreaming(ctx context.Context, target drivers.Target, rp drivers.ResumePoint, w drivers.StreamWriter, client commander) error {
	if rp.Position == "" {
		<-ctx.Done()
		return ctx.Err()
	}

	masterRunID := "*"
	if rp.Metadata != nil {
		if id := rp.Metadata["master_run_id"]; id != "" {
			masterRunID = id
		}
	}

	masterOffset := int64(0)
	if rp.Metadata != nil {
		if offset := rp.Metadata["master_offset"]; offset != "" {
			fmt.Sscanf(offset, "%d", &masterOffset)
		}
	}

	_, err := client.Do(ctx, "REPLICAOF", target.Connection["master_host"], target.Connection["master_port"])
	if err != nil {
		return fmt.Errorf("replicaof command failed: %w", err)
	}

	psyncResult, err := client.Do(ctx, "PSYNC", masterRunID, fmt.Sprintf("%d", masterOffset))
	if err != nil {
		return fmt.Errorf("psync command failed: %w", err)
	}

	if psyncResult.Type == TypeError {
		if psyncResult.String == "UNKNOWN" {
			_, err := client.Do(ctx, "PSYNC", "?", "1")
			if err != nil {
				return fmt.Errorf("full sync psync failed: %w", err)
			}
		} else {
			return fmt.Errorf("psync error: %s", psyncResult.String)
		}
	}

	return d.captureReplicationStream(ctx, w, client)
}

func (d *Driver) captureReplicationStream(ctx context.Context, w drivers.StreamWriter, client commander) error {
	var lastOffset int64
	eventCount := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		value, err := client.Do(ctx, "WAIT", "1", "0")
		if err != nil {
			return err
		}

		if value.Type == TypeInteger {
			if value.Int > 0 {
				lastOffset = value.Int
			}
		}

		cmdRecord := struct {
			Command []string `json:"command"`
			Args    []string `json:"args,omitempty"`
		}{
			Command: []string{"WAIT", "1", "0"},
			Args:    []string{},
		}
		payload, err := json.Marshal(cmdRecord)
		if err != nil {
			return err
		}

		record := drivers.StreamRecord{
			ResumePoint: drivers.ResumePoint{
				Driver:   "redis",
				Position: fmt.Sprintf("offset:%d", lastOffset),
				Time:     time.Now(),
				Metadata: map[string]string{
					"master_offset": fmt.Sprintf("%d", lastOffset),
				},
			},
			Payload: payload,
		}

		if err := w.WriteStream(record); err != nil {
			return err
		}

		eventCount++
		if eventCount >= 1000 {
			break
		}
	}

	return nil
}

func (d *Driver) BackupIncrementalPSYNC(ctx context.Context, target drivers.Target, parent manifest.Manifest, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	client, err := d.connect(ctx, target)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	masterRunID := "*"
	masterOffset := int64(0)

	if parent.Streams != nil {
		if id, ok := parent.Streams["redis_master_run_id"]; ok {
			masterRunID = id
		}
		if offsetStr, ok := parent.Streams["redis_master_offset"]; ok {
			fmt.Sscanf(offsetStr, "%d", &masterOffset)
		}
	}

	roleResult, err := client.Do(ctx, "ROLE")
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	if roleResult.Type != TypeArray || len(roleResult.Array) < 1 {
		return drivers.ResumePoint{}, fmt.Errorf("unexpected ROLE response")
	}

	role := roleResult.Array[0].String

	if role == "master" {
		return drivers.ResumePoint{}, fmt.Errorf("redis incremental backup requires replica mode")
	}

	infoResult, err := client.Do(ctx, "INFO", "replication")
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	if infoResult.Type != TypeBulkString {
		return drivers.ResumePoint{}, fmt.Errorf("unexpected INFO response")
	}

	masterHost := parseInfoField(infoResult.String, "master_host")
	masterPort := parseInfoField(infoResult.String, "master_port")
	masterLinkStatus := parseInfoField(infoResult.String, "master_link_status")

	if masterLinkStatus == "down" {
		return drivers.ResumePoint{}, fmt.Errorf("master link is down")
	}

	return drivers.ResumePoint{
		Driver:   "redis",
		Position: fmt.Sprintf("psync:%s:%d", masterRunID, masterOffset),
		Metadata: map[string]string{
			"master_host":    masterHost,
			"master_port":    masterPort,
			"master_run_id":  masterRunID,
			"master_offset":  fmt.Sprintf("%d", masterOffset),
		},
	}, nil
}

func (d *Driver) aofReplayStream(ctx context.Context, target drivers.Target, r drivers.StreamReader, targetPoint drivers.ReplayTarget, client commander) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := r.NextStream()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		if !targetPoint.Time.IsZero() && !record.ResumePoint.Time.IsZero() {
			if record.ResumePoint.Time.After(targetPoint.Time) {
				return nil
			}
		}

		if targetPoint.Position != "" {
			if record.ResumePoint.Position >= targetPoint.Position {
				return nil
			}
		}

		command, err := parseStreamCommand(record.Payload)
		if err != nil {
			return err
		}

		if len(command) > 0 {
			if _, err := client.Do(ctx, command...); err != nil {
				return err
			}
		}
	}
}
