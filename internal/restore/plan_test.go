package restore

import (
	"errors"
	"testing"
	"time"

	"github.com/kronos/kronos/internal/core"
)

func TestBuildPlanWalksChainRootFirst(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	backups := []core.Backup{
		backup("incr-2", "incr-1", core.BackupTypeIncremental, now),
		backup("full", "", core.BackupTypeFull, now.Add(-2*time.Hour)),
		backup("incr-1", "full", core.BackupTypeIncremental, now.Add(-time.Hour)),
	}
	plan, err := BuildPlan(backups, Request{BackupID: "incr-2", TargetID: "restore-target", At: now.Add(time.Minute)})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.BackupID != "incr-2" || plan.TargetID != "restore-target" || plan.StorageID != "storage" {
		t.Fatalf("plan = %#v", plan)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].BackupID != "full" || plan.Steps[1].BackupID != "incr-1" || plan.Steps[2].BackupID != "incr-2" {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if len(plan.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want point-in-time warning", plan.Warnings)
	}
}

func TestBuildPlanUsesOriginalTargetByDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	plan, err := BuildPlan([]core.Backup{backup("full", "", core.BackupTypeFull, now)}, Request{BackupID: "full"})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if plan.TargetID != "target" {
		t.Fatalf("target = %q, want original target", plan.TargetID)
	}
}

func TestBuildPlanRejectsInvalidChains(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	if _, err := BuildPlan(nil, Request{}); err == nil {
		t.Fatal("BuildPlan(no backup id) error = nil, want error")
	}
	if _, err := BuildPlan(nil, Request{BackupID: "missing"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("BuildPlan(missing) error = %v, want ErrNotFound", err)
	}
	if _, err := BuildPlan([]core.Backup{backup("incr", "missing", core.BackupTypeIncremental, now)}, Request{BackupID: "incr"}); err == nil {
		t.Fatal("BuildPlan(missing parent) error = nil, want error")
	}
	cycleA := backup("a", "b", core.BackupTypeIncremental, now)
	cycleB := backup("b", "a", core.BackupTypeIncremental, now)
	if _, err := BuildPlan([]core.Backup{cycleA, cycleB}, Request{BackupID: "a"}); err == nil {
		t.Fatal("BuildPlan(cycle) error = nil, want error")
	}
}

func backup(id core.ID, parentID core.ID, typ core.BackupType, endedAt time.Time) core.Backup {
	return core.Backup{
		ID:         id,
		ParentID:   parentID,
		TargetID:   "target",
		StorageID:  "storage",
		JobID:      "job-" + id,
		Type:       typ,
		ManifestID: "manifest-" + id,
		StartedAt:  endedAt.Add(-time.Minute),
		EndedAt:    endedAt,
	}
}
