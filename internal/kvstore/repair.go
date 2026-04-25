package kvstore

import "fmt"

// Repair rebuilds the free list from pages reachable through the root tree.
func Repair(path string) error {
	if err := recoverRollbackWAL(path); err != nil {
		return err
	}
	pager, err := OpenPager(path)
	if err != nil {
		return err
	}
	defer pager.Close()

	pager.mu.Lock()
	defer pager.mu.Unlock()

	if err := pager.ensureOpen(); err != nil {
		return err
	}
	reachable := map[PageID]struct{}{metaPageID: {}}
	if pager.meta.RootPage != 0 {
		if err := collectReachablePages(pager, pager.meta.RootPage, reachable); err != nil {
			return err
		}
	}
	if err := pager.rebuildFreeListFromReachable(reachable); err != nil {
		return err
	}
	if err := pager.writeMeta(); err != nil {
		return err
	}
	if err := pager.flushMapping(); err != nil {
		return err
	}
	return pager.file.Sync()
}

func collectReachablePages(pager *Pager, id PageID, reachable map[PageID]struct{}) error {
	if _, ok := reachable[id]; ok {
		return nil
	}
	if uint64(id) >= pager.meta.PageCount {
		return fmt.Errorf("reachable page %d out of range", id)
	}
	page, err := pager.pageLocked(id)
	if err != nil {
		return err
	}
	node, err := decodeNode(id, page)
	if err != nil {
		return err
	}
	reachable[id] = struct{}{}
	if node.typ == nodeLeaf {
		if node.next != 0 {
			return collectReachablePages(pager, node.next, reachable)
		}
		return nil
	}
	for _, entry := range node.entries {
		if err := collectReachablePages(pager, entry.child, reachable); err != nil {
			return err
		}
	}
	return nil
}

func (p *Pager) rebuildFreeListFromReachable(reachable map[PageID]struct{}) error {
	p.freeSet = make(map[PageID]struct{})
	p.meta.FreeHead = 0
	p.meta.FreeCount = 0
	for id := PageID(p.meta.PageCount - 1); id > 0; id-- {
		if _, ok := reachable[id]; ok {
			continue
		}
		page, err := p.pageLocked(id)
		if err != nil {
			return err
		}
		zero(page)
		putFreeListNext(page, p.meta.FreeHead)
		p.meta.FreeHead = id
		p.meta.FreeCount++
		p.freeSet[id] = struct{}{}
	}
	return nil
}

func putFreeListNext(page []byte, next PageID) {
	for i := 0; i < 8; i++ {
		page[i] = byte(uint64(next) >> (8 * i))
	}
}
