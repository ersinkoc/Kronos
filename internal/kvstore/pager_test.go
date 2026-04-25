package kvstore

import (
	"bytes"
	"testing"
)

func TestPagerAllocFreeReusesPages(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()

	first, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(first) error = %v", err)
	}
	second, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(second) error = %v", err)
	}
	if first == second || first == 0 || second == 0 {
		t.Fatalf("allocated page ids first=%d second=%d", first, second)
	}
	if pager.PageCount() != 3 {
		t.Fatalf("PageCount() = %d, want 3", pager.PageCount())
	}
	if err := pager.FreePage(first); err != nil {
		t.Fatalf("FreePage(first) error = %v", err)
	}
	if pager.FreeCount() != 1 || !pager.IsFree(first) {
		t.Fatalf("free list did not record page %d", first)
	}
	reused, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(reused) error = %v", err)
	}
	if reused != first {
		t.Fatalf("AllocPage(reused) = %d, want %d", reused, first)
	}
	if pager.FreeCount() != 0 {
		t.Fatalf("FreeCount() = %d, want 0", pager.FreeCount())
	}
}

func TestPagerPersistsFreeListAcrossReopen(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	pager, err := OpenPager(path)
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	ids := make([]PageID, 0, 4)
	for i := 0; i < 4; i++ {
		id, err := pager.AllocPage()
		if err != nil {
			t.Fatalf("AllocPage(%d) error = %v", i, err)
		}
		ids = append(ids, id)
	}
	if err := pager.FreePage(ids[1]); err != nil {
		t.Fatalf("FreePage(ids[1]) error = %v", err)
	}
	if err := pager.FreePage(ids[3]); err != nil {
		t.Fatalf("FreePage(ids[3]) error = %v", err)
	}
	if err := pager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := OpenPager(path)
	if err != nil {
		t.Fatalf("OpenPager(reopen) error = %v", err)
	}
	defer reopened.Close()
	if reopened.PageCount() != 5 {
		t.Fatalf("PageCount() = %d, want 5", reopened.PageCount())
	}
	if reopened.FreeCount() != 2 || !reopened.IsFree(ids[1]) || !reopened.IsFree(ids[3]) {
		t.Fatalf("free list not preserved: count=%d", reopened.FreeCount())
	}
	first, err := reopened.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(first reused) error = %v", err)
	}
	second, err := reopened.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage(second reused) error = %v", err)
	}
	if first != ids[3] || second != ids[1] {
		t.Fatalf("reused pages first=%d second=%d, want %d then %d", first, second, ids[3], ids[1])
	}
}

func TestPagerPageDataRoundTrip(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/kronos.db"
	pager, err := OpenPager(path)
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	id, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage() error = %v", err)
	}
	page, err := pager.Page(id)
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	copy(page[:16], []byte("hello from page"))
	if err := pager.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := OpenPager(path)
	if err != nil {
		t.Fatalf("OpenPager(reopen) error = %v", err)
	}
	defer reopened.Close()
	got, err := reopened.Page(id)
	if err != nil {
		t.Fatalf("Page(reopen) error = %v", err)
	}
	if !bytes.Equal(got[:16], []byte("hello from page\x00")) {
		t.Fatalf("page prefix = %q", got[:16])
	}
}

func TestPagerRejectsInvalidFree(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()
	if err := pager.FreePage(0); err == nil {
		t.Fatal("FreePage(0) error = nil, want error")
	}
	id, err := pager.AllocPage()
	if err != nil {
		t.Fatalf("AllocPage() error = %v", err)
	}
	if err := pager.FreePage(id); err != nil {
		t.Fatalf("FreePage(id) error = %v", err)
	}
	if err := pager.FreePage(id); err == nil {
		t.Fatal("FreePage(double) error = nil, want error")
	}
}

func TestPagerAllocFreeManyNoLeak(t *testing.T) {
	t.Parallel()

	pager, err := OpenPager(t.TempDir() + "/kronos.db")
	if err != nil {
		t.Fatalf("OpenPager() error = %v", err)
	}
	defer pager.Close()

	const count = 1000
	ids := make([]PageID, 0, count)
	for i := 0; i < count; i++ {
		id, err := pager.AllocPage()
		if err != nil {
			t.Fatalf("AllocPage(%d) error = %v", i, err)
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if err := pager.FreePage(id); err != nil {
			t.Fatalf("FreePage(%d) error = %v", id, err)
		}
	}
	if pager.FreeCount() != count {
		t.Fatalf("FreeCount() = %d, want %d", pager.FreeCount(), count)
	}
	pageCount := pager.PageCount()
	for i := 0; i < count; i++ {
		if _, err := pager.AllocPage(); err != nil {
			t.Fatalf("AllocPage(reuse %d) error = %v", i, err)
		}
	}
	if pager.PageCount() != pageCount || pager.FreeCount() != 0 {
		t.Fatalf("page leak: count before=%d after=%d free=%d", pageCount, pager.PageCount(), pager.FreeCount())
	}
}
