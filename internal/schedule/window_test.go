package schedule

import (
	"testing"
	"time"
)

func TestParseWindowNext(t *testing.T) {
	t.Parallel()

	window, err := ParseWindow("@between 02:00-04:00 UTC")
	if err != nil {
		t.Fatalf("ParseWindow() error = %v", err)
	}
	got, err := window.Next(time.Date(2026, 4, 25, 1, 59, 0, 0, time.UTC), "schedule-1")
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	want := time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next() = %s, want %s", got, want)
	}
	got, err = window.Next(time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC), "schedule-1")
	if err != nil {
		t.Fatalf("Next(second) error = %v", err)
	}
	want = time.Date(2026, 4, 26, 2, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next(second) = %s, want %s", got, want)
	}
}

func TestWindowRandomIsStableAndInsideRange(t *testing.T) {
	t.Parallel()

	window, err := ParseWindow("@between 02:00-04:00 UTC random")
	if err != nil {
		t.Fatalf("ParseWindow() error = %v", err)
	}
	after := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	first, err := window.Next(after, "schedule-1")
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	second, err := window.Next(after, "schedule-1")
	if err != nil {
		t.Fatalf("Next(second) error = %v", err)
	}
	if !first.Equal(second) {
		t.Fatalf("random window is not stable: %s vs %s", first, second)
	}
	start := time.Date(2026, 4, 25, 2, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 25, 4, 0, 0, 0, time.UTC)
	if first.Before(start) || !first.Before(end) {
		t.Fatalf("random window = %s outside [%s, %s)", first, start, end)
	}
	other, err := window.Next(after, "schedule-2")
	if err != nil {
		t.Fatalf("Next(other) error = %v", err)
	}
	if first.Equal(other) {
		t.Fatalf("two stable keys picked identical instant %s; want spread", first)
	}
}

func TestWindowTimezone(t *testing.T) {
	t.Parallel()

	window, err := ParseWindow("@between 02:00-03:00 Europe/Tallinn")
	if err != nil {
		t.Fatalf("ParseWindow() error = %v", err)
	}
	got, err := window.Next(time.Date(2026, 4, 24, 23, 30, 0, 0, time.UTC), "schedule-1")
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if got.Location().String() != "Europe/Tallinn" || got.Hour() != 2 {
		t.Fatalf("Next() = %s, want 02:00 Europe/Tallinn", got)
	}
}

func TestWindowRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"@daily",
		"@between nope UTC",
		"@between 04:00-02:00 UTC",
		"@between 24:00-25:00 UTC",
	}
	for _, expr := range tests {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseWindow(expr); err == nil {
				t.Fatalf("ParseWindow(%q) error = nil, want error", expr)
			}
		})
	}
}
