package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestRetentionPolicyStoreSaveListDelete(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewRetentionPolicyStore(db)
	if err != nil {
		t.Fatalf("NewRetentionPolicyStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	for _, policy := range []core.RetentionPolicy{
		{ID: "policy-b", Name: "weekly", Rules: []core.RetentionRule{{Kind: "count", Params: map[string]any{"n": 7}}}, CreatedAt: now, UpdatedAt: now},
		{ID: "policy-a", Name: "daily", Rules: []core.RetentionRule{{Kind: "time", Params: map[string]any{"days": 14}}}, CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.Save(policy); err != nil {
			t.Fatalf("Save(%s) error = %v", policy.ID, err)
		}
	}
	policies, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(policies) != 2 || policies[0].ID != "policy-a" || policies[1].ID != "policy-b" {
		t.Fatalf("policies = %#v", policies)
	}
	got, ok, err := store.Get("policy-a")
	if err != nil || !ok || got.Name != "daily" {
		t.Fatalf("Get(policy-a) = %#v ok=%v err=%v", got, ok, err)
	}
	if err := store.Delete("policy-a"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("policy-a"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want false nil", ok, err)
	}
}

func TestRetentionPolicyStoreValidation(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewRetentionPolicyStore(db)
	if err != nil {
		t.Fatalf("NewRetentionPolicyStore() error = %v", err)
	}
	if err := store.Save(core.RetentionPolicy{Name: "daily", Rules: []core.RetentionRule{{Kind: "count"}}}); err == nil {
		t.Fatal("Save(no id) error = nil, want error")
	}
	if err := store.Save(core.RetentionPolicy{ID: "policy", Rules: []core.RetentionRule{{Kind: "count"}}}); err == nil {
		t.Fatal("Save(no name) error = nil, want error")
	}
	if err := store.Save(core.RetentionPolicy{ID: "policy", Name: "daily"}); err == nil {
		t.Fatal("Save(no rules) error = nil, want error")
	}
	if err := store.Save(core.RetentionPolicy{ID: "policy", Name: "daily", Rules: []core.RetentionRule{{}}}); err == nil {
		t.Fatal("Save(no rule kind) error = nil, want error")
	}
}
