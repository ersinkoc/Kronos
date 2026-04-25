package kvstore

import "testing"

func TestRepairRebuildsFreeListFromReachablePages(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := db.Put([]byte("alpha"), []byte("1")); err != nil {
		t.Fatalf("Put(alpha) error = %v", err)
	}
	if err := db.Put([]byte("bravo"), []byte("2")); err != nil {
		t.Fatalf("Put(bravo) error = %v", err)
	}
	orphan, err := db.pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(orphan) error = %v", err)
	}
	page, err := db.pager.Page(orphan)
	if err != nil {
		t.Fatalf("Page(orphan) error = %v", err)
	}
	page[0] = 222
	if err := db.pager.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := Repair(path); err != nil {
		t.Fatalf("Repair() error = %v", err)
	}
	pager, err := OpenPager(path)
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	if !pager.IsFree(orphan) {
		t.Fatalf("orphan page %d was not returned to free list", orphan)
	}
}

func TestRepairRejectsCorruptReachablePage(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	root := db.tree.Root()
	page, err := db.pager.Page(root)
	if err != nil {
		t.Fatalf("Page(root) error = %v", err)
	}
	page[0] = 99
	if err := db.pager.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := Repair(path); err == nil {
		t.Fatal("Repair(corrupt root) error = nil, want error")
	}
}
