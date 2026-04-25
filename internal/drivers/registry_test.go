package drivers

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/kronos/kronos/internal/manifest"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	driver := fakeDriver{name: "redis"}
	if err := registry.Register(driver); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(driver); err == nil {
		t.Fatal("Register(duplicate) error = nil, want error")
	}
	got, ok := registry.Get("redis")
	if !ok || got.Name() != "redis" {
		t.Fatalf("Get(redis) = %v, %v", got, ok)
	}
	names := registry.Names()
	if len(names) != 1 || names[0] != "redis" {
		t.Fatalf("Names() = %v", names)
	}
}

func TestMemoryRecordStream(t *testing.T) {
	t.Parallel()

	var stream MemoryRecordStream
	obj := ObjectRef{Schema: "public", Name: "users", Kind: "table"}
	if err := stream.WriteRecord(obj, []byte("row")); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if err := stream.FinishObject(obj, 1); err != nil {
		t.Fatalf("FinishObject() error = %v", err)
	}
	records := stream.Records()
	records[0].Done = true
	if stream.Records()[0].Done {
		t.Fatal("Records() returned mutable record slice")
	}
	record, err := stream.NextRecord()
	if err != nil {
		t.Fatalf("NextRecord(first) error = %v", err)
	}
	if string(record.Payload) != "row" || record.Done {
		t.Fatalf("record = %#v", record)
	}
	record, err = stream.NextRecord()
	if err != nil {
		t.Fatalf("NextRecord(second) error = %v", err)
	}
	if !record.Done || record.Rows != 1 {
		t.Fatalf("finish record = %#v", record)
	}
	if _, err := stream.NextRecord(); !errors.Is(err, io.EOF) {
		t.Fatalf("NextRecord(eof) error = %v, want io.EOF", err)
	}
}

type fakeDriver struct {
	name string
}

func (d fakeDriver) Name() string { return d.name }

func (d fakeDriver) Version(context.Context, Target) (string, error) { return "test", nil }

func (d fakeDriver) Test(context.Context, Target) error { return nil }

func (d fakeDriver) BackupFull(context.Context, Target, RecordWriter) (ResumePoint, error) {
	return ResumePoint{}, nil
}

func (d fakeDriver) BackupIncremental(context.Context, Target, manifest.Manifest, RecordWriter) (ResumePoint, error) {
	return ResumePoint{}, ErrIncrementalUnsupported
}

func (d fakeDriver) Stream(context.Context, Target, ResumePoint, StreamWriter) error { return nil }

func (d fakeDriver) Restore(context.Context, Target, RecordReader, RestoreOptions) error { return nil }

func (d fakeDriver) ReplayStream(context.Context, Target, StreamReader, ReplayTarget) error {
	return nil
}
