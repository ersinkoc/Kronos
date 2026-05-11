package mysql

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

const databaseObjectKind = "database"

// Driver implements MySQL/MariaDB logical backups with mysqldump or native protocol.
type Driver struct {
	runner commandRunner
	native mysqlQueryer
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte, env []string) ([]byte, error)
}

type execRunner struct{}

// NewDriver returns a MySQL driver.
func NewDriver() *Driver {
	return &Driver{runner: execRunner{}, native: mysqlRunner{}}
}

// Name returns the driver name.
func (d *Driver) Name() string {
	return "mysql"
}

// Version returns the local mysql client version.
func (d *Driver) Version(ctx context.Context, target drivers.Target) (string, error) {
	out, err := d.run(ctx, "mysql", []string{"--version"}, nil, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Test validates that mysql can connect and run a trivial query.
func (d *Driver) Test(ctx context.Context, target drivers.Target) error {
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = mysqlRunner{}
		}
		_, err := queryer.SimpleQuery(ctx, target, "SELECT 1")
		return err
	}
	_, err := d.run(ctx, "mysql", append(mysqlArgs(target), "--execute", "SELECT 1"), nil, mysqlEnv(target))
	return err
}

// BackupFull emits one logical SQL dump record from mysqldump or native protocol.
func (d *Driver) BackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = mysqlRunner{}
		}
		return mysqlNativeBackupFull(ctx, target, w, queryer)
	}
	database := databaseName(target)
	args := append(mysqlDumpArgs(target), database)
	payload, err := d.run(ctx, "mysqldump", args, nil, mysqlEnv(target))
	if err != nil {
		return drivers.ResumePoint{}, err
	}
	obj := drivers.ObjectRef{Name: database, Kind: databaseObjectKind}
	if err := w.WriteRecord(obj, payload); err != nil {
		return drivers.ResumePoint{}, err
	}
	if err := w.FinishObject(obj, 0); err != nil {
		return drivers.ResumePoint{}, err
	}
	return drivers.ResumePoint{Driver: d.Name(), Position: "mysqldump:single-transaction"}, nil
}

// BackupIncremental captures binlog position for incremental backup.
func (d *Driver) BackupIncremental(ctx context.Context, target drivers.Target, parent manifest.Manifest, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if !useNativeProtocol(target) {
		return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
	}
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	queryer := d.native
	if queryer == nil {
		queryer = mysqlRunner{}
	}

	file, pos, err := queryer.GetMasterStatus(ctx, target)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	position := fmt.Sprintf("mysql:binlog:%s:%s", file, pos)
	return drivers.ResumePoint{
		Driver:   d.Name(),
		Position: position,
		Metadata: map[string]string{
			"binlog_file": file,
			"binlog_pos":   pos,
		},
	}, nil
}

// Stream streams binlog events for PITR.
func (d *Driver) Stream(ctx context.Context, target drivers.Target, rp drivers.ResumePoint, w drivers.StreamWriter) error {
	if !useNativeProtocol(target) {
		<-ctx.Done()
		return ctx.Err()
	}
	if w == nil {
		return fmt.Errorf("stream writer is required")
	}
	queryer := d.native
	if queryer == nil {
		queryer = mysqlRunner{}
	}

	var position string
	if rp.Metadata != nil {
		position = rp.Metadata["binlog_file"]
	}

	events, err := queryer.GetBinlogEvents(ctx, target, position)
	if err != nil {
		return err
	}

	for _, event := range events {
		record := drivers.StreamRecord{
			ResumePoint: drivers.ResumePoint{
				Driver:   d.Name(),
				Position: fmt.Sprintf("%s:%d", event.LogName, event.Pos),
				Metadata: map[string]string{
					"binlog_file": event.LogName,
					"binlog_pos":  fmt.Sprintf("%d", event.Pos),
				},
			},
			Payload: event.Data,
		}
		if err := w.WriteStream(record); err != nil {
			return err
		}
	}
	return nil
}

// Restore applies SQL records through mysql.
func (d *Driver) Restore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions) error {
	if r == nil {
		return fmt.Errorf("record reader is required")
	}
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = mysqlRunner{}
		}
		return mysqlNativeRestore(ctx, target, r, opts, queryer)
	}
	for {
		record, err := r.NextRecord()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Done || record.Object.Kind != databaseObjectKind {
			continue
		}
		if opts.DryRun {
			continue
		}
		if !opts.ReplaceExisting {
			return fmt.Errorf("mysql restore requires replace_existing=true because SQL restore can overwrite existing objects")
		}
		if _, err := d.run(ctx, "mysql", mysqlArgs(target), record.Payload, mysqlEnv(target)); err != nil {
			return err
		}
	}
}

