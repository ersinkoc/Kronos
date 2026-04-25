package kvstore

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	nodeHeaderSize = 16
	nodeEntrySize  = 16

	nodeLeaf   = byte(1)
	nodeBranch = byte(2)
)

// BTree is a read/write tree rooted at one pager page.
type BTree struct {
	pager *Pager
	root  PageID
}

// NewBTree returns a B+Tree view rooted at root.
func NewBTree(pager *Pager, root PageID) *BTree {
	return &BTree{pager: pager, root: root}
}

// CreateBTree allocates an empty root leaf and returns a tree for it.
func CreateBTree(pager *Pager) (*BTree, error) {
	if pager == nil {
		return nil, fmt.Errorf("pager is required")
	}
	root, err := pager.AllocPage()
	if err != nil {
		return nil, err
	}
	tree := NewBTree(pager, root)
	if err := tree.writeNode(btreeNode{id: root, typ: nodeLeaf}); err != nil {
		return nil, err
	}
	return tree, nil
}

// Root returns the root page id.
func (t *BTree) Root() PageID {
	if t == nil {
		return 0
	}
	return t.root
}

// Get returns a copy of the value for key.
func (t *BTree) Get(key []byte) ([]byte, bool, error) {
	if t == nil || t.pager == nil {
		return nil, false, fmt.Errorf("btree pager is required")
	}
	if len(key) == 0 {
		return nil, false, fmt.Errorf("key is required")
	}
	return t.getAt(t.root, key)
}

// Put inserts or replaces key with value.
func (t *BTree) Put(key []byte, value []byte) error {
	if t == nil || t.pager == nil {
		return fmt.Errorf("btree pager is required")
	}
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	root, err := t.readNode(t.root)
	if err != nil {
		return err
	}
	switch root.typ {
	case nodeLeaf:
		updated := insertEntry(root.entries, btreeEntry{
			key:   append([]byte(nil), key...),
			value: append([]byte(nil), value...),
		})
		root.entries = updated
		if nodeFits(root) {
			return t.writeNode(root)
		}
		return t.splitRootLeaf(root)
	case nodeBranch:
		return t.putRootBranch(root, key, value)
	default:
		return fmt.Errorf("unsupported node type %d", root.typ)
	}
}

// Delete removes key. Missing keys are ignored.
func (t *BTree) Delete(key []byte) error {
	if t == nil || t.pager == nil {
		return fmt.Errorf("btree pager is required")
	}
	if len(key) == 0 {
		return fmt.Errorf("key is required")
	}
	root, err := t.readNode(t.root)
	if err != nil {
		return err
	}
	switch root.typ {
	case nodeLeaf:
		next, ok := deleteEntry(root.entries, key)
		if !ok {
			return nil
		}
		root.entries = next
		return t.writeNode(root)
	case nodeBranch:
		deleted, err := t.deleteFromBranch(root, key, true)
		if err != nil {
			return err
		}
		if !deleted {
			return nil
		}
		root, err = t.readNode(root.id)
		if err != nil {
			return err
		}
		if len(root.entries) == 1 {
			only, err := t.readNode(root.entries[0].child)
			if err != nil {
				return err
			}
			only.id = t.root
			return t.writeNode(only)
		}
		return t.writeNode(root)
	default:
		return fmt.Errorf("unsupported node type %d", root.typ)
	}
}

func (t *BTree) getAt(pageID PageID, key []byte) ([]byte, bool, error) {
	node, err := t.readNode(pageID)
	if err != nil {
		return nil, false, err
	}
	switch node.typ {
	case nodeLeaf:
		idx, ok := node.search(key)
		if !ok {
			return nil, false, nil
		}
		return append([]byte(nil), node.entries[idx].value...), true, nil
	case nodeBranch:
		child, err := node.childFor(key)
		if err != nil {
			return nil, false, err
		}
		return t.getAt(child, key)
	default:
		return nil, false, fmt.Errorf("unsupported node type %d", node.typ)
	}
}

