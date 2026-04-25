package schedule

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

// Window is a daily time window used by @between schedules.
type Window struct {
	Start  time.Duration
	End    time.Duration
	Zone   *time.Location
	Random bool
}

// ParseWindow parses expressions such as "@between 02:00-04:00 UTC random".
func ParseWindow(expr string) (Window, error) {
	fields := strings.Fields(expr)
	if len(fields) < 2 || fields[0] != "@between" {
		return Window{}, fmt.Errorf("window expression must start with @between")
	}
	start, end, err := parseWindowRange(fields[1])
	if err != nil {
		return Window{}, err
	}
	window := Window{Start: start, End: end, Zone: time.UTC}
	for _, field := range fields[2:] {
		if field == "random" {
			window.Random = true
			continue
		}
		loc, err := time.LoadLocation(field)
		if err != nil {
			return Window{}, fmt.Errorf("load timezone %q: %w", field, err)
		}
		window.Zone = loc
	}
	if window.End <= window.Start {
		return Window{}, fmt.Errorf("window end must be after start")
	}
	return window, nil
}

// Next returns the next window instant strictly after after.
func (w Window) Next(after time.Time, stableKey string) (time.Time, error) {
	if w.Zone == nil {
		w.Zone = time.UTC
	}
	local := after.In(w.Zone)
	for i := 0; i < 370; i++ {
		day := time.Date(local.Year(), local.Month(), local.Day()+i, 0, 0, 0, 0, w.Zone)
		candidate := day.Add(w.Start)
		if w.Random {
			candidate = day.Add(w.Start + w.offsetFor(day, stableKey))
		}
		if candidate.After(local) {
			return candidate, nil
		}
	}
	return time.Time{}, fmt.Errorf("no window time found within one year")
}

func (w Window) offsetFor(day time.Time, stableKey string) time.Duration {
	width := w.End - w.Start
	if width <= 0 {
		return 0
	}
	hash := fnv.New64a()
	fmt.Fprintf(hash, "%s:%s", stableKey, day.Format("2006-01-02"))
	return time.Duration(hash.Sum64() % uint64(width))
}

func parseWindowRange(value string) (time.Duration, time.Duration, error) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid window range %q", value)
	}
	start, err := parseClock(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("start: %w", err)
	}
	end, err := parseClock(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("end: %w", err)
	}
	return start, end, nil
}

func parseClock(value string) (time.Duration, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid clock %q", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("clock %q outside 00:00-23:59", value)
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}
