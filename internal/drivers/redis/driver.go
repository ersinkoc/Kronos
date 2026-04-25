package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

// Driver implements Redis/Valkey logical backup via SCAN + DUMP.
type Driver struct {
	dial func(context.Context, drivers.Target) (commander, error)
}

type commander interface {
	Do(ctx context.Context, args ...string) (Value, error)
	Auth(ctx context.Context, username string, password string) error
	Hello(ctx context.Context, version int, username string, password string) (Value, error)
}

// NewDriver returns a Redis driver.
func NewDriver() *Driver {
	return &Driver{dial: dialTarget}
}

// Name returns the driver name.
func (d *Driver) Name() string {
	return "redis"
}

// Version returns INFO server redis_version when available.
func (d *Driver) Version(ctx context.Context, target drivers.Target) (string, error) {
	client, err := d.connect(ctx, target)
	if err != nil {
		return "", err
	}
	value, err := client.Do(ctx, "INFO", "server")
	if err != nil {
		return "", err
	}
	if value.Type != TypeBulkString {
		return "", fmt.Errorf("unexpected INFO response: %#v", value)
	}
	return parseInfoField(value.String, "redis_version"), nil
}

// Test validates connectivity and authentication.
func (d *Driver) Test(ctx context.Context, target drivers.Target) error {
	client, err := d.connect(ctx, target)
	if err != nil {
		return err
	}
	value, err := client.Do(ctx, "PING")
	if err != nil {
		return err
	}
	if value.Type != TypeSimpleString || value.String != "PONG" {
		return fmt.Errorf("unexpected PING response: %#v", value)
	}
	return nil
}

// BackupFull emits DUMP payload records for every key reachable by SCAN.
func (d *Driver) BackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	client, err := d.connect(ctx, target)
	if err != nil {
		return drivers.ResumePoint{}, err
	}
	if err := d.backupACL(ctx, client, w); err != nil {
		return drivers.ResumePoint{}, err
	}
	cursor := "0"
	for {
		if err := ctx.Err(); err != nil {
			return drivers.ResumePoint{}, err
		}
		response, err := client.Do(ctx, "SCAN", cursor, "COUNT", "100")
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		nextCursor, keys, err := parseScanResponse(response)
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		for _, key := range keys {
			if err := d.backupKey(ctx, client, key, w); err != nil {
				return drivers.ResumePoint{}, err
			}
		}
		cursor = nextCursor
		if cursor == "0" {
			break
		}
	}
	return drivers.ResumePoint{Driver: d.Name(), Position: "scan:0"}, nil
}

// BackupIncremental is not supported by Redis SCAN/DUMP logical backups.
func (d *Driver) BackupIncremental(context.Context, drivers.Target, manifest.Manifest, drivers.RecordWriter) (drivers.ResumePoint, error) {
	return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
}

// Stream is reserved for PSYNC/AOF-like capture.
func (d *Driver) Stream(ctx context.Context, target drivers.Target, rp drivers.ResumePoint, w drivers.StreamWriter) error {
	<-ctx.Done()
	return ctx.Err()
}

// Restore applies DUMP records using RESTORE.
func (d *Driver) Restore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions) error {
	client, err := d.connect(ctx, target)
	if err != nil {
		return err
	}
	for {
		record, err := r.NextRecord()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Done {
			continue
		}
		if record.Object.Kind == "acl" {
			if err := restoreACL(ctx, client, record.Payload, opts); err != nil {
				return err
			}
			continue
		}
		var redisRecord keyRecord
		if err := json.Unmarshal(record.Payload, &redisRecord); err != nil {
			return err
		}
		args := []string{"RESTORE", redisRecord.Key, strconv.FormatInt(redisRecord.TTLMillis, 10), string(redisRecord.Dump)}
		if opts.ReplaceExisting {
			args = append(args, "REPLACE")
		}
		if opts.DryRun {
			continue
		}
		if _, err := client.Do(ctx, args...); err != nil {
			return err
		}
	}
}

// ReplayStream replays JSON encoded Redis command records.
func (d *Driver) ReplayStream(ctx context.Context, target drivers.Target, r drivers.StreamReader, targetPoint drivers.ReplayTarget) error {
	if r == nil {
		return fmt.Errorf("stream reader is required")
	}
	client, err := d.connect(ctx, target)
	if err != nil {
		return err
	}
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
		if !targetPoint.Time.IsZero() && record.ResumePoint.Time.After(targetPoint.Time) {
			return nil
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
		if targetPoint.Position != "" && record.ResumePoint.Position == targetPoint.Position {
			return nil
		}
	}
}