func (t *BTree) putRootBranch(root btreeNode, key []byte, value []byte) error {
	childIndex := root.childIndexFor(key)
	child, err := t.readNode(root.entries[childIndex].child)
	if err != nil {
		return err
	}
	split, err := t.putNonRoot(child, key, value)
	if err != nil {
		return err
	}
	child, err = t.readNode(root.entries[childIndex].child)
	if err != nil {
		return err
	}
	root.entries[childIndex].key = cloneBytes(child.firstKey())
	if split != nil {
		root.entries = insertEntry(root.entries, *split)
	}
	if nodeFits(root) {
		return t.writeNode(root)
	}
	return t.splitRootBranch(root)
}

func (t *BTree) putNonRoot(node btreeNode, key []byte, value []byte) (*btreeEntry, error) {
	switch node.typ {
	case nodeLeaf:
		node.entries = insertEntry(node.entries, btreeEntry{
			key:   append([]byte(nil), key...),
			value: append([]byte(nil), value...),
		})
		if nodeFits(node) {
			return nil, t.writeNode(node)
		}
		return t.splitNonRoot(node)
	case nodeBranch:
		childIndex := node.childIndexFor(key)
		child, err := t.readNode(node.entries[childIndex].child)
		if err != nil {
			return nil, err
		}
		split, err := t.putNonRoot(child, key, value)
		if err != nil {
			return nil, err
		}
		child, err = t.readNode(node.entries[childIndex].child)
		if err != nil {
			return nil, err
		}
		node.entries[childIndex].key = cloneBytes(child.firstKey())
		if split != nil {
			node.entries = insertEntry(node.entries, *split)
		}
		if nodeFits(node) {
			return nil, t.writeNode(node)
		}
		return t.splitNonRoot(node)
	default:
		return nil, fmt.Errorf("unsupported node type %d", node.typ)
	}
}

func (t *BTree) splitNonRoot(node btreeNode) (*btreeEntry, error) {
	leftEntries, rightEntries := splitEntries(node.entries)
	node.entries = leftEntries
	rightID, err := t.pager.AllocPage()
	if err != nil {
		return nil, err
	}
	right := btreeNode{id: rightID, typ: node.typ, next: node.next, entries: rightEntries}
	if node.typ == nodeLeaf {
		node.next = rightID
	} else {
		right.next = 0
		node.next = 0
	}
	if err := t.writeNode(node); err != nil {
		return nil, err
	}
	if err := t.writeNode(right); err != nil {
		return nil, err
	}
	return &btreeEntry{key: cloneBytes(right.firstKey()), child: rightID}, nil
}

func (t *BTree) splitRootBranch(root btreeNode) error {
	leftEntries, rightEntries := splitEntries(root.entries)
	leftID, err := t.pager.AllocPage()
	if err != nil {
		return err
	}
	rightID, err := t.pager.AllocPage()
	if err != nil {
		return err
	}
	left := btreeNode{id: leftID, typ: nodeBranch, entries: leftEntries}
	right := btreeNode{id: rightID, typ: nodeBranch, entries: rightEntries}
	newRoot := btreeNode{
		id:  root.id,
		typ: nodeBranch,
		entries: []btreeEntry{
			{key: cloneBytes(left.firstKey()), child: leftID},
			{key: cloneBytes(right.firstKey()), child: rightID},
		},
	}
	if err := t.writeNode(left); err != nil {
		return err
	}
	if err := t.writeNode(right); err != nil {
		return err
	}
	return t.writeNode(newRoot)
}

