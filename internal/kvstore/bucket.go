package kvstore

import (
	"bytes"
	"fmt"
)

var bucketSeparator = []byte{0}

// Bucket is a namespaced view over DB keys.
type Bucket struct {
	tx     *Tx
	prefix []byte
}

// Bucket returns a top-level bucket view.
func (tx *Tx) Bucket(name []byte) (*Bucket, error) {
	if tx == nil || tx.db == nil {
		return nil, fmt.Errorf("transaction is closed")
	}
	if len(name) == 0 || bytes.Contains(name, bucketSeparator) {
		return nil, fmt.Errorf("invalid bucket name")
	}
	return &Bucket{tx: tx, prefix: bucketPrefix(nil, name)}, nil
}

// Bucket returns a nested bucket view.
func (b *Bucket) Bucket(name []byte) (*Bucket, error) {
	if b == nil || b.tx == nil {
		return nil, fmt.Errorf("bucket is closed")
	}
	if len(name) == 0 || bytes.Contains(name, bucketSeparator) {
		return nil, fmt.Errorf("invalid bucket name")
	}
	return &Bucket{tx: b.tx, prefix: bucketPrefix(b.prefix, name)}, nil
}

// Get returns key's value in the bucket.
func (b *Bucket) Get(key []byte) ([]byte, bool, error) {
	full, err := b.key(key)
	if err != nil {
		return nil, false, err
	}
	return b.tx.Get(full)
}

// Put inserts or replaces key in the bucket.
func (b *Bucket) Put(key []byte, value []byte) error {
	full, err := b.key(key)
	if err != nil {
		return err
	}
	return b.tx.Put(full, value)
}

// Delete removes key from the bucket.
func (b *Bucket) Delete(key []byte) error {
	full, err := b.key(key)
	if err != nil {
		return err
	}
	return b.tx.Delete(full)
}

// Scan returns an iterator scoped to the bucket.
func (b *Bucket) Scan(start []byte, end []byte) (*BucketIterator, error) {
	startKey, err := b.key(start)
	if err != nil {
		return nil, err
	}
	var endKey []byte
	if end == nil {
		endKey = prefixEnd(b.prefix)
	} else {
		endKey, err = b.key(end)
		if err != nil {
			return nil, err
		}
	}
	it, err := b.tx.Scan(startKey, endKey)
	if err != nil {
		return nil, err
	}
	return &BucketIterator{bucket: b, it: it}, nil
}

func (b *Bucket) key(key []byte) ([]byte, error) {
	if b == nil || b.tx == nil {
		return nil, fmt.Errorf("bucket is closed")
	}
	if len(key) == 0 || bytes.Contains(key, bucketSeparator) {
		return nil, fmt.Errorf("invalid bucket key")
	}
	full := make([]byte, 0, len(b.prefix)+len(key))
	full = append(full, b.prefix...)
	full = append(full, key...)
	return full, nil
}

// BucketIterator unwraps keys from a bucket-scoped scan.
type BucketIterator struct {
	bucket *Bucket
	it     *Iterator
}

// Valid reports whether the iterator points at a key/value pair.
func (it *BucketIterator) Valid() bool {
	return it != nil && it.it != nil && it.it.Valid() && bytes.HasPrefix(it.it.Key(), it.bucket.prefix)
}

// Key returns the bucket-local key.
func (it *BucketIterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	key := it.it.Key()
	return append([]byte(nil), key[len(it.bucket.prefix):]...)
}

// Value returns a copy of the current value.
func (it *BucketIterator) Value() []byte {
	if !it.Valid() {
		return nil
	}
	return it.it.Value()
}

// Next advances the iterator.
func (it *BucketIterator) Next() {
	if it != nil && it.it != nil {
		it.it.Next()
	}
}

// Err returns the first iterator error.
func (it *BucketIterator) Err() error {
	if it == nil || it.it == nil {
		return nil
	}
	return it.it.Err()
}

func bucketPrefix(parent []byte, name []byte) []byte {
	prefix := make([]byte, 0, len(parent)+len(name)+1)
	prefix = append(prefix, parent...)
	prefix = append(prefix, name...)
	prefix = append(prefix, bucketSeparator...)
	return prefix
}

func prefixEnd(prefix []byte) []byte {
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}
