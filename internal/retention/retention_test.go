package retention

import (
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
)

func TestResolveCountKeepsNewestOfType(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("full-old", core.BackupTypeFull, now.Add(-4*time.Hour), 10, false),
		backup("incr-new", core.BackupTypeIncremental, now.Add(-time.Hour), 10, false),
		backup("full-new", core.BackupTypeFull, now.Add(-2*time.Hour), 10, false),
		backup("full-mid", core.BackupTypeFull, now.Add(-3*time.Hour), 10, false),
	}
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{{
		Kind:   "count",
		Params: map[string]any{"n": 2, "type": "full"},
	}}}

	plan, err := Resolve(backups, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "full-new", "full-mid")
	assertDrop(t, plan, "incr-new", "full-old")
}

func TestResolveTimeAndProtected(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("new", core.BackupTypeFull, now.Add(-30*time.Minute), 10, false),
		backup("old", core.BackupTypeFull, now.Add(-72*time.Hour), 10, false),
		backup("legal-hold", core.BackupTypeFull, now.Add(-720*time.Hour), 10, true),
	}
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{{
		Kind:   "time",
		Params: map[string]any{"duration": "24h"},
	}}}

	plan, err := Resolve(backups, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "new", "legal-hold")
	assertDrop(t, plan, "old")
}

func TestResolveSizeKeepsNewestWithinCap(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("new", core.BackupTypeFull, now.Add(-time.Hour), 70, false),
		backup("mid", core.BackupTypeFull, now.Add(-2*time.Hour), 40, false),
		backup("old", core.BackupTypeFull, now.Add(-3*time.Hour), 10, false),
	}
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{{
		Kind:   "size",
		Params: map[string]any{"max_bytes": 80},
	}}}

	plan, err := Resolve(backups, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "new", "old")
	assertDrop(t, plan, "mid")
}

func TestResolveGFS(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("today", core.BackupTypeFull, time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC), 10, false),
		backup("yesterday", core.BackupTypeFull, time.Date(2026, 4, 24, 2, 0, 0, 0, time.UTC), 10, false),
		backup("same-week", core.BackupTypeFull, time.Date(2026, 4, 23, 2, 0, 0, 0, time.UTC), 10, false),
		backup("last-week", core.BackupTypeFull, time.Date(2026, 4, 18, 2, 0, 0, 0, time.UTC), 10, false),
		backup("last-month", core.BackupTypeFull, time.Date(2026, 3, 20, 2, 0, 0, 0, time.UTC), 10, false),
		backup("last-year", core.BackupTypeFull, time.Date(2025, 12, 20, 2, 0, 0, 0, time.UTC), 10, false),
	}
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{{
		Kind:   "gfs",
		Params: map[string]any{"daily": 2, "weekly": 2, "monthly": 2, "yearly": 2},
	}}}

	plan, err := Resolve(backups, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "today", "yesterday", "last-week", "last-month", "last-year")
	assertDrop(t, plan, "same-week")
}

func TestResolveCombinesRulesIdempotently(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("new", core.BackupTypeFull, now.Add(-time.Hour), 10, false),
		backup("mid", core.BackupTypeFull, now.Add(-48*time.Hour), 10, false),
		backup("old", core.BackupTypeFull, now.Add(-96*time.Hour), 10, false),
	}
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{
		{Kind: "count", Params: map[string]any{"n": 1}},
		{Kind: "time", Params: map[string]any{"duration": "72h"}},
		{Kind: "count", Params: map[string]any{"n": 1}},
	}}

	plan, err := Resolve(backups, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "new", "mid")
	assertDrop(t, plan, "old")
	for _, item := range plan.Items {
		if item.Backup.ID == "new" && len(item.Reasons) != 2 {
			t.Fatalf("new reasons = %v, want count and time without duplicate count", item.Reasons)
		}
	}
}

func TestResolveKeepsAncestorsForKeptDescendants(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	full := backup("full", core.BackupTypeFull, now.Add(-3*time.Hour), 10, false)
	incr1 := backup("incr-1", core.BackupTypeIncremental, now.Add(-2*time.Hour), 10, false)
	incr1.ParentID = full.ID
	incr2 := backup("incr-2", core.BackupTypeIncremental, now.Add(-time.Hour), 10, false)
	incr2.ParentID = incr1.ID
	unrelated := backup("unrelated", core.BackupTypeFull, now.Add(-4*time.Hour), 10, false)
	policy := core.RetentionPolicy{Rules: []core.RetentionRule{{
		Kind:   "count",
		Params: map[string]any{"n": 1},
	}}}

	plan, err := Resolve([]core.Backup{unrelated, full, incr1, incr2}, policy, now)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	assertKeep(t, plan, "incr-2", "incr-1", "full")
	assertDrop(t, plan, "unrelated")
	for _, item := range plan.Items {
		if item.Backup.ID == "full" || item.Backup.ID == "incr-1" {
			if !hasReason(item.Reasons, "chain") {
				t.Fatalf("%s reasons = %v, want chain", item.Backup.ID, item.Reasons)
			}
		}
	}
}

func TestResolveRejectsUnknownRule(t *testing.T) {
	t.Parallel()

	_, err := Resolve(nil, core.RetentionPolicy{Rules: []core.RetentionRule{{Kind: "mystery"}}}, time.Now())
	if err == nil {
		t.Fatal("Resolve() error = nil, want unknown rule error")
	}
}

func backup(id string, typ core.BackupType, endedAt time.Time, size int64, protected bool) core.Backup {
	return core.Backup{
		ID:        core.ID(id),
		Type:      typ,
		EndedAt:   endedAt,
		SizeBytes: size,
		Protected: protected,
	}
}

func assertKeep(t *testing.T, plan Plan, ids ...string) {
	t.Helper()
	kept := plan.KeepIDs()
	for _, id := range ids {
		if _, ok := kept[core.ID(id)]; !ok {
			t.Fatalf("backup %q was not kept; plan=%#v", id, plan.Items)
		}
	}
}

func assertDrop(t *testing.T, plan Plan, ids ...string) {
	t.Helper()
	dropped := plan.DropIDs()
	for _, id := range ids {
		if _, ok := dropped[core.ID(id)]; !ok {
			t.Fatalf("backup %q was not dropped; plan=%#v", id, plan.Items)
		}
	}
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
