package drivers

import (
	"context"
	"errors"
	"time"

	"github.com/kronos/kronos/internal/manifest"
)

// ErrIncrementalUnsupported indicates a driver cannot produce a cheap incremental backup.
var ErrIncrementalUnsupported = errors.New("incremental backup unsupported")

// Driver is implemented by every database integration.
type Driver interface {
	Name() string
	Version(ctx context.Context, target Target) (string, error)
	Test(ctx context.Context, target Target) error
	BackupFull(ctx context.Context, target Target, w RecordWriter) (ResumePoint, error)
	BackupIncremental(ctx context.Context, target Target, parent manifest.Manifest, w RecordWriter) (ResumePoint, error)
	Stream(ctx context.Context, target Target, rp ResumePoint, w StreamWriter) error
	Restore(ctx context.Context, target Target, r RecordReader, opts RestoreOptions) error
	ReplayStream(ctx context.Context, target Target, r StreamReader, targetPoint ReplayTarget) error
}

// Target is the connection material passed to a database driver.
type Target struct {
	Name       string
	Driver     string
	Connection map[string]string
	Options    map[string]string
}

// ObjectRef identifies one logical object in a driver stream.
type ObjectRef struct {
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
	Kind   string `json:"kind,omitempty"`
}

// ResumePoint identifies where streaming/incremental capture should resume.
type ResumePoint struct {
	Driver   string            `json:"driver"`
	Position string            `json:"position"`
	Time     time.Time         `json:"time,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ReplayTarget tells a stream restore when to stop.
type ReplayTarget struct {
	Position string
	Time     time.Time
}

// RestoreOptions configures restore behavior.
type RestoreOptions struct {
	ReplaceExisting bool
	DryRun          bool
	Metadata        map[string]string
}

// RecordWriter is the driver's logical backup output.
type RecordWriter interface {
	WriteRecord(obj ObjectRef, payload []byte) error
	FinishObject(obj ObjectRef, rows int64) error
}

// RecordReader is consumed by restore implementations.
type RecordReader interface {
	NextRecord() (Record, error)
}

// StreamWriter is the driver's continuous stream output.
type StreamWriter interface {
	WriteStream(record StreamRecord) error
}

// StreamReader is consumed by stream replay implementations.
type StreamReader interface {
	NextStream() (StreamRecord, error)
}

// Record is one logical backup payload.
type Record struct {
	Object  ObjectRef
	Payload []byte
	Rows    int64
	Done    bool
}

// StreamRecord is one PITR stream payload.
type StreamRecord struct {
	ResumePoint ResumePoint
	Payload     []byte
}
