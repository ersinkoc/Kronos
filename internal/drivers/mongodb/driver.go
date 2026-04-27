package mongodb

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

// Driver implements MongoDB logical backups with mongodump archive output.
type Driver struct {
	runner commandRunner
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error)
}

type execRunner struct{}

// NewDriver returns a MongoDB driver.
func NewDriver() *Driver {
	return &Driver{runner: execRunner{}}
}

// Name returns the driver name.
func (d *Driver) Name() string {
	return "mongodb"
}

// Version returns the local mongodump version string.
func (d *Driver) Version(ctx context.Context, target drivers.Target) (string, error) {
	out, err := d.run(ctx, "mongodump", []string{"--version"}, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Test validates that mongodump can connect to the target database.
func (d *Driver) Test(ctx context.Context, target drivers.Target) error {
	args := append(mongoDumpArgs(target), "--collection", connectionTestCollection(target))
	_, err := d.run(ctx, "mongodump", args, nil)
	return err
}

// BackupFull emits one MongoDB archive record from mongodump.
func (d *Driver) BackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	database := databaseName(target)
	payload, err := d.run(ctx, "mongodump", mongoDumpArgs(target), nil)
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
	return drivers.ResumePoint{Driver: d.Name(), Position: "mongodump:archive"}, nil
}

// BackupIncremental is not supported by mongodump logical backups.
func (d *Driver) BackupIncremental(context.Context, drivers.Target, manifest.Manifest, drivers.RecordWriter) (drivers.ResumePoint, error) {
	return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
}

// Stream is reserved for oplog/change-stream capture.
func (d *Driver) Stream(ctx context.Context, _ drivers.Target, _ drivers.ResumePoint, _ drivers.StreamWriter) error {
	<-ctx.Done()
	return ctx.Err()
}

// Restore applies MongoDB archive records through mongorestore.
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
			return fmt.Errorf("mongodb restore requires replace_existing=true because archive restore can overwrite existing collections")
		}
		args := mongoRestoreArgs(target, record.Object.Name)
		if _, err := d.run(ctx, "mongorestore", args, record.Payload); err != nil {
			return err
		}
	}
}

// ReplayStream is reserved for oplog/change-stream replay.
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

func mongoDumpArgs(target drivers.Target) []string {
	return []string{"--uri", mongoURI(target), "--db", databaseName(target), "--archive"}
}

func mongoRestoreArgs(target drivers.Target, sourceDatabase string) []string {
	args := []string{"--uri", mongoURI(target), "--archive", "--drop"}
	targetDatabase := databaseName(target)
	if sourceDatabase != "" && targetDatabase != "" && sourceDatabase != targetDatabase {
		args = append(args, "--nsFrom", sourceDatabase+".*", "--nsTo", targetDatabase+".*")
	}
	return args
}

func mongoURI(target drivers.Target) string {
	if value := strings.TrimSpace(firstNonEmpty(target.Connection["uri"], target.Connection["dsn"], target.Options["uri"], target.Options["dsn"])); value != "" {
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
		port = "27017"
	}
	u := url.URL{
		Scheme: "mongodb",
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	username := firstNonEmpty(target.Connection["username"], target.Connection["user"])
	password := firstNonEmpty(target.Connection["password"], target.Options["password"])
	if username != "" && password != "" {
		u.User = url.UserPassword(username, password)
	} else if username != "" {
		u.User = url.User(username)
	}
	query := u.Query()
	if authSource := strings.TrimSpace(firstNonEmpty(target.Connection["authSource"], target.Connection["auth_source"], target.Options["authSource"], target.Options["auth_source"])); authSource != "" {
		query.Set("authSource", authSource)
	}
	tlsMode := strings.ToLower(strings.TrimSpace(firstNonEmpty(target.Connection["tls"], target.Options["tls"], target.Connection["ssl"], target.Options["ssl"])))
	switch tlsMode {
	case "true", "on", "require", "required":
		query.Set("tls", "true")
	case "false", "off", "disable", "disabled":
		query.Set("tls", "false")
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
	return "admin"
}

func connectionTestCollection(target drivers.Target) string {
	if value := strings.TrimSpace(firstNonEmpty(target.Options["connection_test_collection"], target.Connection["connection_test_collection"])); value != "" {
		return value
	}
	return "__kronos_connection_test__"
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
