package server

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
	"github.com/kronos/kronos/internal/kvstore"
)

func TestUserStoreSaveListGrantDelete(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewUserStore(db)
	if err != nil {
		t.Fatalf("NewUserStore() error = %v", err)
	}
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	if err := store.Save(core.User{ID: "user-2", Email: "viewer@example.com", DisplayName: "Viewer", Role: core.RoleViewer, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Save(user-2) error = %v", err)
	}
	if err := store.Save(core.User{ID: "user-1", Email: "admin@example.com", DisplayName: "Admin", Role: core.RoleAdmin, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Save(user-1) error = %v", err)
	}
	users, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(users) != 2 || users[0].ID != "user-1" || users[1].ID != "user-2" {
		t.Fatalf("users = %#v", users)
	}
	granted, err := store.Grant("user-2", core.RoleOperator, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	if granted.Role != core.RoleOperator || !granted.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("granted = %#v", granted)
	}
	if err := store.Delete("user-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := store.Get("user-1"); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want false nil", ok, err)
	}
}

func TestUserStoreValidation(t *testing.T) {
	t.Parallel()

	db, err := kvstore.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	store, err := NewUserStore(db)
	if err != nil {
		t.Fatalf("NewUserStore() error = %v", err)
	}
	if err := store.Save(core.User{Email: "a@example.com", DisplayName: "A", Role: core.RoleAdmin}); err == nil {
		t.Fatal("Save(no id) error = nil, want error")
	}
	if err := store.Save(core.User{ID: "user-1", DisplayName: "A", Role: core.RoleAdmin}); err == nil {
		t.Fatal("Save(no email) error = nil, want error")
	}
	if err := store.Save(core.User{ID: "user-1", Email: "a@example.com", Role: core.RoleAdmin}); err == nil {
		t.Fatal("Save(no display name) error = nil, want error")
	}
	if err := store.Save(core.User{ID: "user-1", Email: "a@example.com", DisplayName: "A", Role: "root"}); err == nil {
		t.Fatal("Save(bad role) error = nil, want error")
	}
	if _, err := store.Grant("missing", core.RoleViewer, time.Time{}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Grant(missing) error = %v, want ErrNotFound", err)
	}
}
