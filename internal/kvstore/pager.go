package kvstore

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"syscall"
)

const (
	// PageSize is the fixed on-disk page size used by the KV store.
	PageSize = 4096

	pagerMagic   = "KRKV"
	pagerVersion = uint16(1)
	metaPageID   = PageID(0)
)

// PageID identifies one 4 KiB page in the database file.
type PageID uint64

// Pager manages fixed-size mmap-backed pages and a persistent free list.
type Pager struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	data    []byte
	meta    pagerMeta
	freeSet map[PageID]struct{}
	closed  bool
}

type pagerMeta struct {
	PageCount uint64
	FreeHead  PageID
	FreeCount uint64
	RootPage  PageID
}

// OpenPager opens or creates a page file at path.
func OpenPager(path string) (*Pager, error) {
	if path == "" {
		return nil, fmt.Errorf("pager path is required")
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}

	p := &Pager{file: file, path: path, freeSet: make(map[PageID]struct{})}
	if err := p.open(); err != nil {
		file.Close()
		return nil, err
	}
	return p, nil
}

func (p *Pager) open() error {
	info, err := p.file.Stat()
	if err != nil {
		return err
	}
	size := info.Size()
	if size == 0 {
		if err := p.file.Truncate(PageSize); err != nil {
			return err
		}
		if err := p.mapFile(); err != nil {
			return err
		}
		p.meta = pagerMeta{PageCount: 1}
		return p.writeMeta()
	}
	if size%PageSize != 0 {
		return fmt.Errorf("invalid pager file size %d", size)
	}
	if err := p.mapFile(); err != nil {
		return err
	}
	if err := p.readMeta(); err != nil {
		p.unmap()
		return err
	}
	if p.meta.PageCount == 0 || p.meta.PageCount > uint64(size/PageSize) {
		p.unmap()
		return fmt.Errorf("invalid page count %d for size %d", p.meta.PageCount, size)
	}
	return p.rebuildFreeSet()
}

// Close flushes metadata, unmaps the file, and closes the file descriptor.
func (p *Pager) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	var first error
	if err := p.writeMeta(); err != nil {
		first = err
	}
	if err := p.flushMapping(); first == nil && err != nil {
		first = err
	}
	if err := p.file.Sync(); first == nil && err != nil {
		first = err
	}
	if err := p.unmap(); first == nil && err != nil {
		first = err
	}
	if err := p.file.Close(); first == nil && err != nil {
		first = err
	}
	p.closed = true
	return first
}

// Flush persists pager metadata and asks the OS to flush the file.
func (p *Pager) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureOpen(); err != nil {
		return err
	}
	if err := p.writeMeta(); err != nil {
		return err
	}
	if err := p.flushMapping(); err != nil {
		return err
	}
	return p.file.Sync()
}

// AllocPage returns a zeroed free page, reusing the free list before growing.
func (p *Pager) AllocPage() (PageID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureOpen(); err != nil {
		return 0, err
	}
	if p.meta.FreeHead != 0 {
		id := p.meta.FreeHead
		page, err := p.pageLocked(id)
		if err != nil {
			return 0, err
		}
		next := PageID(binary.LittleEndian.Uint64(page[:8]))
		delete(p.freeSet, id)
		p.meta.FreeHead = next
		p.meta.FreeCount--
		zero(page)
		if err := p.writeMeta(); err != nil {
			return 0, err
		}
		return id, nil
	}

	id := PageID(p.meta.PageCount)
	p.meta.PageCount++
	if err := p.resize(int64(p.meta.PageCount) * PageSize); err != nil {
		p.meta.PageCount--
		return 0, err
	}
	page, err := p.pageLocked(id)
	if err != nil {
		return 0, err
	}
	zero(page)
	if err := p.writeMeta(); err != nil {
		return 0, err
	}
	return id, nil
}

// FreePage returns id to the persistent free list.
func (p *Pager) FreePage(id PageID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureOpen(); err != nil {
		return err
	}
	if id == metaPageID {
		return fmt.Errorf("cannot free meta page")
	}
	if uint64(id) >= p.meta.PageCount {
		return fmt.Errorf("page %d out of range", id)
	}
	if _, ok := p.freeSet[id]; ok {
		return fmt.Errorf("page %d already free", id)
	}
	page, err := p.pageLocked(id)
	if err != nil {
		return err
	}
	zero(page)
	binary.LittleEndian.PutUint64(page[:8], uint64(p.meta.FreeHead))
	p.meta.FreeHead = id
	p.meta.FreeCount++
	p.freeSet[id] = struct{}{}
	return p.writeMeta()
}

// Page returns the mmap-backed bytes for id.
func (p *Pager) Page(id PageID) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureOpen(); err != nil {
		return nil, err
	}
	return p.pageLocked(id)
}

// PageCount returns the total number of pages including page zero.
func (p *Pager) PageCount() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.meta.PageCount
}

