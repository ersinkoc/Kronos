package core

import (
	"strings"
	"testing"
	"time"
)

func TestNewIDIsUUIDv7(t *testing.T) {
	t.Parallel()

	clock := NewFakeClock(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	id, err := NewID(clock)
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}

	text := id.String()
	if len(text) != 36 {
		t.Fatalf("len(id) = %d, want 36", len(text))
	}
	if text[14] != '7' {
		t.Fatalf("version nibble = %q, want 7", text[14])
	}
	if !strings.ContainsRune("89ab", rune(text[19])) {
		t.Fatalf("variant nibble = %q, want one of 89ab", text[19])
	}
}

func TestNewIDSortsByTimestampAcrossMilliseconds(t *testing.T) {
	t.Parallel()

	clock := NewFakeClock(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	first, err := NewID(clock)
	if err != nil {
		t.Fatalf("NewID(first) error = %v", err)
	}

	clock.Advance(time.Millisecond)
	second, err := NewID(clock)
	if err != nil {
		t.Fatalf("NewID(second) error = %v", err)
	}

	if first.String() >= second.String() {
		t.Fatalf("first ID %q should sort before second ID %q", first, second)
	}
}

func TestIDIsZero(t *testing.T) {
	t.Parallel()

	if !(ID("").IsZero()) {
		t.Fatal("empty ID should be zero")
	}
	if ID("018f8f84-95c0-7000-8000-000000000000").IsZero() {
		t.Fatal("non-empty ID should not be zero")
	}
}