// ReplayStream replays binlog events for PITR restore.
func (d *Driver) ReplayStream(ctx context.Context, target drivers.Target, r drivers.StreamReader, targetPoint drivers.ReplayTarget) error {
	if !useNativeProtocol(target) {
		return drivers.ErrIncrementalUnsupported
	}
	if r == nil {
		return fmt.Errorf("stream reader is required")
	}
	queryer := d.native
	if queryer == nil {
		queryer = mysqlRunner{}
	}

	for {
		record, err := r.NextStream()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		// Check if we've reached the target replay point
		if targetPoint.Position != "" {
			if record.ResumePoint.Position >= targetPoint.Position {
				return nil
			}
		}
		if !targetPoint.Time.IsZero() && !record.ResumePoint.Time.IsZero() {
			if record.ResumePoint.Time.After(targetPoint.Time) {
				return nil
			}
		}

		// Execute the binlog data as SQL
		if len(record.Payload) > 0 {
			_, err := queryer.SimpleQuery(ctx, target, string(record.Payload))
			if err != nil {
				return err
			}
		}
	}
}

func (d *Driver) run(ctx context.Context, name string, args []string, stdin []byte, env []string) ([]byte, error) {
	runner := d.runner
	if runner == nil {
		runner = execRunner{}
	}
	return runner.Run(ctx, name, args, stdin, env)
}

func (execRunner) Run(ctx context.Context, name string, args []string, stdin []byte, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("%s: %w: %s", name, err, message)
	}
	return out, nil
}

func mysqlDumpArgs(target drivers.Target) []string {
	args := append(mysqlConnectionArgs(target),
		"--single-transaction",
		"--routines",
		"--triggers",
		"--events",
	)
	if mode := mysqlSetGTIDPurgedMode(target); mode != "" {
		args = append(args, "--set-gtid-purged="+mode)
	}
	return args
}

func mysqlSetGTIDPurgedMode(target drivers.Target) string {
	value := strings.TrimSpace(firstNonEmpty(
		target.Options["set_gtid_purged"],
		target.Options["dump_set_gtid_purged"],
		target.Connection["set_gtid_purged"],
	))
	switch strings.ToLower(value) {
	case "", "false", "0", "off":
		return "OFF"
	case "omit", "unsupported", "none":
		return ""
	default:
		return strings.ToUpper(value)
	}
}

func mysqlArgs(target drivers.Target) []string {
	return append(mysqlConnectionArgs(target), "--database", databaseName(target))
}

func mysqlConnectionArgs(target drivers.Target) []string {
	host, port := splitAddress(target.Connection["addr"])
	if value := strings.TrimSpace(target.Connection["host"]); value != "" {
		host = value
	}
	if value := strings.TrimSpace(target.Connection["port"]); value != "" {
		port = value
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "3306"
	}
	args := []string{"--host", host, "--port", port}
	if username := strings.TrimSpace(firstNonEmpty(target.Connection["username"], target.Connection["user"])); username != "" {
		args = append(args, "--user", username)
	}
	return args
}

func mysqlEnv(target drivers.Target) []string {
	password := firstNonEmpty(target.Connection["password"], target.Options["password"])
	if strings.TrimSpace(password) == "" {
		return nil
	}
	return []string{"MYSQL_PWD=" + password}
}

func databaseName(target drivers.Target) string {
	if value := strings.TrimSpace(target.Connection["database"]); value != "" {
		return value
	}
	if value := strings.TrimSpace(target.Options["database"]); value != "" {
		return value
	}
	return "mysql"
}

func splitAddress(address string) (string, string) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", ""
	}
	host, port, err := net.SplitHostPort(address)
	if err == nil {
		return host, port
	}
	if strings.Count(address, ":") == 1 {
		parts := strings.SplitN(address, ":", 2)
		return parts[0], parts[1]
	}
	return address, ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func useNativeProtocol(target drivers.Target) bool {
	value := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		target.Connection["protocol"],
		target.Connection["native_protocol"],
		target.Connection["native"],
		target.Options["protocol"],
		target.Options["native_protocol"],
		target.Options["native"],
	)))
	switch value {
	case "mysqldump", "shell", "external":
		return false
	default:
		return true
	}
}
