package kvstore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestBTreeGetFromLeaf(t *testing.T) {
	t.Parallel()

	pager, root := buildTestTree(t)
	defer pager.Close()

	tree := NewBTree(pager, root)
	value, ok, err := tree.Get([]byte("bravo"))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || !bytes.Equal(value, []byte("2")) {
		t.Fatalf("Get(bravo) = %q, %v; want 2, true", value, ok)
	}
	if _, ok, err := tree.Get([]byte("delta")); err != nil || ok {
		t.Fatalf("Get(delta) ok=%v err=%v, want missing", ok, err)
	}
}

func TestBTreeGetThroughBranch(t *testing.T) {
	t.Parallel()

	pager, root := buildTwoLeafTree(t)
	defer pager.Close()

	tree := NewBTree(pager, root)
	cases := map[string]string{
		"alpha":   "1",
		"charlie": "3",
		"echo":    "5",
	}
	for key, want := range cases {
		value, ok, err := tree.Get([]byte(key))
		if err != nil {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
		if !ok || string(value) != want {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", key, value, ok, want)
		}
	}
}

func TestBTreeScanRange(t *testing.T) {
	t.Parallel()

	pager, root := buildTwoLeafTree(t)
	defer pager.Close()

	tree := NewBTree(pager, root)
	it, err := tree.Scan([]byte("bravo"), []byte("foxtrot"))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	var got []string
	for it.Valid() {
		got = append(got, string(it.Key())+"="+string(it.Value()))
		it.Next()
	}
	if err := it.Err(); err != nil {
		t.Fatalf("iterator error = %v", err)
	}
	want := []string{"bravo=2", "charlie=3", "delta=4", "echo=5"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("scan = %v, want %v", got, want)
	}
}

func TestBTreePutGetDelete(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	tree, err := CreateBTree(pager)
	if err != nil {
		t.Fatalf("CreateBTree() error = %v", err)
	}

	if err := tree.Put([]byte("bravo"), []byte("2")); err != nil {
		t.Fatalf("Put(bravo) error = %v", err)
	}
	if err := tree.Put([]byte("alpha"), []byte("1")); err != nil {
		t.Fatalf("Put(alpha) error = %v", err)
	}
	if err := tree.Put([]byte("bravo"), []byte("two")); err != nil {
		t.Fatalf("Put(update) error = %v", err)
	}
	value, ok, err := tree.Get([]byte("bravo"))
	if err != nil {
		t.Fatalf("Get(bravo) error = %v", err)
	}
	if !ok || string(value) != "two" {
		t.Fatalf("Get(bravo) = %q, %v; want two, true", value, ok)
	}
	if err := tree.Delete([]byte("bravo")); err != nil {
		t.Fatalf("Delete(bravo) error = %v", err)
	}
	if _, ok, err := tree.Get([]byte("bravo")); err != nil || ok {
		t.Fatalf("Get(deleted) ok=%v err=%v, want missing", ok, err)
	}
}

func TestBTreePutSplitsRootLeaf(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	tree, err := CreateBTree(pager)
	if err != nil {
		t.Fatalf("CreateBTree() error = %v", err)
	}
	for i := 0; i < 180; i++ {
		key := []byte(fmt.Sprintf("key-%03d", i))
		value := []byte(fmt.Sprintf("value-%03d", i))
		if err := tree.Put(key, value); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}
	for _, i := range []int{0, 77, 179} {
		key := []byte(fmt.Sprintf("key-%03d", i))
		want := fmt.Sprintf("value-%03d", i)
		value, ok, err := tree.Get(key)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
		if !ok || string(value) != want {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", key, value, ok, want)
		}
	}
	it, err := tree.Scan([]byte("key-050"), []byte("key-055"))
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	var keys []string
	for it.Valid() {
		keys = append(keys, string(it.Key()))
		it.Next()
	}
	want := []string{"key-050", "key-051", "key-052", "key-053", "key-054"}
	if fmt.Sprint(keys) != fmt.Sprint(want) {
		t.Fatalf("scan keys = %v, want %v", keys, want)
	}
}

func TestBTreePutGrowsMultipleLevels(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	tree, err := CreateBTree(pager)
	if err != nil {
		t.Fatalf("CreateBTree() error = %v", err)
	}
	suffix := strings.Repeat("x", 90)
	for i := 0; i < 2600; i++ {
		key := []byte(fmt.Sprintf("key-%04d-%s", i, suffix))
		value := []byte(fmt.Sprintf("value-%04d", i))
		if err := tree.Put(key, value); err != nil {
			t.Fatalf("Put(%s) error = %v", key, err)
		}
	}
	for _, i := range []int{0, 777, 1599, 2599} {
		key := []byte(fmt.Sprintf("key-%04d-%s", i, suffix))
		want := fmt.Sprintf("value-%04d", i)
		value, ok, err := tree.Get(key)
		if err != nil {
			t.Fatalf("Get(%s) error = %v", key, err)
		}
		if !ok || string(value) != want {
			t.Fatalf("Get(%s) = %q, %v; want %q, true", key, value, ok, want)
		}
	}
	target := []byte(fmt.Sprintf("key-%04d-%s", 1599, suffix))
	if err := tree.Put(target, []byte("updated")); err != nil {
		t.Fatalf("Put(update) error = %v", err)
	}
	value, ok, err := tree.Get(target)
	if err != nil {
		t.Fatalf("Get(updated) error = %v", err)
	}
	if !ok || string(value) != "updated" {
		t.Fatalf("Get(updated) = %q, %v", value, ok)
	}
}

func TestBTreeDeleteThroughBranchAndCollapseRoot(t *testing.T) {
	t.Parallel()

	pager, root := buildTwoLeafTree(t)
	defer pager.Close()
	tree := NewBTree(pager, root)

	if err := tree.Delete([]byte("missing")); err != nil {
		t.Fatalf("Delete(missing) error = %v", err)
	}
	for _, key := range []string{"alpha", "bravo", "charlie"} {
		if err := tree.Delete([]byte(key)); err != nil {
			t.Fatalf("Delete(%s) error = %v", key, err)
		}
	}
	if _, ok, err := tree.Get([]byte("alpha")); err != nil || ok {
		t.Fatalf("Get(alpha) ok=%v err=%v, want missing", ok, err)
	}
	value, ok, err := tree.Get([]byte("delta"))
	if err != nil {
		t.Fatalf("Get(delta) error = %v", err)
	}
	if !ok || string(value) != "4" {
		t.Fatalf("Get(delta) = %q, %v; want 4, true", value, ok)
	}
	if err := tree.Delete([]byte("delta")); err != nil {
		t.Fatalf("Delete(delta) error = %v", err)
	}
	if value, ok, err := tree.Get([]byte("echo")); err != nil || !ok || string(value) != "5" {
		t.Fatalf("Get(echo) = %q, %v, %v; want 5, true, nil", value, ok, err)
	}
}

func TestBTreeRejectsInvalidOperations(t *testing.T) {
	t.Parallel()

	var tree *BTree
	if _, _, err := tree.Get([]byte("x")); err == nil {
		t.Fatal("nil Get() error = nil, want error")
	}
	if err := tree.Put([]byte("x"), []byte("y")); err == nil {
		t.Fatal("nil Put() error = nil, want error")
	}
	if err := tree.Delete([]byte("x")); err == nil {
		t.Fatal("nil Delete() error = nil, want error")
	}

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	tree, err = CreateBTree(pager)
	if err != nil {
		t.Fatalf("CreateBTree() error = %v", err)
	}
	if _, _, err := tree.Get(nil); err == nil {
		t.Fatal("Get(nil key) error = nil, want error")
	}
	if err := tree.Put(nil, []byte("value")); err == nil {
		t.Fatal("Put(nil key) error = nil, want error")
	}
	if err := tree.Delete(nil); err == nil {
		t.Fatal("Delete(nil key) error = nil, want error")
	}
}

func TestBTreeRejectsCorruptNode(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	root, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage() error = %v", err)
	}
	page, err := pager.Page(root)
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	page[0] = 99

	tree := NewBTree(pager, root)
	if _, _, err := tree.Get([]byte("x")); err == nil {
		t.Fatal("Get(corrupt) error = nil, want error")
	}
}

