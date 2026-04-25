package audit

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestLogAppendListVerify(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	clock := core.NewFakeClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	log, err := New(db, clock)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	first, err := log.Append(context.Background(), core.AuditEvent{
		Action:       "target.created",
		ResourceType: "target",
		ResourceID:   "target-1",
	})
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := log.Append(context.Background(), core.AuditEvent{
		Action:       "backup.started",
		ResourceType: "backup",
		ResourceID:   "backup-1",
	})
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if first.Seq != 1 || second.Seq != 2 || second.PrevHash != first.Hash || second.Hash == "" {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	events, err := log.List(context.Background(), 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(events) != 2 || events[0].ID != first.ID || events[1].ID != second.ID {
		t.Fatalf("events = %#v", events)
	}
	if err := log.Verify(context.Background()); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestLogVerifyDetectsTamper(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	log, err := New(db, core.NewFakeClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	event, err := log.Append(context.Background(), core.AuditEvent{Action: "storage.created", ResourceType: "storage"})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	event.Action = "storage.deleted"
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	err = db.Update(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(eventsBucket)
		if err != nil {
			return err
		}
		return bucket.Put(seqKeyBytes(event.Seq), data)
	})
	if err != nil {
		t.Fatalf("tamper Put() error = %v", err)
	}
	if err := log.Verify(context.Background()); err == nil {
		t.Fatal("Verify() error = nil, want tamper detection")
	}
}

func TestLogAppendValidatesRequiredFields(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(t.TempDir() + "/state.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	log, err := New(db, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := log.Append(context.Background(), core.AuditEvent{ResourceType: "target"}); err == nil {
		t.Fatal("Append(no action) error = nil, want error")
	}
	if _, err := log.Append(context.Background(), core.AuditEvent{Action: "target.created"}); err == nil {
		t.Fatal("Append(no resource type) error = nil, want error")
	}
}
