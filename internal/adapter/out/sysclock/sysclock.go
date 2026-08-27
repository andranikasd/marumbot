// Package sysclock is the only place in Marum that reads the system clock.
//
// Everything else takes a Clock, so a lease, a retry schedule or a reminder can
// be tested by moving time rather than by sleeping — and a test that sleeps is a
// test that is slow when it passes and flaky when it fails.
package sysclock

import "time"

// Clock reads the wall clock.
type Clock struct{}

// New returns the system clock.
func New() Clock { return Clock{} }

// Now is the current instant.
func (Clock) Now() time.Time { return time.Now() }