func buildTestTree(t *testing.T) (*Pager, PageID) {
	t.Helper()
	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	root, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage() error = %v", err)
	}
	writeLeaf(t, pager, root, 0, []kvPair{
		{key: "alpha", value: "1"},
		{key: "bravo", value: "2"},
		{key: "charlie", value: "3"},
	})
	return pager, root
}

func buildTwoLeafTree(t *testing.T) (*Pager, PageID) {
	t.Helper()
	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	left, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(left) error = %v", err)
	}
	right, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(right) error = %v", err)
	}
	root, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(root) error = %v", err)
	}
	writeLeaf(t, pager, left, right, []kvPair{
		{key: "alpha", value: "1"},
		{key: "bravo", value: "2"},
		{key: "charlie", value: "3"},
	})
	writeLeaf(t, pager, right, 0, []kvPair{
		{key: "delta", value: "4"},
		{key: "echo", value: "5"},
		{key: "foxtrot", value: "6"},
	})
	writeBranch(t, pager, root, []branchPair{
		{key: "alpha", child: left},
		{key: "delta", child: right},
	})
	return pager, root
}

type kvPair struct {
	key   string
	value string
}

type branchPair struct {
	key   string
	child PageID
}

func writeLeaf(t *testing.T, pager *Pager, id PageID, next PageID, pairs []kvPair) {
	t.Helper()
	page, err := pager.Page(id)
	if err != nil {
		t.Fatalf("Page(%d) error = %v", id, err)
	}
	zero(page)
	page[0] = nodeLeaf
	binary.LittleEndian.PutUint16(page[2:4], uint16(len(pairs)))
	binary.LittleEndian.PutUint64(page[8:16], uint64(next))
	cursor := PageSize
	for i, pair := range pairs {
		key := []byte(pair.key)
		value := []byte(pair.value)
		cursor -= len(value)
		copy(page[cursor:cursor+len(value)], value)
		valueOff := cursor
		cursor -= len(key)
		copy(page[cursor:cursor+len(key)], key)
		keyOff := cursor
		base := nodeHeaderSize + i*nodeEntrySize
		binary.LittleEndian.PutUint16(page[base:base+2], uint16(keyOff))
		binary.LittleEndian.PutUint16(page[base+2:base+4], uint16(len(key)))
		binary.LittleEndian.PutUint16(page[base+4:base+6], uint16(valueOff))
		binary.LittleEndian.PutUint16(page[base+6:base+8], uint16(len(value)))
	}
}

func writeBranch(t *testing.T, pager *Pager, id PageID, pairs []branchPair) {
	t.Helper()
	page, err := pager.Page(id)
	if err != nil {
		t.Fatalf("Page(%d) error = %v", id, err)
	}
	zero(page)
	page[0] = nodeBranch
	binary.LittleEndian.PutUint16(page[2:4], uint16(len(pairs)))
	cursor := PageSize
	for i, pair := range pairs {
		key := []byte(pair.key)
		cursor -= len(key)
		copy(page[cursor:cursor+len(key)], key)
		base := nodeHeaderSize + i*nodeEntrySize
		binary.LittleEndian.PutUint16(page[base:base+2], uint16(cursor))
		binary.LittleEndian.PutUint16(page[base+2:base+4], uint16(len(key)))
		binary.LittleEndian.PutUint64(page[base+8:base+16], uint64(pair.child))
	}
}
