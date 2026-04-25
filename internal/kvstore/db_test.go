package kvstore

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestDBPutGetReopenDelete(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	for i := 0; i < 120; i++ {
		key := []byte(fmt.Sprintf("key-%03d", i))
		value := []byte(fmt.Sprintf("value-%03d", i))
		if err := db.Put(key, value); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer reopened.Close()
	value, ok, err := reopened.Get([]byte("key-077"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || string(value) != "value-077" {
		t.Fatalf("Get(key-077) = %q, %v", value, ok)
	}
	if err := reopened.Delete([]byte("key-077")); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok, err := reopened.Get([]byte("key-077")); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestDBConcurrentReadersSingleWriter(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := db.Put([]byte("key"), []byte("value")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				value, ok, err := db.Get([]byte("key"))
				if err != nil {
					t.Errorf("Get() error = %v", err)
					return
				}
				if !ok || string(value) != "value" {
					t.Errorf("Get() = %q, %v", value, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestDBUpdateAndViewTransactions(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		if err := tx.Put([]byte("alpha"), []byte("1")); err != nil {
			return err
		}
		return tx.Put([]byte("bravo"), []byte("2"))
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	err = db.View(func(tx *Tx) error {
		value, ok, err := tx.Get([]byte("alpha"))
		if err != nil {
			return err
		}
		if !ok || string(value) != "1" {
			t.Fatalf("Get(alpha) = %q, %v", value, ok)
		}
		it, err := tx.Scan([]byte("alpha"), []byte("charlie"))
		if err != nil {
			return err
		}
		var count int
		for it.Valid() {
			count++
			it.Next()
		}
		if err := it.Err(); err != nil {
			return err
		}
		if count != 2 {
			t.Fatalf("scan count = %d, want 2", count)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
}

func TestDBViewRejectsWrites(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	err = db.View(func(tx *Tx) error {
		return tx.Put([]byte("alpha"), []byte("1"))
	})
	if err == nil {
		t.Fatal("View Put() error = nil, want read-only error")
	}
}

func TestDBUpdateRollsBackOnError(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := db.Put([]byte("stable"), []byte("value")); err != nil {
		t.Fatalf("Put(stable) error = %v", err)
	}

	want := errors.New("stop")
	err = db.Update(func(tx *Tx) error {
		if err := tx.Put([]byte("partial"), []byte("dirty")); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("Update() error = %v, want %v", err, want)
	}
	value, ok, err := db.Get([]byte("stable"))
	if err != nil {
		t.Fatalf("Get(stable) error = %v", err)
	}
	if !ok || string(value) != "value" {
		t.Fatalf("Get(stable) = %q, %v", value, ok)
	}
	if _, ok, err := db.Get([]byte("partial")); err != nil || ok {
		t.Fatalf("Get(partial) ok=%v err=%v, want rollback", ok, err)
	}
}
