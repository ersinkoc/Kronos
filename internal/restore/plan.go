package restore

import (
	"fmt"
	"time"

	"github.com/kronos/kronos/internal/core"
)

// Request describes a restore preview request.
type Request struct {
	BackupID        core.ID   `json:"backup_id"`
	TargetID        core.ID   `json:"target_id,omitempty"`
	At              time.Time `json:"at,omitempty"`
	DryRun          bool      `json:"dry_run,omitempty"`
	ReplaceExisting bool      `json:"replace_existing,omitempty"`
}

// Step describes one backup that must be applied during restore.
type Step struct {
	BackupID   core.ID         `json:"backup_id"`
	Type       core.BackupType `json:"type"`
	ParentID   core.ID         `json:"parent_id,omitempty"`
	ManifestID core.ID         `json:"manifest_id"`
	TargetID   core.ID         `json:"target_id"`
	StorageID  core.ID         `json:"storage_id"`
	StartedAt  time.Time       `json:"started_at"`
	EndedAt    time.Time       `json:"ended_at"`
}

// Plan is a deterministic restore plan suitable for a UI wizard or dry run.
type Plan struct {
	BackupID  core.ID   `json:"backup_id"`
	TargetID  core.ID   `json:"target_id"`
	StorageID core.ID   `json:"storage_id"`
	At        time.Time `json:"at,omitempty"`
	Steps     []Step    `json:"steps"`
	Warnings  []string  `json:"warnings,omitempty"`
}

// BuildPlan walks backup parent links and returns the restore chain root-first.
func BuildPlan(backups []core.Backup, request Request) (Plan, error) {
	if request.BackupID.IsZero() {
		return Plan{}, fmt.Errorf("backup_id is required")
	}
	byID := make(map[core.ID]core.Backup, len(backups))
	for _, backup := range backups {
		if backup.ID.IsZero() {
			continue
		}
		byID[backup.ID] = backup
	}
	selected, ok := byID[request.BackupID]
	if !ok {
		return Plan{}, core.WrapKind(core.ErrorKindNotFound, "build restore plan", fmt.Errorf("backup %q not found", request.BackupID))
	}
	targetID := request.TargetID
	if targetID.IsZero() {
		targetID = selected.TargetID
	}
	chain := make([]core.Backup, 0, 4)
	seen := make(map[core.ID]struct{})
	for current := selected; ; {
		if _, ok := seen[current.ID]; ok {
			return Plan{}, fmt.Errorf("backup chain contains a cycle at %q", current.ID)
		}
		seen[current.ID] = struct{}{}
		chain = append(chain, current)
		if current.ParentID.IsZero() {
			break
		}
		parent, ok := byID[current.ParentID]
		if !ok {
			return Plan{}, fmt.Errorf("backup %q parent %q is missing", current.ID, current.ParentID)
		}
		current = parent
	}
	reverseBackups(chain)
	steps := make([]Step, 0, len(chain))
	for _, backup := range chain {
		steps = append(steps, Step{
			BackupID:   backup.ID,
			Type:       backup.Type,
			ParentID:   backup.ParentID,
			ManifestID: backup.ManifestID,
			TargetID:   backup.TargetID,
			StorageID:  backup.StorageID,
			StartedAt:  backup.StartedAt,
			EndedAt:    backup.EndedAt,
		})
	}
	plan := Plan{
		BackupID:  selected.ID,
		TargetID:  targetID,
		StorageID: selected.StorageID,
		At:        request.At,
		Steps:     steps,
	}
	if !request.At.IsZero() && request.At.After(selected.EndedAt) {
		plan.Warnings = append(plan.Warnings, "point-in-time replay requires stream backups after the selected snapshot")
	}
	return plan, nil
}

func reverseBackups(backups []core.Backup) {
	for i, j := 0, len(backups)-1; i < j; i, j = i+1, j-1 {
		backups[i], backups[j] = backups[j], backups[i]
	}
}
