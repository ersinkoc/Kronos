package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestTokenStoreCreateListVerifyRevoke(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	clock := core.NewFakeClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	store, err := NewTokenStore(db, clock)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	created, err := store.Create("ci", "user-1", []string{"backup:write", "backup:read", "backup:read"}, clock.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Secret == "" || created.Token.ID.IsZero() || len(created.Token.Scopes) != 2 {
		t.Fatalf("created = %#v", created)
	}
	if parseID, ok := parseTokenID(created.Secret); !ok || parseID != created.Token.ID {
		t.Fatalf("parseTokenID() = %q ok=%v, want %q", parseID, ok, created.Token.ID)
	}

	tokens, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tokens) != 1 || tokens[0].ID != created.Token.ID || tokens[0].Name != "ci" {
		t.Fatalf("tokens = %#v", tokens)
	}
	got, ok, err := store.Get(created.Token.ID)
	if err != nil || !ok || got.ID != created.Token.ID {
		t.Fatalf("Get() token=%#v ok=%v err=%v", got, ok, err)
	}
	token, ok, err := store.Verify(created.Secret)
	if err != nil || !ok || token.ID != created.Token.ID {
		t.Fatalf("Verify() token=%#v ok=%v err=%v", token, ok, err)
	}
	if _, ok, err := store.Verify(created.Secret + "x"); err != nil || ok {
		t.Fatalf("Verify(bad) ok=%v err=%v, want false nil", ok, err)
	}

	revoked, err := store.Revoke(created.Token.ID)
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.RevokedAt.IsZero() {
		t.Fatalf("revoked = %#v", revoked)
	}
	if _, ok, err := store.Verify(created.Secret); err != nil || ok {
		t.Fatalf("Verify(revoked) ok=%v err=%v, want false nil", ok, err)
	}
}

func TestTokenStoreRejectsInvalidCreate(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	clock := core.NewFakeClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	store, err := NewTokenStore(db, clock)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	if _, err := store.Create("", "user-1", []string{"backup:read"}, time.Time{}); err == nil {
		t.Fatal("Create(no name) error = nil, want error")
	}
	if _, err := store.Create("ci", "", []string{"backup:read"}, time.Time{}); err == nil {
		t.Fatal("Create(no user) error = nil, want error")
	}
	if _, err := store.Create("ci", "user-1", nil, time.Time{}); err == nil {
		t.Fatal("Create(no scopes) error = nil, want error")
	}
	if _, err := store.Create("ci", "user-1", []string{"backup:read"}, clock.Now().Add(-time.Second)); err == nil {
		t.Fatal("Create(expired) error = nil, want error")
	}
}

func TestTokenStorePruneInactive(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	clock := core.NewFakeClock(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	store, err := NewTokenStore(db, clock)
	if err != nil {
		t.Fatalf("NewTokenStore() error = %v", err)
	}
	active, err := store.Create("active", "user-1", []string{"backup:read"}, clock.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Create(active) error = %v", err)
	}
	revoked, err := store.Create("revoked", "user-1", []string{"backup:read"}, time.Time{})
	if err != nil {
		t.Fatalf("Create(revoked) error = %v", err)
	}
	if _, err := store.Revoke(revoked.Token.ID); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	expiring, err := store.Create("expiring", "user-1", []string{"backup:read"}, clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Create(expiring) error = %v", err)
	}
	clock.Advance(2 * time.Minute)

	inactive, err := store.Inactive()
	if err != nil {
		t.Fatalf("Inactive() error = %v", err)
	}
	if len(inactive) != 2 {
		t.Fatalf("inactive = %#v, want 2 tokens", inactive)
	}
	if _, ok, err := store.Get(expiring.Token.ID); err != nil || !ok {
		t.Fatalf("Get(expiring before prune) ok=%v err=%v, want present", ok, err)
	}

	deleted, err := store.PruneInactive()
	if err != nil {
		t.Fatalf("PruneInactive() error = %v", err)
	}
	if len(deleted) != 2 {
		t.Fatalf("deleted = %#v, want 2 tokens", deleted)
	}
	if _, ok, err := store.Get(active.Token.ID); err != nil || !ok {
		t.Fatalf("Get(active) ok=%v err=%v, want active token", ok, err)
	}
	if _, ok, err := store.Get(revoked.Token.ID); err != nil || ok {
		t.Fatalf("Get(revoked) ok=%v err=%v, want pruned", ok, err)
	}
	if _, ok, err := store.Get(expiring.Token.ID); err != nil || ok {
		t.Fatalf("Get(expiring) ok=%v err=%v, want pruned", ok, err)
	}
}
