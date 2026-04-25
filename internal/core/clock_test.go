package core

import (
	"runtime"
	"testing"
	"time"
)

func TestFakeClockAdvanceWakesSleepers(t *testing.T) {
	t.Parallel()

	clock := NewFakeClock(time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	done := make(chan struct{})

	go func() {
		clock.Sleep(5 * time.Second)
		close(done)
	}()

	waitForFakeWaiter(t, clock)

	clock.Advance(5 * time.Second)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sleep did not return after time advanced")
	}
}

func waitForFakeWaiter(t *testing.T, clock *FakeClock) {
	t.Helper()

	for i := 0; i < 1000; i++ {
		clock.mu.Lock()
		waiters := len(clock.waiters)
		clock.mu.Unlock()
		if waiters > 0 {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("fake clock sleeper did not register")
}

func TestFakeClockSet(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	next := start.Add(time.Hour)
	clock := NewFakeClock(start)

	clock.Set(next)

	if got := clock.Now(); !got.Equal(next) {
		t.Fatalf("Now() = %s, want %s", got, next)
	}
}

func TestRealClockAndFakeClockImmediateSleep(t *testing.T) {
	t.Parallel()

	before := time.Now()
	clock := RealClock{}
	if got := clock.Now(); got.Before(before) {
		t.Fatalf("RealClock.Now() = %s, before %s", got, before)
	}
	clock.Sleep(0)

	fake := NewFakeClock(before)
	fake.Sleep(0)
}
