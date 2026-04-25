package audit

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
	"lukechampine.com/blake3"
)

var (
	eventsBucket = []byte("audit_events")
	metaBucket   = []byte("audit_meta")
	seqKey       = []byte("last_seq")
	hashKey      = []byte("last_hash")
)

// Log appends and verifies tamper-evident audit events.
type Log struct {
	db    *kvstore.DB
	clock core.Clock
}

// New returns an audit log backed by db.
func New(db *kvstore.DB, clock core.Clock) (*Log, error) {
	if db == nil {
		return nil, fmt.Errorf("kv database is required")
	}
	if clock == nil {
		clock = core.RealClock{}
	}
	return &Log{db: db, clock: clock}, nil
}

// Append stores event at the end of the hash chain.
func (l *Log) Append(ctx context.Context, event core.AuditEvent) (core.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return core.AuditEvent{}, err
	}
	if event.Action == "" {
		return core.AuditEvent{}, fmt.Errorf("audit action is required")
	}
	if event.ResourceType == "" {
		return core.AuditEvent{}, fmt.Errorf("audit resource type is required")
	}
	now := event.OccurredAt
	if now.IsZero() {
		now = l.clock.Now().UTC()
	}
	if event.ID.IsZero() {
		id, err := core.NewID(l.clock)
		if err != nil {
			return core.AuditEvent{}, err
		}
		event.ID = id
	}
	event.OccurredAt = now.UTC()

	err := l.db.Update(func(tx *kvstore.Tx) error {
		meta, err := tx.Bucket(metaBucket)
		if err != nil {
			return err
		}
		events, err := tx.Bucket(eventsBucket)
		if err != nil {
			return err
		}
		lastSeq, err := readSeq(meta)
		if err != nil {
			return err
		}
		lastHash, err := readHash(meta)
		if err != nil {
			return err
		}
		event.Seq = lastSeq + 1
		event.PrevHash = lastHash
		hash, err := hashEvent(event)
		if err != nil {
			return err
		}
		event.Hash = hash
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if err := events.Put(seqKeyBytes(event.Seq), data); err != nil {
			return err
		}
		if err := meta.Put(seqKey, seqBytes(event.Seq)); err != nil {
			return err
		}
		return meta.Put(hashKey, []byte(event.Hash))
	})
	if err != nil {
		return core.AuditEvent{}, err
	}
	return event, nil
}

// List returns audit events in append order.
func (l *Log) List(ctx context.Context, limit int) ([]core.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var events []core.AuditEvent
	err := l.db.View(func(tx *kvstore.Tx) error {
		bucket, err := tx.Bucket(eventsBucket)
		if err != nil {
			return err
		}
		it, err := bucket.Scan([]byte{1}, nil)
		if err != nil {
			return err
		}
		for it.Valid() {
			var event core.AuditEvent
			if err := json.Unmarshal(it.Value(), &event); err != nil {
				return err
			}
			events = append(events, event)
			if limit > 0 && len(events) >= limit {
				break
			}
			it.Next()
		}
		return it.Err()
	})
	return events, err
}

// Verify checks sequence continuity and every hash-chain link.
func (l *Log) Verify(ctx context.Context) error {
	events, err := l.List(ctx, 0)
	if err != nil {
		return err
	}
	var previousHash string
	for i, event := range events {
		wantSeq := uint64(i + 1)
		if event.Seq != wantSeq {
			return fmt.Errorf("audit sequence %d: got %d", wantSeq, event.Seq)
		}
		if event.PrevHash != previousHash {
			return fmt.Errorf("audit sequence %d: previous hash mismatch", event.Seq)
		}
		hash, err := hashEvent(event)
		if err != nil {
			return err
		}
		if event.Hash != hash {
			return fmt.Errorf("audit sequence %d: hash mismatch", event.Seq)
		}
		previousHash = event.Hash
	}
	return nil
}

func readSeq(bucket *kvstore.Bucket) (uint64, error) {
	data, ok, err := bucket.Get(seqKey)
	if err != nil || !ok {
		return 0, err
	}
	if len(data) != 8 {
		return 0, fmt.Errorf("invalid audit sequence metadata")
	}
	return binary.BigEndian.Uint64(data), nil
}

func readHash(bucket *kvstore.Bucket) (string, error) {
	data, ok, err := bucket.Get(hashKey)
	if err != nil || !ok {
		return "", err
	}
	return string(data), nil
}

func hashEvent(event core.AuditEvent) (string, error) {
	event.Hash = ""
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := blake3.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func seqBytes(seq uint64) []byte {
	var out [8]byte
	binary.BigEndian.PutUint64(out[:], seq)
	return append([]byte(nil), out[:]...)
}

func seqKeyBytes(seq uint64) []byte {
	return []byte(fmt.Sprintf("%020d", seq))
}
