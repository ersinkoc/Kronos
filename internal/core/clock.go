package core

import (
	"sync"
	"time"
)

// Clock provides a testable time source.
type Clock interface {
	Now() time.Time
	Sleep(d time.Duration)
}

// RealClock is the production wall-clock implementation.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time {
	return time.Now()
}

// Sleep blocks for d using time.Sleep.
func (RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// FakeClock is a deterministic clock for tests.
type FakeClock struct {
	mu      sync.Mutex
	now     time.Time
	waiters []fakeWaiter
}

type fakeWaiter struct {
	until time.Time
	done  chan struct{}
}

// NewFakeClock returns a fake clock initialised to start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now returns the fake clock's current time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// Set moves the fake clock to t and wakes sleepers whose deadline has passed.
func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.now = t
	c.wakeLocked()
	c.mu.Unlock()
}

// Advance moves the fake clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.wakeLocked()
	c.mu.Unlock()
}

// Sleep blocks until the fake clock has advanced by at least d.
func (c *FakeClock) Sleep(d time.Duration) {
	c.mu.Lock()
	until := c.now.Add(d)
	if !c.now.Before(until) {
		c.mu.Unlock()
		return
	}
	done := make(chan struct{})
	c.waiters = append(c.waiters, fakeWaiter{until: until, done: done})
	c.mu.Unlock()

	<-done
}

func (c *FakeClock) wakeLocked() {
	kept := c.waiters[:0]
	for _, waiter := range c.waiters {
		if c.now.Before(waiter.until) {
			kept = append(kept, waiter)
			continue
		}
		close(waiter.done)
	}
	c.waiters = kept
}
