package postgres

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/kronos/kronos/internal/drivers"
	"github.com/kronos/kronos/internal/manifest"
)

const (
	databaseObjectKind = "database"
	globalsObjectKind  = "postgres_globals"
)

// Driver implements PostgreSQL logical backup with pg_dump plain SQL output.
type Driver struct {
	runner commandRunner
	native pgNativeQueryer
}

type commandRunner interface {
	Run(ctx context.Context, name string, args []string, stdin []byte, env []string) ([]byte, error)
}

type execRunner struct{}

// NewDriver returns a PostgreSQL driver.
func NewDriver() *Driver {
	return &Driver{runner: execRunner{}, native: pgNativeRunner{}}
}

// Name returns the driver name.
func (d *Driver) Name() string {
	return "postgres"
}

// Version returns the local pg_dump version string.
func (d *Driver) Version(ctx context.Context, target drivers.Target) (string, error) {
	out, err := d.run(ctx, "pg_dump", []string{"--version"}, nil, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Test validates that pg_dump can connect and inspect schema metadata.
func (d *Driver) Test(ctx context.Context, target drivers.Target) error {
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = pgNativeRunner{}
		}
		_, err := queryer.SimpleQuery(ctx, target, "select 1")
		return err
	}
	_, err := d.run(ctx, "pg_dump", []string{"--schema-only", "--dbname", postgresDSN(target)}, nil, postgresEnv(target))
	return err
}

// BackupFull emits one plain SQL database record from pg_dump.
func (d *Driver) BackupFull(ctx context.Context, target drivers.Target, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = pgNativeRunner{}
		}
		return pgNativeBackupFull(ctx, target, w, queryer)
	}
	position := "pg_dump:plain"
	if includeGlobals(target) {
		payload, err := d.run(ctx, "pg_dumpall", []string{
			"--globals-only",
			"--no-role-passwords",
			"--dbname", postgresDSN(target),
		}, nil, postgresEnv(target))
		if err != nil {
			return drivers.ResumePoint{}, err
		}
		obj := drivers.ObjectRef{Name: "globals", Kind: globalsObjectKind}
		if err := w.WriteRecord(obj, payload); err != nil {
			return drivers.ResumePoint{}, err
		}
		if err := w.FinishObject(obj, 0); err != nil {
			return drivers.ResumePoint{}, err
		}
		position = "pg_dumpall:globals+pg_dump:plain"
	}
	payload, err := d.run(ctx, "pg_dump", []string{
		"--format=plain",
		"--no-owner",
		"--no-privileges",
		"--dbname", postgresDSN(target),
	}, nil, postgresEnv(target))
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
	return drivers.ResumePoint{Driver: d.Name(), Position: position}, nil
}

// BackupIncremental captures changes since the last backup using pg_current_wal_lsn().
func (d *Driver) BackupIncremental(ctx context.Context, target drivers.Target, parent manifest.Manifest, w drivers.RecordWriter) (drivers.ResumePoint, error) {
	if !useNativeProtocol(target) {
		return drivers.ResumePoint{}, drivers.ErrIncrementalUnsupported
	}
	if w == nil {
		return drivers.ResumePoint{}, fmt.Errorf("record writer is required")
	}
	queryer := d.native
	if queryer == nil {
		queryer = pgNativeRunner{}
	}

	// Get the current LSN as our resume point
	lsn, err := queryer.GetCurrentLSN(ctx, target)
	if err != nil {
		return drivers.ResumePoint{}, err
	}

	// Also capture the LSN from the parent manifest if available
	var parentLSN string
	if parent.Streams != nil {
		parentLSN = parent.Streams["lsn"]
	}

	position := fmt.Sprintf("pgwire:lsn:%s", lsn)

	return drivers.ResumePoint{
		Driver:   d.Name(),
		Position: position,
		Metadata: map[string]string{
			"lsn":       lsn,
			"parent_lsn": parentLSN,
		},
	}, nil
}

