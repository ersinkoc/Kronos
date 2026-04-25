package retention

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/kronos/kronos/internal/core"
)

// Plan is the resolved retention decision for a set of backups.
type Plan struct {
	Items []PlanItem `json:"items"`
}

// PlanItem is one backup plus its keep/drop decision and rule reasons.
type PlanItem struct {
	Backup  core.Backup `json:"backup"`
	Keep    bool        `json:"keep"`
	Reasons []string    `json:"reasons,omitempty"`
}

// Resolve applies policy rules and returns a newest-first plan.
func Resolve(backups []core.Backup, policy core.RetentionPolicy, now time.Time) (Plan, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	items := make([]PlanItem, 0, len(backups))
	for _, backup := range backups {
		items = append(items, PlanItem{Backup: backup})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return backupTime(items[i].Backup).After(backupTime(items[j].Backup))
	})

	for i := range items {
		if items[i].Backup.Protected {
			keep(&items[i], "protected")
		}
	}
	for _, rule := range policy.Rules {
		switch rule.Kind {
		case "count":
			if err := applyCount(items, rule); err != nil {
				return Plan{}, err
			}
		case "time":
			if err := applyTime(items, rule, now); err != nil {
				return Plan{}, err
			}
		case "size":
			if err := applySize(items, rule); err != nil {
				return Plan{}, err
			}
		case "gfs":
			if err := applyGFS(items, rule); err != nil {
				return Plan{}, err
			}
		default:
			return Plan{}, fmt.Errorf("unsupported retention rule %q", rule.Kind)
		}
	}
	protectKeptAncestors(items)
	return Plan{Items: items}, nil
}

// KeepIDs returns the IDs retained by the plan.
func (p Plan) KeepIDs() map[core.ID]struct{} {
	ids := make(map[core.ID]struct{})
	for _, item := range p.Items {
		if item.Keep {
			ids[item.Backup.ID] = struct{}{}
		}
	}
	return ids
}

// DropIDs returns the IDs not retained by the plan.
func (p Plan) DropIDs() map[core.ID]struct{} {
	ids := make(map[core.ID]struct{})
	for _, item := range p.Items {
		if !item.Keep {
			ids[item.Backup.ID] = struct{}{}
		}
	}
	return ids
}

func applyCount(items []PlanItem, rule core.RetentionRule) error {
	n, err := intParam(rule.Params, "n", "keep", "count")
	if err != nil {
		return err
	}
	if n < 0 {
		return fmt.Errorf("count retention must be >= 0")
	}
	backupType := stringParam(rule.Params, "type")
	kept := 0
	for i := range items {
		if backupType != "" && string(items[i].Backup.Type) != backupType {
			continue
		}
		if kept < n {
			keep(&items[i], "count")
			kept++
		}
	}
	return nil
}

func applyTime(items []PlanItem, rule core.RetentionRule, now time.Time) error {
	duration, err := durationParam(rule.Params, "duration", "younger_than")
	if err != nil {
		return err
	}
	if duration < 0 {
		return fmt.Errorf("time retention duration must be >= 0")
	}
	cutoff := now.Add(-duration)
	for i := range items {
		if !backupTime(items[i].Backup).Before(cutoff) {
			keep(&items[i], "time")
		}
	}
	return nil
}

func applySize(items []PlanItem, rule core.RetentionRule) error {
	maxBytes, err := int64Param(rule.Params, "max_bytes", "bytes")
	if err != nil {
		return err
	}
	if maxBytes < 0 {
		return fmt.Errorf("size retention max_bytes must be >= 0")
	}
	var total int64
	for i := range items {
		if items[i].Backup.Protected {
			continue
		}
		if total+items[i].Backup.SizeBytes <= maxBytes {
			keep(&items[i], "size")
			total += items[i].Backup.SizeBytes
		}
	}
	return nil
}

