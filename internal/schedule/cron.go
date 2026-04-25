package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cron is a parsed 5-field or 6-field cron expression.
type Cron struct {
	expr    string
	seconds bitset
	minutes bitset
	hours   bitset
	dom     bitset
	months  bitset
	dow     bitset
	domAny  bool
	dowAny  bool
}

type bitset uint64

// ParseCron parses standard 5-field cron or second-precision 6-field cron.
func ParseCron(expr string) (Cron, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Cron{}, fmt.Errorf("cron expression is required")
	}
	switch expr {
	case "@hourly":
		expr = "0 * * * *"
	case "@daily", "@midnight":
		expr = "0 0 * * *"
	case "@weekly":
		expr = "0 0 * * 0"
	case "@monthly":
		expr = "0 0 1 * *"
	case "@yearly", "@annually":
		expr = "0 0 1 1 *"
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 && len(fields) != 6 {
		return Cron{}, fmt.Errorf("cron expression must have 5 or 6 fields")
	}
	if len(fields) == 5 {
		fields = append([]string{"0"}, fields...)
	}
	seconds, err := parseField(fields[0], 0, 59, nil)
	if err != nil {
		return Cron{}, fmt.Errorf("seconds: %w", err)
	}
	minutes, err := parseField(fields[1], 0, 59, nil)
	if err != nil {
		return Cron{}, fmt.Errorf("minutes: %w", err)
	}
	hours, err := parseField(fields[2], 0, 23, nil)
	if err != nil {
		return Cron{}, fmt.Errorf("hours: %w", err)
	}
	dom, err := parseField(fields[3], 1, 31, nil)
	if err != nil {
		return Cron{}, fmt.Errorf("day-of-month: %w", err)
	}
	months, err := parseField(fields[4], 1, 12, map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	})
	if err != nil {
		return Cron{}, fmt.Errorf("month: %w", err)
	}
	dow, err := parseField(fields[5], 0, 7, map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	})
	if err != nil {
		return Cron{}, fmt.Errorf("day-of-week: %w", err)
	}
	if dow.has(7) {
		dow |= 1
		dow &^= 1 << 7
	}
	return Cron{
		expr:    expr,
		seconds: seconds,
		minutes: minutes,
		hours:   hours,
		dom:     dom,
		months:  months,
		dow:     dow,
		domAny:  isAny(fields[3]),
		dowAny:  isAny(fields[5]),
	}, nil
}

// String returns the normalized expression.
func (c Cron) String() string {
	return c.expr
}

// Next returns the next matching instant strictly after after.
func (c Cron) Next(after time.Time) (time.Time, error) {
	if c.seconds == 0 || c.minutes == 0 || c.hours == 0 || c.dom == 0 || c.months == 0 || c.dow == 0 {
		return time.Time{}, fmt.Errorf("cron expression is not initialized")
	}
	loc := after.Location()
	t := after.Add(time.Second).Truncate(time.Second)
	deadline := after.AddDate(5, 0, 0)
	for !t.After(deadline) {
		if !c.months.has(int(t.Month())) {
			t = firstOfNextMonth(t)
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !c.hours.has(t.Hour()) {
			next, ok := c.hours.next(t.Hour()+1, 23)
			if ok {
				t = time.Date(t.Year(), t.Month(), t.Day(), next, 0, 0, 0, loc)
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			}
			continue
		}
		if !c.minutes.has(t.Minute()) {
			next, ok := c.minutes.next(t.Minute()+1, 59)
			if ok {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), next, 0, 0, loc)
			} else {
				t = t.Add(time.Hour).Truncate(time.Hour)
			}
			continue
		}
		if !c.seconds.has(t.Second()) {
			next, ok := c.seconds.next(t.Second()+1, 59)
			if ok {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), next, 0, loc)
			} else {
				t = t.Add(time.Minute).Truncate(time.Minute)
			}
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no matching time found within 5 years")
}

func parseField(field string, min, max int, names map[string]int) (bitset, error) {
	var out bitset
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return 0, fmt.Errorf("empty field part")
		}
		base, step, err := splitStep(part)
		if err != nil {
			return 0, err
		}
		start, end, err := parseRange(base, min, max, names)
		if err != nil {
			return 0, err
		}
		for i := start; i <= end; i += step {
			out |= 1 << i
		}
	}
	return out, nil
}

func splitStep(part string) (string, int, error) {
	pieces := strings.Split(part, "/")
	if len(pieces) == 1 {
		return part, 1, nil
	}
	if len(pieces) != 2 {
		return "", 0, fmt.Errorf("invalid step %q", part)
	}
	step, err := strconv.Atoi(pieces[1])
	if err != nil || step <= 0 {
		return "", 0, fmt.Errorf("invalid step %q", pieces[1])
	}
	return pieces[0], step, nil
}

func parseRange(part string, min, max int, names map[string]int) (int, int, error) {
	if part == "*" || part == "?" {
		return min, max, nil
	}
	pieces := strings.Split(part, "-")
	if len(pieces) == 1 {
		value, err := parseValue(pieces[0], names)
		if err != nil {
			return 0, 0, err
		}
		if value < min || value > max {
			return 0, 0, fmt.Errorf("value %d outside %d-%d", value, min, max)
		}
		return value, value, nil
	}
	if len(pieces) != 2 {
		return 0, 0, fmt.Errorf("invalid range %q", part)
	}
	start, err := parseValue(pieces[0], names)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseValue(pieces[1], names)
	if err != nil {
		return 0, 0, err
	}
	if start < min || end > max || start > end {
		return 0, 0, fmt.Errorf("range %d-%d outside %d-%d", start, end, min, max)
	}
	return start, end, nil
}

func parseValue(value string, names map[string]int) (int, error) {
	lower := strings.ToLower(value)
	if names != nil {
		if n, ok := names[lower]; ok {
			return n, nil
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", value)
	}
	return n, nil
}

func (c Cron) dayMatches(t time.Time) bool {
	domMatch := c.dom.has(t.Day())
	dowMatch := c.dow.has(int(t.Weekday()))
	if c.domAny && c.dowAny {
		return true
	}
	if c.domAny {
		return dowMatch
	}
	if c.dowAny {
		return domMatch
	}
	return domMatch || dowMatch
}

func (b bitset) has(n int) bool {
	return b&(1<<n) != 0
}

func (b bitset) next(start, max int) (int, bool) {
	for i := start; i <= max; i++ {
		if b.has(i) {
			return i, true
		}
	}
	return 0, false
}

func firstOfNextMonth(t time.Time) time.Time {
	loc := t.Location()
	if t.Month() == time.December {
		return time.Date(t.Year()+1, time.January, 1, 0, 0, 0, 0, loc)
	}
	return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
}

func isAny(field string) bool {
	for _, part := range strings.Split(field, ",") {
		base, _, err := splitStep(strings.TrimSpace(part))
		if err != nil {
			return false
		}
		if base == "*" || base == "?" {
			return true
		}
	}
	return false
}
