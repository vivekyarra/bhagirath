// Package clock provides the single source of "now" for every agent.
//
// All timestamps are produced in Go (never by database defaults) so that the
// demo deployment can fast-forward time to show deadline timeouts and so that
// the backtest replays history through exactly the same code paths.
package clock

import (
	"sync"
	"time"
)

// Clock returns the current instant.
type Clock interface {
	Now() time.Time
}

// System is the wall clock.
type System struct{}

// Now returns the current UTC time truncated to microseconds (Postgres precision).
func (System) Now() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// Offset is a wall clock that can be shifted forward. It is only enabled in
// demo mode, where judges can "advance 24 hours" to watch timers fire.
type Offset struct {
	mu     sync.RWMutex
	offset time.Duration
}

// Now returns wall time plus the configured offset.
func (o *Offset) Now() time.Time {
	o.mu.RLock()
	d := o.offset
	o.mu.RUnlock()
	return time.Now().UTC().Add(d).Truncate(time.Microsecond)
}

// Advance shifts the clock forward by d (negative values are ignored).
func (o *Offset) Advance(d time.Duration) {
	if d <= 0 {
		return
	}
	o.mu.Lock()
	o.offset += d
	o.mu.Unlock()
}

// Set replaces the offset (used when restoring persisted demo state).
func (o *Offset) Set(d time.Duration) {
	o.mu.Lock()
	o.offset = d
	o.mu.Unlock()
}

// Elapsed reports the current offset.
func (o *Offset) Elapsed() time.Duration {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.offset
}

// Fixed is a manually controlled clock for tests and backtests.
type Fixed struct {
	mu sync.Mutex
	t  time.Time
}

// NewFixed returns a clock frozen at t.
func NewFixed(t time.Time) *Fixed { return &Fixed{t: t.UTC()} }

// Now returns the frozen time.
func (f *Fixed) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

// Set moves the clock to t.
func (f *Fixed) Set(t time.Time) {
	f.mu.Lock()
	f.t = t.UTC()
	f.mu.Unlock()
}

// Advance moves the clock forward by d.
func (f *Fixed) Advance(d time.Duration) {
	f.mu.Lock()
	f.t = f.t.Add(d)
	f.mu.Unlock()
}

// IST is India Standard Time, used only for display.
var IST = time.FixedZone("IST", 5*3600+1800)