func (t *BTree) splitRootLeaf(root btreeNode) error {
	leftEntries, rightEntries := splitEntries(root.entries)
	leftID, err := t.pager.AllocPage()
	if err != nil {
		return err
	}
	rightID, err := t.pager.AllocPage()
	if err != nil {
		return err
	}
	left := btreeNode{id: leftID, typ: nodeLeaf, next: rightID, entries: leftEntries}
	right := btreeNode{id: rightID, typ: nodeLeaf, entries: rightEntries}
	branch := btreeNode{
		id:  root.id,
		typ: nodeBranch,
		entries: []btreeEntry{
			{key: cloneBytes(left.firstKey()), child: leftID},
			{key: cloneBytes(right.firstKey()), child: rightID},
		},
	}
	if err := t.writeNode(left); err != nil {
		return err
	}
	if err := t.writeNode(right); err != nil {
		return err
	}
	return t.writeNode(branch)
}

func (t *BTree) deleteFromBranch(node btreeNode, key []byte, isRoot bool) (bool, error) {
	if len(node.entries) == 0 {
		return false, fmt.Errorf("branch node %d has no entries", node.id)
	}
	childIndex := node.childIndexFor(key)
	child, err := t.readNode(node.entries[childIndex].child)
	if err != nil {
		return false, err
	}

	var deleted bool
	switch child.typ {
	case nodeLeaf:
		next, ok := deleteEntry(child.entries, key)
		if !ok {
			return false, nil
		}
		deleted = true
		child.entries = next
	case nodeBranch:
		deleted, err = t.deleteFromBranch(child, key, false)
		if err != nil || !deleted {
			return deleted, err
		}
		child, err = t.readNode(child.id)
		if err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unsupported node type %d", child.typ)
	}

	if len(child.entries) == 0 {
		node.entries = append(node.entries[:childIndex], node.entries[childIndex+1:]...)
		if !isRoot {
			if err := t.pager.FreePage(child.id); err != nil {
				return false, err
			}
		}
	} else {
		if err := t.writeNode(child); err != nil {
			return false, err
		}
		node.entries[childIndex].key = cloneBytes(child.firstKey())
	}
	if err := t.writeNode(node); err != nil {
		return false, err
	}
	return deleted, nil
}

// Iterator scans key/value pairs in ascending key order.
type Iterator struct {
	tree  *BTree
	node  btreeNode
	index int
	end   []byte
	err   error
}

// Scan returns an iterator over [start, end). A nil end scans to the tree end.
func (t *BTree) Scan(start []byte, end []byte) (*Iterator, error) {
	if t == nil || t.pager == nil {
		return nil, fmt.Errorf("btree pager is required")
	}
	if end != nil && bytes.Compare(start, end) >= 0 {
		return &Iterator{}, nil
	}
	leaf, err := t.firstLeafAt(t.root, start)
	if err != nil {
		return nil, err
	}
	idx := leaf.lowerBound(start)
	it := &Iterator{
		tree:  t,
		node:  leaf,
		index: idx,
		end:   append([]byte(nil), end...),
	}
	it.skipEmpty()
	return it, nil
}

// Valid reports whether the iterator points at a key/value pair.
func (it *Iterator) Valid() bool {
	return it != nil && it.err == nil && it.node.typ == nodeLeaf && it.index < len(it.node.entries) && !it.pastEnd()
}

// Key returns a copy of the current key.
func (it *Iterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	return append([]byte(nil), it.node.entries[it.index].key...)
}

// Value returns a copy of the current value.
func (it *Iterator) Value() []byte {
	if !it.Valid() {
		return nil
	}
	return append([]byte(nil), it.node.entries[it.index].value...)
}

// Next advances the iterator.
func (it *Iterator) Next() {
	if !it.Valid() {
		return
	}
	it.index++
	it.skipEmpty()
}

// Err returns the first iterator error.
func (it *Iterator) Err() error {
	if it == nil {
		return nil
	}
	return it.err
}

func (it *Iterator) skipEmpty() {
	for it.err == nil && it.node.typ == nodeLeaf && it.index >= len(it.node.entries) && it.node.next != 0 {
		next, err := it.tree.readNode(it.node.next)
		if err != nil {
			it.err = err
			return
		}
		it.node = next
		it.index = 0
	}
}

func (it *Iterator) pastEnd() bool {
	return it.end != nil && bytes.Compare(it.node.entries[it.index].key, it.end) >= 0
}

