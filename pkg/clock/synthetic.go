package clock

import (
	"sync"
	"time"
)

// SyntheticClock provides deterministic replay of recorded timing.
// It advances through pre-loaded deltas, optionally sleeping in real-time
// or running as fast as possible for testing.
type SyntheticClock interface {
	Clock

	// Load initializes the clock with a start time and sequence of deltas
	Load(start MonoTime, deltas []time.Duration)

	// Advance moves to the next delta, optionally sleeping in real-time
	Advance()

	// SetSpeed sets the playback speed multiplier (1.0 = real-time, 2.0 = 2x speed)
	SetSpeed(mult float64)

	// SetNoSleep disables real-time sleeping (for fast testing)
	SetNoSleep(noSleep bool)

	// Reset clears the loaded deltas and resets to start
	Reset()

	// HasNext returns true if there are more deltas to advance through
	HasNext() bool

	// CurrentIndex returns the current position in the delta sequence
	CurrentIndex() int
}

// DeltaClock implements SyntheticClock for deterministic replay.
type DeltaClock struct {
	mu sync.RWMutex

	start   MonoTime          // Initial timestamp
	deltas  []time.Duration   // Pre-loaded deltas
	current MonoTime          // Current monotonic time
	index   int               // Current position in deltas
	speed   float64           // Playback speed multiplier
	noSleep bool              // If true, skip real-time sleeping
	realClk *SystemClock      // For real-time sleeping
}

// NewDeltaClock creates a new SyntheticClock.
func NewDeltaClock() *DeltaClock {
	return &DeltaClock{
		start:   0,
		deltas:  nil,
		current: 0,
		index:   0,
		speed:   1.0,
		noSleep: false,
		realClk: NewSystemClock(),
	}
}

// Load initializes the clock with a start time and deltas.
func (d *DeltaClock) Load(start MonoTime, deltas []time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.start = start
	d.current = start
	d.deltas = make([]time.Duration, len(deltas))
	copy(d.deltas, deltas)
	d.index = 0
}

// Now returns the current monotonic time.
func (d *DeltaClock) Now() MonoTime {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.current
}

// Since returns the duration elapsed since the given time.
func (d *DeltaClock) Since(t MonoTime) time.Duration {
	return ToDuration(d.Now() - t)
}

// Advance moves to the next delta in the sequence.
// If noSleep is false, it sleeps in real-time (scaled by speed multiplier).
func (d *DeltaClock) Advance() {
	d.mu.Lock()

	if d.index >= len(d.deltas) {
		d.mu.Unlock()
		return // No more deltas
	}

	delta := d.deltas[d.index]
	d.index++

	// Calculate sleep duration with speed multiplier
	var sleepDuration time.Duration
	if !d.noSleep && d.speed > 0 {
		sleepDuration = time.Duration(float64(delta) / d.speed)
	}

	// Update current time
	d.current += FromDuration(delta)

	d.mu.Unlock()

	// Sleep outside the lock
	if sleepDuration > 0 {
		time.Sleep(sleepDuration)
	}
}

// SetSpeed sets the playback speed multiplier.
func (d *DeltaClock) SetSpeed(mult float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if mult < 0 {
		mult = 1.0
	}
	d.speed = mult
}

// SetNoSleep enables or disables real-time sleeping.
func (d *DeltaClock) SetNoSleep(noSleep bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.noSleep = noSleep
}

// Reset clears the deltas and resets to start time.
func (d *DeltaClock) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.current = d.start
	d.index = 0
}

// HasNext returns true if there are more deltas to advance.
func (d *DeltaClock) HasNext() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.index < len(d.deltas)
}

// CurrentIndex returns the current position in the delta sequence.
func (d *DeltaClock) CurrentIndex() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.index
}

// AdvanceAll advances through all remaining deltas.
// Useful for fast-forwarding to the end.
func (d *DeltaClock) AdvanceAll() {
	for d.HasNext() {
		d.Advance()
	}
}

// RemainingDeltas returns the number of deltas left to process.
func (d *DeltaClock) RemainingDeltas() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.deltas) - d.index
}

// TotalDeltas returns the total number of deltas loaded.
func (d *DeltaClock) TotalDeltas() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.deltas)
}