// Stream captures WAL records for PITR using pg_logical_slot_get_changes.
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
		queryer = pgNativeRunner{}
	}

	// Extract start LSN from resume point
	startLSN := ""
	if rp.Metadata != nil {
		startLSN = rp.Metadata["lsn"]
	}

	// Use pg_logical_slot_get_changes to stream WAL
	query := "SELECT lsn, xid, data FROM pg_logical_slot_get_changes('kronos_slot', NULL, NULL)"
	if startLSN != "" {
		query = fmt.Sprintf("SELECT lsn, xid, data FROM pg_logical_slot_get_changes('kronos_slot', NULL, NULL) WHERE lsn > '%s'", startLSN)
	}

	result, err := queryer.SimpleQuery(ctx, target, query)
	if err != nil {
		return err
	}

	for _, row := range result.Rows {
		if len(row) < 3 || row[0] == nil || row[2] == nil {
			continue
		}
		record := drivers.StreamRecord{
			ResumePoint: drivers.ResumePoint{
				Driver:   d.Name(),
				Position: *row[0],
				Metadata: map[string]string{
					"lsn": *row[0],
					"xid": *row[1],
				},
			},
			Payload: []byte(*row[2]),
		}
		if err := w.WriteStream(record); err != nil {
			return err
		}
	}
	return nil
}

// Restore applies plain SQL records through psql.
func (d *Driver) Restore(ctx context.Context, target drivers.Target, r drivers.RecordReader, opts drivers.RestoreOptions) error {
	if r == nil {
		return fmt.Errorf("record reader is required")
	}
	if useNativeProtocol(target) {
		queryer := d.native
		if queryer == nil {
			queryer = pgNativeRunner{}
		}
		return pgNativeRestore(ctx, target, r, opts, queryer)
	}
	for {
		record, err := r.NextRecord()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if record.Done || !isRestorableObject(record.Object.Kind) {
			continue
		}
		if opts.DryRun {
			continue
		}
		if !opts.ReplaceExisting {
			return fmt.Errorf("postgres restore requires replace_existing=true because plain SQL restore can overwrite existing objects")
		}
		args := []string{"--single-transaction", "--set", "ON_ERROR_STOP=1", "--dbname", postgresDSN(target)}
		if _, err := d.run(ctx, "psql", args, record.Payload, postgresEnv(target)); err != nil {
			return err
		}
	}
}

func isRestorableObject(kind string) bool {
	return kind == databaseObjectKind || kind == globalsObjectKind
}

// ReplayStream replays WAL records for PITR restore.
func (d *Driver) ReplayStream(ctx context.Context, target drivers.Target, r drivers.StreamReader, targetPoint drivers.ReplayTarget) error {
	if r == nil {
		return fmt.Errorf("stream reader is required")
	}
	if !useNativeProtocol(target) {
		return drivers.ErrIncrementalUnsupported
	}

	queryer := d.native
	if queryer == nil {
		queryer = pgNativeRunner{}
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
		if !targetPoint.Time.IsZero() && !record.ResumePoint.Time.IsZero() {
			if record.ResumePoint.Time.After(targetPoint.Time) {
				return nil
			}
		}
		if targetPoint.Position != "" && record.ResumePoint.Position != "" {
			if record.ResumePoint.Position >= targetPoint.Position {
				return nil
			}
		}

		// Execute the WAL data as SQL
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

func postgresDSN(target drivers.Target) string {
	if value := strings.TrimSpace(target.Connection["dsn"]); value != "" {
		return postgresDSNWithoutPassword(value)
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
	if username != "" {
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

func postgresEnv(target drivers.Target) []string {
	password := firstNonEmpty(target.Connection["password"], target.Options["password"])
	if password == "" {
		if dsn := strings.TrimSpace(target.Connection["dsn"]); dsn != "" {
			password = passwordFromURL(dsn)
		}
	}
	if strings.TrimSpace(password) == "" {
		return nil
	}
	return []string{"PGPASSWORD=" + password}
}

func postgresDSNWithoutPassword(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.User == nil {
		return value
	}
	username := u.User.Username()
	if username == "" {
		u.User = nil
	} else {
		u.User = url.User(username)
	}
	return u.String()
}

func passwordFromURL(value string) string {
	u, err := url.Parse(value)
	if err != nil || u.User == nil {
		return ""
	}
	password, _ := u.User.Password()
	return password
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

func includeGlobals(target drivers.Target) bool {
	value := firstNonEmpty(
		target.Connection["include_globals"],
		target.Connection["includeGlobals"],
		target.Options["include_globals"],
		target.Options["includeGlobals"],
		target.Options["globals"],
	)
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
	case "pg_dump", "shell", "external":
		return false
	default:
		return true
	}
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
