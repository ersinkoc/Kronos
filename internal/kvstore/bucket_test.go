package kvstore

import "testing"

func TestBucketCRUD(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		targets, err := tx.Bucket([]byte("targets"))
		if err != nil {
			return err
		}
		if err := targets.Put([]byte("prod"), []byte("postgres")); err != nil {
			return err
		}
		nested, err := targets.Bucket([]byte("metadata"))
		if err != nil {
			return err
		}
		return nested.Put([]byte("prod"), []byte("critical"))
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	err = db.View(func(tx *Tx) error {
		targets, err := tx.Bucket([]byte("targets"))
		if err != nil {
			return err
		}
		value, ok, err := targets.Get([]byte("prod"))
		if err != nil {
			return err
		}
		if !ok || string(value) != "postgres" {
			t.Fatalf("targets/prod = %q, %v", value, ok)
		}
		nested, err := targets.Bucket([]byte("metadata"))
		if err != nil {
			return err
		}
		value, ok, err = nested.Get([]byte("prod"))
		if err != nil {
			return err
		}
		if !ok || string(value) != "critical" {
			t.Fatalf("targets/metadata/prod = %q, %v", value, ok)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
}

func TestBucketScan(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		a, err := tx.Bucket([]byte("a"))
		if err != nil {
			return err
		}
		b, err := tx.Bucket([]byte("b"))
		if err != nil {
			return err
		}
		if err := a.Put([]byte("one"), []byte("1")); err != nil {
			return err
		}
		if err := a.Put([]byte("two"), []byte("2")); err != nil {
			return err
		}
		return b.Put([]byte("one"), []byte("other"))
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	err = db.View(func(tx *Tx) error {
		a, err := tx.Bucket([]byte("a"))
		if err != nil {
			return err
		}
		it, err := a.Scan([]byte("one"), nil)
		if err != nil {
			return err
		}
		var keys []string
		for it.Valid() {
			keys = append(keys, string(it.Key()))
			it.Next()
		}
		if err := it.Err(); err != nil {
			return err
		}
		if len(keys) != 2 || keys[0] != "one" || keys[1] != "two" {
			t.Fatalf("bucket scan keys = %v", keys)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("View() error = %v", err)
	}
}

func TestBucketDeleteAndIteratorValue(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		bucket, err := tx.Bucket([]byte("jobs"))
		if err != nil {
			return err
		}
		if err := bucket.Put([]byte("one"), []byte("queued")); err != nil {
			return err
		}
		if err := bucket.Put([]byte("two"), []byte("done")); err != nil {
			return err
		}
		return bucket.Delete([]byte("one"))
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if err := db.View(func(tx *Tx) error {
		bucket, err := tx.Bucket([]byte("jobs"))
		if err != nil {
			return err
		}
		if value, ok, err := bucket.Get([]byte("one")); err != nil || ok || value != nil {
			t.Fatalf("Get(deleted) = %q, %v, %v; want nil, false, nil", value, ok, err)
		}
		it, err := bucket.Scan([]byte("two"), nil)
		if err != nil {
			return err
		}
		if !it.Valid() || string(it.Key()) != "two" || string(it.Value()) != "done" {
			t.Fatalf("iterator key/value = %q/%q valid=%v", it.Key(), it.Value(), it.Valid())
		}
		it.Next()
		if it.Valid() || it.Key() != nil || it.Value() != nil {
			t.Fatalf("iterator after Next key/value = %q/%q valid=%v", it.Key(), it.Value(), it.Valid())
		}
		it.Next()
		return it.Err()
	}); err != nil {
		t.Fatalf("View() error = %v", err)
	}
}

func TestBucketRejectsInvalidNamesAndKeys(t *testing.T) {
	t.Parallel()

	db, err := Open(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Update(func(tx *Tx) error {
		if _, err := tx.Bucket(nil); err == nil {
			t.Fatal("Bucket(nil) error = nil, want error")
		}
		if _, err := tx.Bucket([]byte{'a', 0, 'b'}); err == nil {
			t.Fatal("Bucket(name with separator) error = nil, want error")
		}
		bucket, err := tx.Bucket([]byte("valid"))
		if err != nil {
			return err
		}
		if _, err := bucket.Bucket(nil); err == nil {
			t.Fatal("nested Bucket(nil) error = nil, want error")
		}
		if err := bucket.Put(nil, []byte("value")); err == nil {
			t.Fatal("Put(nil key) error = nil, want error")
		}
		if err := bucket.Delete([]byte{'a', 0, 'b'}); err == nil {
			t.Fatal("Delete(key with separator) error = nil, want error")
		}
		return nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}
