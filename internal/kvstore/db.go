package kvstore

import (
	"fmt"
	"sync"
)

// DB is the durable single-writer facade over the pager and root B+Tree.
type DB struct {
	mu     sync.RWMutex
	pager  *Pager
	tree   *BTree
	closed bool
}

// Tx is a database transaction.
type Tx struct {
	db       *DB
	writable bool
}

// Open opens or creates a KV database at path.
func Open(path string) (*DB, error) {
	if err := recoverRollbackWAL(path); err != nil {
		return nil, err
	}
	pager, err := OpenPager(path)
	if err != nil {
		return nil, err
	}
	root := pager.RootPage()
	var tree *BTree
	if root == 0 {
		tree, err = CreateBTree(pager)
		if err != nil {
			pager.Close()
			return nil, err
		}
		if err := pager.SetRootPage(tree.Root()); err != nil {
			pager.Close()
			return nil, err
		}
		if err := pager.Flush(); err != nil {
			pager.Close()
			return nil, err
		}
	} else {
		tree = NewBTree(pager, root)
	}
	return &DB{pager: pager, tree: tree}, nil
}

// Close flushes and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true
	return db.pager.Close()
}

// Get returns a copy of key's value.
func (db *DB) Get(key []byte) ([]byte, bool, error) {
	var value []byte
	var ok bool
	err := db.View(func(tx *Tx) error {
		var err error
		value, ok, err = tx.Get(key)
		return err
	})
	return value, ok, err
}

// Put inserts or replaces key.
func (db *DB) Put(key []byte, value []byte) error {
	return db.Update(func(tx *Tx) error {
		return tx.Put(key, value)
	})
}

// Delete removes key.
func (db *DB) Delete(key []byte) error {
	return db.Update(func(tx *Tx) error {
		return tx.Delete(key)
	})
}

// View runs fn inside a read-only transaction.
func (db *DB) View(fn func(*Tx) error) error {
	if fn == nil {
		return fmt.Errorf("transaction function is required")
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	if err := db.ensureOpen(); err != nil {
		return err
	}
	return fn(&Tx{db: db})
}

// Update runs fn inside a single-writer transaction.
func (db *DB) Update(fn func(*Tx) error) error {
	if fn == nil {
		return fmt.Errorf("transaction function is required")
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := db.ensureOpen(); err != nil {
		return err
	}
	if err := writeRollbackWAL(db.pager.path); err != nil {
		return err
	}
	tx := &Tx{db: db, writable: true}
	if err := fn(tx); err != nil {
		if rollbackErr := db.rollbackLocked(); rollbackErr != nil {
			return fmt.Errorf("rollback transaction after error %v: %w", err, rollbackErr)
		}
		return err
	}
	if err := db.pager.Flush(); err != nil {
		return err
	}
	return removeRollbackWAL(db.pager.path)
}

func (db *DB) rollbackLocked() error {
	path := db.pager.path
	if err := db.pager.Close(); err != nil {
		return err
	}
	if err := recoverRollbackWAL(path); err != nil {
		return err
	}
	pager, err := OpenPager(path)
	if err != nil {
		return err
	}
	root := pager.RootPage()
	if root == 0 {
		pager.Close()
		return fmt.Errorf("database root is missing after rollback")
	}
	db.pager = pager
	db.tree = NewBTree(pager, root)
	db.closed = false
	return nil
}

// Get returns a copy of key's value.
func (tx *Tx) Get(key []byte) ([]byte, bool, error) {
	if tx == nil || tx.db == nil {
		return nil, false, fmt.Errorf("transaction is closed")
	}
	return tx.db.tree.Get(key)
}

// Put inserts or replaces key in a writable transaction.
func (tx *Tx) Put(key []byte, value []byte) error {
	if tx == nil || tx.db == nil {
		return fmt.Errorf("transaction is closed")
	}
	if !tx.writable {
		return fmt.Errorf("transaction is read-only")
	}
	return tx.db.tree.Put(key, value)
}

// Delete removes key in a writable transaction.
func (tx *Tx) Delete(key []byte) error {
	if tx == nil || tx.db == nil {
		return fmt.Errorf("transaction is closed")
	}
	if !tx.writable {
		return fmt.Errorf("transaction is read-only")
	}
	return tx.db.tree.Delete(key)
}

// Scan returns a range iterator for the transaction.
func (tx *Tx) Scan(start []byte, end []byte) (*Iterator, error) {
	if tx == nil || tx.db == nil {
		return nil, fmt.Errorf("transaction is closed")
	}
	return tx.db.tree.Scan(start, end)
}

func (db *DB) ensureOpen() error {
	if db == nil || db.pager == nil || db.tree == nil {
		return fmt.Errorf("database is not open")
	}
	if db.closed {
		return fmt.Errorf("database is closed")
	}
	return nil
}
