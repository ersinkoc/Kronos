package core

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestDomainTypesJSONRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	id := ID("018f8f84-95c0-7000-8000-000000000000")

	cases := []struct {
		name  string
		value any
	}{
		{
			name: "target",
			value: Target{
				ID: id, Name: "prod-pg", Driver: TargetDriverPostgres, Endpoint: "127.0.0.1:5432",
				Database: "app", CreatedAt: now, UpdatedAt: now, Labels: map[string]string{"env": "prod"},
			},
		},
		{
			name: "storage",
			value: Storage{
				ID: id, Name: "local", Kind: StorageKindLocal, URI: "file:///var/lib/kronos",
				CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			name: "schedule",
			value: Schedule{
				ID: id, Name: "nightly", TargetID: id, StorageID: id, BackupType: BackupTypeFull,
				Expression: "0 2 * * *", CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			name: "job",
			value: Job{
				ID: id, TargetID: id, StorageID: id, Type: BackupTypeFull, Status: JobStatusQueued,
				QueuedAt: now,
			},
		},
		{
			name: "backup",
			value: Backup{
				ID: id, TargetID: id, StorageID: id, JobID: id, Type: BackupTypeFull, ManifestID: id,
				StartedAt: now, EndedAt: now, SizeBytes: 42, ChunkCount: 1,
			},
		},
		{
			name: "manifest",
			value: Manifest{
				ID: id, BackupID: id, Version: 1, ChunkHashes: []string{"abc"}, CreatedAt: now,
			},
		},
		{
			name: "retention policy",
			value: RetentionPolicy{
				ID: id, Name: "gfs", Rules: []RetentionRule{{Kind: "gfs", Params: map[string]any{"daily": float64(7)}}},
				CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			name: "user",
			value: User{
				ID: id, Email: "ops@example.com", DisplayName: "Ops", Role: RoleAdmin,
				TOTPEnforced: true, CreatedAt: now, UpdatedAt: now,
			},
		},
		{
			name:  "role",
			value: Role{Name: RoleOperator, Description: "Runs jobs", Scopes: []string{"backup:write"}},
		},
		{
			name: "token",
			value: Token{
				ID: id, UserID: id, Name: "ci", Scopes: []string{"backup:read"}, CreatedAt: now,
			},
		},
		{
			name: "audit event",
			value: AuditEvent{
				ID: id, ActorID: id, Action: "target.create", ResourceType: "target", ResourceID: id,
				OccurredAt: now, Hash: "hash",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			got := reflect.New(reflect.TypeOf(tc.value)).Interface()
			if err := json.Unmarshal(data, got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
		})
	}
}