func (t *BTree) firstLeafAt(pageID PageID, key []byte) (btreeNode, error) {
	node, err := t.readNode(pageID)
	if err != nil {
		return btreeNode{}, err
	}
	switch node.typ {
	case nodeLeaf:
		return node, nil
	case nodeBranch:
		child, err := node.childFor(key)
		if err != nil {
			return btreeNode{}, err
		}
		return t.firstLeafAt(child, key)
	default:
		return btreeNode{}, fmt.Errorf("unsupported node type %d", node.typ)
	}
}

func (t *BTree) readNode(pageID PageID) (btreeNode, error) {
	page, err := t.pager.Page(pageID)
	if err != nil {
		return btreeNode{}, err
	}
	return decodeNode(pageID, page)
}

type btreeNode struct {
	id      PageID
	typ     byte
	next    PageID
	entries []btreeEntry
}

type btreeEntry struct {
	key   []byte
	value []byte
	child PageID
}

func (n btreeNode) search(key []byte) (int, bool) {
	idx := n.lowerBound(key)
	if idx >= len(n.entries) || !bytes.Equal(n.entries[idx].key, key) {
		return 0, false
	}
	return idx, true
}

func (n btreeNode) lowerBound(key []byte) int {
	lo, hi := 0, len(n.entries)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if bytes.Compare(n.entries[mid].key, key) < 0 {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (n btreeNode) childFor(key []byte) (PageID, error) {
	if n.typ != nodeBranch {
		return 0, fmt.Errorf("node %d is not a branch", n.id)
	}
	if len(n.entries) == 0 {
		return 0, fmt.Errorf("branch node %d has no entries", n.id)
	}
	idx := n.lowerBound(key)
	if idx >= len(n.entries) {
		idx = len(n.entries) - 1
	}
	if bytes.Compare(key, n.entries[idx].key) < 0 && idx > 0 {
		idx--
	}
	return n.entries[idx].child, nil
}

func (n btreeNode) childIndexFor(key []byte) int {
	idx := n.lowerBound(key)
	if idx >= len(n.entries) {
		return len(n.entries) - 1
	}
	if bytes.Compare(key, n.entries[idx].key) < 0 && idx > 0 {
		return idx - 1
	}
	return idx
}

func (n btreeNode) firstKey() []byte {
	if len(n.entries) == 0 {
		return nil
	}
	return n.entries[0].key
}

func decodeNode(id PageID, page []byte) (btreeNode, error) {
	if len(page) != PageSize {
		return btreeNode{}, fmt.Errorf("page %d has size %d", id, len(page))
	}
	typ := page[0]
	if typ != nodeLeaf && typ != nodeBranch {
		return btreeNode{}, fmt.Errorf("page %d has invalid node type %d", id, typ)
	}
	count := int(binary.LittleEndian.Uint16(page[2:4]))
	next := PageID(binary.LittleEndian.Uint64(page[8:16]))
	if nodeHeaderSize+count*nodeEntrySize > PageSize {
		return btreeNode{}, fmt.Errorf("page %d entry directory overflow", id)
	}
	node := btreeNode{
		id:      id,
		typ:     typ,
		next:    next,
		entries: make([]btreeEntry, 0, count),
	}
	for i := 0; i < count; i++ {
		base := nodeHeaderSize + i*nodeEntrySize
		keyOff := int(binary.LittleEndian.Uint16(page[base : base+2]))
		keyLen := int(binary.LittleEndian.Uint16(page[base+2 : base+4]))
		valOff := int(binary.LittleEndian.Uint16(page[base+4 : base+6]))
		valLen := int(binary.LittleEndian.Uint16(page[base+6 : base+8]))
		child := PageID(binary.LittleEndian.Uint64(page[base+8 : base+16]))
		if keyLen == 0 {
			return btreeNode{}, fmt.Errorf("page %d entry %d has empty key", id, i)
		}
		if keyOff < 0 || keyLen < 0 || keyOff+keyLen > PageSize {
			return btreeNode{}, fmt.Errorf("page %d entry %d key out of range", id, i)
		}
		if valOff < 0 || valLen < 0 || valOff+valLen > PageSize {
			return btreeNode{}, fmt.Errorf("page %d entry %d value out of range", id, i)
		}
		entry := btreeEntry{
			key:   append([]byte(nil), page[keyOff:keyOff+keyLen]...),
			value: append([]byte(nil), page[valOff:valOff+valLen]...),
			child: child,
		}
		if i > 0 && bytes.Compare(node.entries[i-1].key, entry.key) >= 0 {
			return btreeNode{}, fmt.Errorf("page %d keys out of order", id)
		}
		if typ == nodeBranch && child == 0 {
			return btreeNode{}, fmt.Errorf("branch page %d entry %d has no child", id, i)
		}
		node.entries = append(node.entries, entry)
	}
	return node, nil
}

func (t *BTree) writeNode(node btreeNode) error {
	page, err := t.pager.Page(node.id)
	if err != nil {
		return err
	}
	if !nodeFits(node) {
		return fmt.Errorf("node %d does not fit in one page", node.id)
	}
	zero(page)
	page[0] = node.typ
	binary.LittleEndian.PutUint16(page[2:4], uint16(len(node.entries)))
	binary.LittleEndian.PutUint64(page[8:16], uint64(node.next))
	cursor := PageSize
	for i, entry := range node.entries {
		if len(entry.key) == 0 {
			return fmt.Errorf("node %d entry %d has empty key", node.id, i)
		}
		cursor -= len(entry.value)
		valueOff := cursor
		copy(page[valueOff:valueOff+len(entry.value)], entry.value)
		cursor -= len(entry.key)
		keyOff := cursor
		copy(page[keyOff:keyOff+len(entry.key)], entry.key)
		base := nodeHeaderSize + i*nodeEntrySize
		binary.LittleEndian.PutUint16(page[base:base+2], uint16(keyOff))
		binary.LittleEndian.PutUint16(page[base+2:base+4], uint16(len(entry.key)))
		binary.LittleEndian.PutUint16(page[base+4:base+6], uint16(valueOff))
		binary.LittleEndian.PutUint16(page[base+6:base+8], uint16(len(entry.value)))
		binary.LittleEndian.PutUint64(page[base+8:base+16], uint64(entry.child))
	}
	return nil
}

func nodeFits(node btreeNode) bool {
	used := nodeHeaderSize + len(node.entries)*nodeEntrySize
	for _, entry := range node.entries {
		used += len(entry.key) + len(entry.value)
	}
	return used <= PageSize
}

func insertEntry(entries []btreeEntry, entry btreeEntry) []btreeEntry {
	idx := btreeNode{entries: entries}.lowerBound(entry.key)
	if idx < len(entries) && bytes.Equal(entries[idx].key, entry.key) {
		entries[idx] = entry
		return entries
	}
	entries = append(entries, btreeEntry{})
	copy(entries[idx+1:], entries[idx:])
	entries[idx] = entry
	return entries
}

func deleteEntry(entries []btreeEntry, key []byte) ([]btreeEntry, bool) {
	idx, ok := btreeNode{entries: entries}.search(key)
	if !ok {
		return entries, false
	}
	copy(entries[idx:], entries[idx+1:])
	entries[len(entries)-1] = btreeEntry{}
	return entries[:len(entries)-1], true
}

func splitEntries(entries []btreeEntry) ([]btreeEntry, []btreeEntry) {
	mid := len(entries) / 2
	if mid == 0 {
		mid = 1
	}
	left := make([]btreeEntry, mid)
	right := make([]btreeEntry, len(entries)-mid)
	copy(left, entries[:mid])
	copy(right, entries[mid:])
	return left, right
}

func cloneBytes(data []byte) []byte {
	if data == nil {
		return nil
	}
	return append([]byte(nil), data...)
}
