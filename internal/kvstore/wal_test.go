package kvstore

import (
	"os"
	"testing"
)

func TestRollbackWALRecoversPreviousImage(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Put([]byte("stable"), []byte("value")); err != nil {
		t.Fatalf("Put(stable) error = %v", err)
	}
	if err := writeRollbackWAL(path); err != nil {
		t.Fatalf("writeRollbackWAL() error = %v", err)
	}
	if err := db.tree.Put([]byte("partial"), []byte("dirty")); err != nil {
		t.Fatalf("tree.Put(partial) error = %v", err)
	}
	if err := db.pager.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := db.pager.Close(); err != nil {
		t.Fatalf("pager.Close() error = %v", err)
	}
	db.closed = true

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(recover) error = %v", err)
	}
	defer reopened.Close()
	value, ok, err := reopened.Get([]byte("stable"))
	if err != nil {
		t.Fatalf("Get(stable) error = %v", err)
	}
	if !ok || string(value) != "value" {
		t.Fatalf("Get(stable) = %q, %v", value, ok)
	}
	if _, ok, err := reopened.Get([]byte("partial")); err != nil || ok {
		t.Fatalf("Get(partial) ok=%v err=%v, want missing after rollback", ok, err)
	}
	if _, err := os.Stat(rollbackWALPath(path)); !os.IsNotExist(err) {
		t.Fatalf("wal still exists after recovery: %v", err)
	}
}
