package kvstore

import (
	"fmt"
	"testing"
)

func BenchmarkKVRandomWrite(b *testing.B) {
	db, err := Open(b.TempDir() + "/kronos.db")
	if err != nil {
		b.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key-%09d", permuteBenchIndex(i)))
		value := []byte(fmt.Sprintf("value-%09d", i))
		if err := db.Put(key, value); err != nil {
			b.Fatalf("Put() error = %v", err)
		}
	}
}

func BenchmarkKVRandomRead(b *testing.B) {
	db, err := Open(b.TempDir() + "/kronos.db")
	if err != nil {
		b.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	const count = 10000
	for i := 0; i < count; i++ {
		key := []byte(fmt.Sprintf("key-%09d", i))
		value := []byte(fmt.Sprintf("value-%09d", i))
		if err := db.Put(key, value); err != nil {
			b.Fatalf("Put(seed) error = %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("key-%09d", permuteBenchIndex(i)%count))
		if _, ok, err := db.Get(key); err != nil || !ok {
			b.Fatalf("Get() ok=%v err=%v", ok, err)
		}
	}
}

func BenchmarkKVScan(b *testing.B) {
	db, err := Open(b.TempDir() + "/kronos.db")
	if err != nil {
		b.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	const count = 10000
	for i := 0; i < count; i++ {
		key := []byte(fmt.Sprintf("key-%09d", i))
		value := []byte(fmt.Sprintf("value-%09d", i))
		if err := db.Put(key, value); err != nil {
			b.Fatalf("Put(seed) error = %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.View(func(tx *Tx) error {
			it, err := tx.Scan([]byte("key-000000000"), []byte("key-000010000"))
			if err != nil {
				return err
			}
			for it.Valid() {
				it.Next()
			}
			return it.Err()
		})
		if err != nil {
			b.Fatalf("Scan() error = %v", err)
		}
	}
}

func permuteBenchIndex(i int) int {
	return (i * 2654435761) & 0x7fffffff
}
