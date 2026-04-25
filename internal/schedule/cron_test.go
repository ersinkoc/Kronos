package schedule

import (
	"testing"
	"time"
)

func TestCronNextFiveField(t *testing.T) {
	t.Parallel()

	cron, err := ParseCron("30 2 * * *")
	if err != nil {
		t.Fatalf("ParseCron() error = %v", err)
	}
	if got := cron.String(); got != "30 2 * * *" {
		t.Fatalf("String() = %q", got)
	}
	after := time.Date(2026, 4, 25, 2, 29, 59, 0, time.UTC)
	got, err := cron.Next(after)
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	want := time.Date(2026, 4, 25, 2, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next() = %s, want %s", got, want)
	}
}

func TestCronFirstOfNextMonthDecember(t *testing.T) {
	t.Parallel()

	got := firstOfNextMonth(time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC))
	want := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("firstOfNextMonth() = %s, want %s", got, want)
	}
}

func TestCronNextSixFieldStep(t *testing.T) {
	t.Parallel()

	cron, err := ParseCron("*/15 * * * * *")
	if err != nil {
		t.Fatalf("ParseCron() error = %v", err)
	}
	after := time.Date(2026, 4, 25, 12, 0, 14, 0, time.UTC)
	got, err := cron.Next(after)
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	want := time.Date(2026, 4, 25, 12, 0, 15, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next() = %s, want %s", got, want)
	}
}

func TestCronMacrosAndNames(t *testing.T) {
	t.Parallel()

	cron, err := ParseCron("@weekly")
	if err != nil {
		t.Fatalf("ParseCron(@weekly) error = %v", err)
	}
	got, err := cron.Next(time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Next(@weekly) error = %v", err)
	}
	want := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next(@weekly) = %s, want %s", got, want)
	}

	cron, err = ParseCron("0 9 * jan mon")
	if err != nil {
		t.Fatalf("ParseCron(names) error = %v", err)
	}
	got, err = cron.Next(time.Date(2026, 1, 4, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Next(names) error = %v", err)
	}
	want = time.Date(2026, 1, 5, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next(names) = %s, want %s", got, want)
	}
}

func TestCronRangesAndLists(t *testing.T) {
	t.Parallel()

	cron, err := ParseCron("0 8-10 * * 1,3,5")
	if err != nil {
		t.Fatalf("ParseCron() error = %v", err)
	}
	got, err := cron.Next(time.Date(2026, 4, 27, 7, 59, 59, 0, time.UTC))
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	want := time.Date(2026, 4, 27, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next() = %s, want %s", got, want)
	}
}

func TestCronDayOfMonthDayOfWeekUsesClassicOr(t *testing.T) {
	t.Parallel()

	cron, err := ParseCron("0 9 15 * mon")
	if err != nil {
		t.Fatalf("ParseCron() error = %v", err)
	}
	got, err := cron.Next(time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	want := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next() = %s, want %s", got, want)
	}
	got, err = cron.Next(time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Next(second) error = %v", err)
	}
	want = time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("Next(second) = %s, want %s", got, want)
	}
}

func TestCronRejectsInvalid(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"* * *",
		"61 * * * * *",
		"*/0 * * * * *",
		"0 0 32 * *",
		"0 0 * nope *",
	}
	for _, expr := range tests {
		expr := expr
		t.Run(expr, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseCron(expr); err == nil {
				t.Fatalf("ParseCron(%q) error = nil, want error", expr)
			}
		})
	}
}

func BenchmarkCronNextTenThousand(b *testing.B) {
	cron, err := ParseCron("*/5 * * * * *")
	if err != nil {
		b.Fatalf("ParseCron() error = %v", err)
	}
	start := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 10000; j++ {
			if _, err := cron.Next(start.Add(time.Duration(j) * time.Second)); err != nil {
				b.Fatal(err)
			}
		}
	}
}
