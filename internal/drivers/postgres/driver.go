package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

const databaseObjectKind = "database"

// Driver implements PostgreSQL logical backup with pg_dump plain SQL output.
type Driver struct {
	runner commandRunner
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

type execRunner struct{}

// NewDriver returns a PostgreSQL driver.
func NewDriver() *Driver {
	return &Driver{runner: execRunner{}}
}

// Name returns the driver name.
func (d *Driver) Name() string {
	return "postgres"
}

// Version returns the local pg_dump version string.
func (d *Driver) Version(ctx context.Context, target drivers.Target) (string, error) {
	out, err := d.run(ctx, "pg_dump", []string{"--version"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Test validates that pg_dump can connect and inspect schema metadata.
func (d *Driver) Test(ctx context.Context, target drivers.Target) error {
	_, err := d.run(ctx, "pg_dump", []string{"--schema-only", "--dbname", postgresDSN(target)}, nil)
	return err
}

// BackupFull emits one plain SQL database record from pg_dump.
func (d *Driver) BackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	payload, err := d.run(ctx, "pg_dump", []string{
		"--format=plain",
		"--no-owner",
		"--no-privileges",
		"--dbname", postgresDSN(target),
	}, nil)
	if err != nil {
		return drivers.ResumePoint{}, err
	}
	obj := drivers.ObjectRef{Schema: "public", Name: databaseName(target), Kind: databaseObjectKind}
	if err := w.WriteRecord(obj, payload); err != nil {
		return drivers.ResumePoint{}, err
	}
	if err := w.FinishObject(obj, 0); err != nil {
		return drivers.ResumePoint{}, err
	}
	return drivers.ResumePoint{Driver: d.Name(), Position: "pg_dump:plain"}, nil
}

// BackupIncremental is not supported by plain pg_dump logical backups.
func (d *Driver) BackupIncremental(context.Context, drivers.Target, manifest.Manifest, drivers.RecordWriter) (drivers.ResumePoint, error) {
	return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
}

// Stream is reserved for WAL archiving or logical replication capture.
func (d *Driver) Stream(ctx context.Context, _ drivers.Target, _ drivers.ResumePoint, _ drivers.StreamWriter) error {
	<-ctx.Done()
	return ctx.Err()
}

// Restore applies plain SQL records through psql.
func (d *Driver) Restore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions) error {
	if r == nil {
		return fmt.Errorf("record reader is required")
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
			return fmt.Errorf("postgres restore requires replace_existing=true because plain SQL restore can overwrite existing objects")
		}
		args := []string{"--single-transaction", "--set", "ON_ERROR_STOP=1", "--dbname", postgresDSN(target)}
		if _, err := d.run(ctx, "psql", args, record.Payload); err != nil {
			return err
		}
	}
}

// ReplayStream is reserved for WAL or logical replication replay.
func (d *Driver) ReplayStream(context.Context, drivers.Target, drivers.StreamReader, drivers.ReplayTarget) error {
	return drivers.ErrIncrementalUnsupported
}

func (d *Driver) run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	runner := d.runner
	if runner == nil {
		runner = execRunner{}
	}
	return runner.Run(ctx, name, args, stdin)
}

func (execRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
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

func postgresDSN(target drivers.Target) string {
	if value := strings.TrimSpace(target.Connection["dsn"]); value != "" {
		return value
	}
	database := databaseName(target)
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
		port = "5432"
	}
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	username := target.Connection["username"]
	password := target.Connection["password"]
	if username != "" && password != "" {
		u.User = url.UserPassword(username, password)
	} else if username != "" {
		u.User = url.User(username)
	}
	query := u.Query()
	sslMode := strings.TrimSpace(firstNonEmpty(target.Connection["sslmode"], target.Connection["tls"], target.Options["sslmode"], target.Options["tls"]))
	switch strings.ToLower(sslMode) {
	case "", "disable", "false", "off":
		query.Set("sslmode", "disable")
	case "true", "on":
		query.Set("sslmode", "require")
	default:
		query.Set("sslmode", sslMode)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func databaseName(target drivers.Target) string {
	if value := strings.TrimSpace(target.Connection["database"]); value != "" {
		return value
	}
	if value := strings.TrimSpace(target.Options["database"]); value != "" {
		return value
	}
	return "postgres"
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