func (d *Driver) connect(ctx context.Context, target drivers.Target) (commander, error) {
	if d.dial == nil {
		d.dial = dialTarget
	}
	client, err := d.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	username := target.Connection["username"]
	password := target.Connection["password"]
	if _, err := client.Hello(ctx, 3, username, password); err == nil {
		return client, nil
	}
	if err := client.Auth(ctx, username, password); err != nil {
		return nil, err
	}
	return client, nil
}

func dialTarget(ctx context.Context, target drivers.Target) (commander, error) {
	address := target.Connection["addr"]
	if address == "" {
		host := target.Connection["host"]
		port := target.Connection["port"]
		if host == "" {
			host = "127.0.0.1"
		}
		if port == "" {
			port = "6379"
		}
		address = host + ":" + port
	}
	return Dial(ctx, address)
}

type keyRecord struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	TTLMillis int64  `json:"ttl_millis"`
	Dump      []byte `json:"dump"`
}

type aclRecord struct {
	Users []string `json:"users"`
}

type streamCommandRecord struct {
	Command []string `json:"command"`
	Args    []string `json:"args,omitempty"`
}

func parseStreamCommand(payload []byte) ([]string, error) {
	var command []string
	if err := json.Unmarshal(payload, &command); err == nil {
		if len(command) == 0 {
			return nil, nil
		}
		return command, nil
	}
	var record streamCommandRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return nil, err
	}
	command = append([]string(nil), record.Command...)
	command = append(command, record.Args...)
	return command, nil
}

func (d *Driver) backupACL(ctx context.Context, client commander, w drivers.RecordWriter) error {
	value, err := client.Do(ctx, "ACL", "LIST")
	if err != nil {
		return err
	}
	if value.Type != TypeArray {
		return fmt.Errorf("unexpected ACL LIST response: %#v", value)
	}
	record := aclRecord{Users: make([]string, 0, len(value.Array))}
	for _, user := range value.Array {
		if user.Type != TypeBulkString && user.Type != TypeSimpleString {
			return fmt.Errorf("unexpected ACL LIST user entry: %#v", user)
		}
		record.Users = append(record.Users, user.String)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	obj := drivers.ObjectRef{Name: "acl", Kind: "acl"}
	if err := w.WriteRecord(obj, payload); err != nil {
		return err
	}
	return w.FinishObject(obj, int64(len(record.Users)))
}

func restoreACL(ctx context.Context, client commander, payload []byte, opts drivers.RestoreOptions) error {
	var record aclRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return err
	}
	for _, user := range record.Users {
		fields := strings.Fields(user)
		if len(fields) < 2 || fields[0] != "user" {
			return fmt.Errorf("invalid ACL user entry %q", user)
		}
		args := append([]string{"ACL", "SETUSER", fields[1]}, fields[2:]...)
		if opts.DryRun {
			continue
		}
		if _, err := client.Do(ctx, args...); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) backupKey(ctx context.Context, client commander, key string, w drivers.RecordWriter) error {
	typeValue, err := client.Do(ctx, "TYPE", key)
	if err != nil {
		return err
	}
	ttlValue, err := client.Do(ctx, "PTTL", key)
	if err != nil {
		return err
	}
	dumpValue, err := client.Do(ctx, "DUMP", key)
	if err != nil {
		return err
	}
	if dumpValue.Type == TypeBulkString && dumpValue.Null {
		return nil
	}
	record := keyRecord{
		Key:       key,
		Type:      typeValue.String,
		TTLMillis: ttlValue.Int,
		Dump:      []byte(dumpValue.String),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	obj := drivers.ObjectRef{Name: key, Kind: typeValue.String}
	if err := w.WriteRecord(obj, payload); err != nil {
		return err
	}
	return w.FinishObject(obj, 1)
}

func parseScanResponse(value Value) (string, []string, error) {
	if value.Type != TypeArray || len(value.Array) != 2 {
		return "", nil, fmt.Errorf("unexpected SCAN response: %#v", value)
	}
	cursor := value.Array[0].String
	keyValues := value.Array[1]
	if keyValues.Type != TypeArray {
		return "", nil, fmt.Errorf("unexpected SCAN key list: %#v", keyValues)
	}
	keys := make([]string, 0, len(keyValues.Array))
	for _, key := range keyValues.Array {
		keys = append(keys, key.String)
	}
	return cursor, keys, nil
}

func parseInfoField(info string, name string) string {
	prefix := name + ":"
	for len(info) > 0 {
		line := info
		if idx := indexByte(line, '\n'); idx >= 0 {
			line = info[:idx]
			info = info[idx+1:]
		} else {
			info = ""
		}
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			return line[len(prefix):]
		}
	}
	return ""
}

func indexByte(value string, needle byte) int {
	for i := 0; i < len(value); i++ {
		if value[i] == needle {
			return i
		}
	}
	return -1
}