func applyGFS(items []PlanItem, rule core.RetentionRule) error {
	daily, err := optionalIntParam(rule.Params, "daily")
	if err != nil {
		return err
	}
	weekly, err := optionalIntParam(rule.Params, "weekly")
	if err != nil {
		return err
	}
	monthly, err := optionalIntParam(rule.Params, "monthly")
	if err != nil {
		return err
	}
	yearly, err := optionalIntParam(rule.Params, "yearly")
	if err != nil {
		return err
	}

	seenDaily := make(map[string]struct{})
	seenWeekly := make(map[string]struct{})
	seenMonthly := make(map[string]struct{})
	seenYearly := make(map[string]struct{})
	for i := range items {
		t := backupTime(items[i].Backup).UTC()
		if daily > len(seenDaily) {
			key := t.Format("2006-01-02")
			if _, ok := seenDaily[key]; !ok {
				seenDaily[key] = struct{}{}
				keep(&items[i], "gfs:daily")
			}
		}
		if weekly > len(seenWeekly) {
			year, week := t.ISOWeek()
			key := fmt.Sprintf("%04d-W%02d", year, week)
			if _, ok := seenWeekly[key]; !ok {
				seenWeekly[key] = struct{}{}
				keep(&items[i], "gfs:weekly")
			}
		}
		if monthly > len(seenMonthly) {
			key := t.Format("2006-01")
			if _, ok := seenMonthly[key]; !ok {
				seenMonthly[key] = struct{}{}
				keep(&items[i], "gfs:monthly")
			}
		}
		if yearly > len(seenYearly) {
			key := t.Format("2006")
			if _, ok := seenYearly[key]; !ok {
				seenYearly[key] = struct{}{}
				keep(&items[i], "gfs:yearly")
			}
		}
	}
	return nil
}

func protectKeptAncestors(items []PlanItem) {
	byID := make(map[core.ID]int, len(items))
	for i, item := range items {
		if !item.Backup.ID.IsZero() {
			byID[item.Backup.ID] = i
		}
	}
	for {
		changed := false
		for _, item := range items {
			if !item.Keep || item.Backup.ParentID.IsZero() {
				continue
			}
			parentIndex, ok := byID[item.Backup.ParentID]
			if !ok || items[parentIndex].Keep {
				continue
			}
			keep(&items[parentIndex], "chain")
			changed = true
		}
		if !changed {
			return
		}
	}
}

func keep(item *PlanItem, reason string) {
	item.Keep = true
	for _, existing := range item.Reasons {
		if existing == reason {
			return
		}
	}
	item.Reasons = append(item.Reasons, reason)
}

func backupTime(backup core.Backup) time.Time {
	if !backup.EndedAt.IsZero() {
		return backup.EndedAt
	}
	return backup.StartedAt
}

func intParam(params map[string]any, names ...string) (int, error) {
	value, ok, name := findParam(params, names...)
	if !ok {
		return 0, fmt.Errorf("missing retention param %q", names[0])
	}
	n, err := toInt64(value)
	if err != nil {
		return 0, fmt.Errorf("invalid retention param %q: %w", name, err)
	}
	return int(n), nil
}

func optionalIntParam(params map[string]any, name string) (int, error) {
	value, ok := params[name]
	if !ok {
		return 0, nil
	}
	n, err := toInt64(value)
	if err != nil {
		return 0, fmt.Errorf("invalid retention param %q: %w", name, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("retention param %q must be >= 0", name)
	}
	return int(n), nil
}

func int64Param(params map[string]any, names ...string) (int64, error) {
	value, ok, name := findParam(params, names...)
	if !ok {
		return 0, fmt.Errorf("missing retention param %q", names[0])
	}
	n, err := toInt64(value)
	if err != nil {
		return 0, fmt.Errorf("invalid retention param %q: %w", name, err)
	}
	return n, nil
}

func durationParam(params map[string]any, names ...string) (time.Duration, error) {
	value, ok, _ := findParam(params, names...)
	if !ok {
		return 0, fmt.Errorf("missing retention param %q", names[0])
	}
	switch v := value.(type) {
	case time.Duration:
		return v, nil
	case string:
		duration, err := time.ParseDuration(v)
		if err != nil {
			return 0, err
		}
		return duration, nil
	default:
		n, err := toInt64(value)
		if err != nil {
			return 0, fmt.Errorf("expected duration string or seconds: %w", err)
		}
		return time.Duration(n) * time.Second, nil
	}
}

func stringParam(params map[string]any, name string) string {
	if params == nil {
		return ""
	}
	value, ok := params[name]
	if !ok {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

func findParam(params map[string]any, names ...string) (any, bool, string) {
	if params == nil {
		return nil, false, ""
	}
	for _, name := range names {
		value, ok := params[name]
		if ok {
			return value, true, name
		}
	}
	return nil, false, ""
}

func toInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}