// FreeCount returns the number of pages currently in the free list.
func (p *Pager) FreeCount() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.meta.FreeCount
}

// IsFree reports whether id is currently on the free list.
func (p *Pager) IsFree(id PageID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.freeSet[id]
	return ok
}

// RootPage returns the persisted B+Tree root page id, if any.
func (p *Pager) RootPage() PageID {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.meta.RootPage
}

// SetRootPage stores the B+Tree root page id in pager metadata.
func (p *Pager) SetRootPage(id PageID) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.ensureOpen(); err != nil {
		return err
	}
	if id != 0 && uint64(id) >= p.meta.PageCount {
		return fmt.Errorf("root page %d out of range", id)
	}
	p.meta.RootPage = id
	return p.writeMeta()
}

func (p *Pager) readMeta() error {
	if len(p.data) < PageSize {
		return fmt.Errorf("pager mapping too small")
	}
	page := p.data[:PageSize]
	if string(page[:4]) != pagerMagic {
		return fmt.Errorf("invalid pager magic")
	}
	version := binary.LittleEndian.Uint16(page[4:6])
	if version != pagerVersion {
		return fmt.Errorf("unsupported pager version %d", version)
	}
	pageSize := binary.LittleEndian.Uint16(page[6:8])
	if pageSize != PageSize {
		return fmt.Errorf("unsupported page size %d", pageSize)
	}
	p.meta = pagerMeta{
		PageCount: binary.LittleEndian.Uint64(page[8:16]),
		FreeHead:  PageID(binary.LittleEndian.Uint64(page[16:24])),
		FreeCount: binary.LittleEndian.Uint64(page[24:32]),
		RootPage:  PageID(binary.LittleEndian.Uint64(page[32:40])),
	}
	return nil
}

func (p *Pager) writeMeta() error {
	if len(p.data) < PageSize {
		return fmt.Errorf("pager mapping too small")
	}
	page := p.data[:PageSize]
	zero(page)
	copy(page[:4], pagerMagic)
	binary.LittleEndian.PutUint16(page[4:6], pagerVersion)
	binary.LittleEndian.PutUint16(page[6:8], PageSize)
	binary.LittleEndian.PutUint64(page[8:16], p.meta.PageCount)
	binary.LittleEndian.PutUint64(page[16:24], uint64(p.meta.FreeHead))
	binary.LittleEndian.PutUint64(page[24:32], p.meta.FreeCount)
	binary.LittleEndian.PutUint64(page[32:40], uint64(p.meta.RootPage))
	return nil
}

func (p *Pager) rebuildFreeSet() error {
	p.freeSet = make(map[PageID]struct{}, p.meta.FreeCount)
	id := p.meta.FreeHead
	for seen := uint64(0); id != 0; seen++ {
		if seen >= p.meta.FreeCount {
			return fmt.Errorf("free list cycle or count mismatch")
		}
		if uint64(id) >= p.meta.PageCount {
			return fmt.Errorf("free page %d out of range", id)
		}
		if _, ok := p.freeSet[id]; ok {
			return fmt.Errorf("free list cycle at page %d", id)
		}
		p.freeSet[id] = struct{}{}
		page, err := p.pageLocked(id)
		if err != nil {
			return err
		}
		id = PageID(binary.LittleEndian.Uint64(page[:8]))
	}
	if uint64(len(p.freeSet)) != p.meta.FreeCount {
		return fmt.Errorf("free list count mismatch: got %d want %d", len(p.freeSet), p.meta.FreeCount)
	}
	return nil
}

func (p *Pager) pageLocked(id PageID) ([]byte, error) {
	if uint64(id) >= p.meta.PageCount {
		return nil, fmt.Errorf("page %d out of range", id)
	}
	off := int(id) * PageSize
	return p.data[off : off+PageSize], nil
}

func (p *Pager) resize(size int64) error {
	if err := p.unmap(); err != nil {
		return err
	}
	if err := p.file.Truncate(size); err != nil {
		return err
	}
	return p.mapFile()
}

func (p *Pager) mapFile() error {
	info, err := p.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("cannot mmap empty pager file")
	}
	data, err := syscall.Mmap(int(p.file.Fd()), 0, int(info.Size()), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}
	p.data = data
	return nil
}

func (p *Pager) unmap() error {
	if p.data == nil {
		return nil
	}
	err := syscall.Munmap(p.data)
	p.data = nil
	return err
}

func (p *Pager) flushMapping() error {
	if p.data == nil {
		return nil
	}
	written, err := p.file.WriteAt(p.data, 0)
	if err != nil {
		return err
	}
	if written != len(p.data) {
		return fmt.Errorf("short pager flush: wrote %d of %d", written, len(p.data))
	}
	return nil
}

func (p *Pager) ensureOpen() error {
	if p.closed {
		return fmt.Errorf("pager is closed")
	}
	if p.file == nil || p.data == nil {
		return fmt.Errorf("pager is not open")
	}
	return nil
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
