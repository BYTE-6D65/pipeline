package clock

import (
	"testing"
	"time"
)

func TestDeltaClock_Load(t *testing.T) {
	clk := NewDeltaClock()

	start := MonoTime(1000000) // 1ms
	deltas := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		15 * time.Millisecond,
	}

	clk.Load(start, deltas)

	if clk.Now() != start {
		t.Errorf("Expected start time %d, got %d", start, clk.Now())
	}

	if !clk.HasNext() {
		t.Error("Should have deltas to advance")
	}

	if clk.TotalDeltas() != 3 {
		t.Errorf("Expected 3 deltas, got %d", clk.TotalDeltas())
	}
}

func TestDeltaClock_Advance(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true) // Fast test

	start := MonoTime(0)
	deltas := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		15 * time.Millisecond,
	}

	clk.Load(start, deltas)

	// Initial state
	if clk.Now() != start {
		t.Errorf("Expected start=%d, got %d", start, clk.Now())
	}

	// Advance 1
	clk.Advance()
	expected := start + FromDuration(10*time.Millisecond)
	if clk.Now() != expected {
		t.Errorf("After delta 1: expected %d, got %d", expected, clk.Now())
	}

	// Advance 2
	clk.Advance()
	expected = start + FromDuration(30*time.Millisecond) // 10+20
	if clk.Now() != expected {
		t.Errorf("After delta 2: expected %d, got %d", expected, clk.Now())
	}

	// Advance 3
	clk.Advance()
	expected = start + FromDuration(45*time.Millisecond) // 10+20+15
	if clk.Now() != expected {
		t.Errorf("After delta 3: expected %d, got %d", expected, clk.Now())
	}

	// No more deltas
	if clk.HasNext() {
		t.Error("Should have no more deltas")
	}
}

func TestDeltaClock_AdvanceAll(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	start := MonoTime(0)
	deltas := []time.Duration{
		5 * time.Millisecond,
		10 * time.Millisecond,
		15 * time.Millisecond,
		20 * time.Millisecond,
	}

	clk.Load(start, deltas)
	clk.AdvanceAll()

	// Should be at sum of all deltas
	totalDelta := time.Duration(0)
	for _, d := range deltas {
		totalDelta += d
	}
	expected := start + FromDuration(totalDelta)

	if clk.Now() != expected {
		t.Errorf("Expected final time %d, got %d", expected, clk.Now())
	}

	if clk.HasNext() {
		t.Error("All deltas should be consumed")
	}
}

func TestDeltaClock_Reset(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	start := MonoTime(1000)
	deltas := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}

	clk.Load(start, deltas)
	clk.Advance()
	clk.Advance()

	// Reset
	clk.Reset()

	if clk.Now() != start {
		t.Errorf("After reset: expected %d, got %d", start, clk.Now())
	}

	if clk.CurrentIndex() != 0 {
		t.Errorf("After reset: expected index 0, got %d", clk.CurrentIndex())
	}

	if !clk.HasNext() {
		t.Error("After reset: should have deltas available")
	}
}

func TestDeltaClock_Since(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	start := MonoTime(0)
	deltas := []time.Duration{50 * time.Millisecond}

	clk.Load(start, deltas)

	t1 := clk.Now()
	clk.Advance()

	elapsed := clk.Since(t1)
	if elapsed != 50*time.Millisecond {
		t.Errorf("Expected 50ms elapsed, got %v", elapsed)
	}
}

func TestDeltaClock_SetSpeed_NoSleep(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true) // Should ignore speed when noSleep=true

	start := MonoTime(0)
	deltas := []time.Duration{100 * time.Millisecond}

	clk.Load(start, deltas)
	clk.SetSpeed(10.0) // 10x speed (should be ignored with noSleep)

	before := time.Now()
	clk.Advance()
	elapsed := time.Since(before)

	// Should be instant (< 10ms) since noSleep=true
	if elapsed > 10*time.Millisecond {
		t.Errorf("With noSleep, advance should be instant, took %v", elapsed)
	}

	// But clock time should still advance by full delta
	if clk.Now() != start+FromDuration(100*time.Millisecond) {
		t.Error("Clock time should advance by full delta regardless of noSleep")
	}
}

func TestDeltaClock_SetSpeed_RealTime(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(false) // Real-time sleeping
	clk.SetSpeed(10.0)    // 10x speed = 1/10th sleep time

	start := MonoTime(0)
	deltas := []time.Duration{100 * time.Millisecond} // Would normally sleep 100ms

	clk.Load(start, deltas)

	before := time.Now()
	clk.Advance()
	elapsed := time.Since(before)

	// Should sleep ~10ms (100ms / 10x speed)
	if elapsed < 5*time.Millisecond || elapsed > 20*time.Millisecond {
		t.Logf("Note: Expected ~10ms sleep, took %v", elapsed)
	}

	// Clock time advances by full delta
	if clk.Now() != start+FromDuration(100*time.Millisecond) {
		t.Error("Clock time should advance by full delta")
	}
}

func TestDeltaClock_CurrentIndex(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	deltas := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}

	clk.Load(0, deltas)

	if clk.CurrentIndex() != 0 {
		t.Error("Initial index should be 0")
	}

	clk.Advance()
	if clk.CurrentIndex() != 1 {
		t.Errorf("After 1 advance: expected index 1, got %d", clk.CurrentIndex())
	}

	clk.Advance()
	if clk.CurrentIndex() != 2 {
		t.Errorf("After 2 advances: expected index 2, got %d", clk.CurrentIndex())
	}

	clk.Advance()
	if clk.CurrentIndex() != 3 {
		t.Errorf("After 3 advances: expected index 3, got %d", clk.CurrentIndex())
	}
}

func TestDeltaClock_RemainingDeltas(t *testing.T) {
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	deltas := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
	}

	clk.Load(0, deltas)

	if clk.RemainingDeltas() != 3 {
		t.Errorf("Expected 3 remaining, got %d", clk.RemainingDeltas())
	}

	clk.Advance()
	if clk.RemainingDeltas() != 2 {
		t.Errorf("Expected 2 remaining, got %d", clk.RemainingDeltas())
	}

	clk.AdvanceAll()
	if clk.RemainingDeltas() != 0 {
		t.Errorf("Expected 0 remaining, got %d", clk.RemainingDeltas())
	}
}

func TestDeltaClock_EmptyDeltas(t *testing.T) {
	clk := NewDeltaClock()

	clk.Load(100, []time.Duration{})

	if clk.HasNext() {
		t.Error("Empty deltas should not have next")
	}

	// Advance should be no-op
	clk.Advance()

	if clk.Now() != 100 {
		t.Error("Time should not change with empty deltas")
	}
}

func TestDeltaClock_NegativeSpeed(t *testing.T) {
	clk := NewDeltaClock()

	clk.SetSpeed(-5.0) // Negative should default to 1.0

	clk.Load(0, []time.Duration{10 * time.Millisecond})
	clk.SetNoSleep(false)

	// Should not panic and should use default speed
	before := time.Now()
	clk.Advance()
	elapsed := time.Since(before)

	// With speed=1.0 (default), should sleep ~10ms
	if elapsed < 5*time.Millisecond || elapsed > 20*time.Millisecond {
		t.Logf("Note: Expected ~10ms sleep with default speed, took %v", elapsed)
	}
}

func TestDeltaClock_ReplayScenario(t *testing.T) {
	// Simulate replaying a recorded macro
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	// Recorded deltas from a typing session
	start := MonoTime(0)
	deltas := []time.Duration{
		0 * time.Millisecond,    // First event at t=0
		75 * time.Millisecond,   // Second event at t=75ms
		80 * time.Millisecond,   // Third event at t=155ms
		100 * time.Millisecond,  // Fourth event at t=255ms
	}

	clk.Load(start, deltas)

	expectedTimes := []MonoTime{
		0,
		FromDuration(0 * time.Millisecond),   // After first delta (0ms)
		FromDuration(75 * time.Millisecond),  // After second delta (75ms)
		FromDuration(155 * time.Millisecond), // After third delta (80ms)
		FromDuration(255 * time.Millisecond), // After fourth delta (100ms)
	}

	// Check initial time
	if clk.Now() != expectedTimes[0] {
		t.Errorf("Initial: expected time %d, got %d", expectedTimes[0], clk.Now())
	}

	// Advance through all deltas
	for i := 0; i < len(deltas); i++ {
		clk.Advance()
		expected := expectedTimes[i+1]
		actual := clk.Now()
		if actual != expected {
			t.Errorf("After advance %d: expected time %d, got %d", i+1, expected, actual)
		}
	}

	t.Logf("✓ Replayed %d events deterministically", len(deltas))
}

func TestDeltaClock_ChordDetection(t *testing.T) {
	// Simulate detecting a chord during replay
	clk := NewDeltaClock()
	clk.SetNoSleep(true)

	start := MonoTime(0)
	// Three keys pressed within 50ms window (a chord)
	deltas := []time.Duration{
		0,                      // Key 1 at t=0
		20 * time.Millisecond,  // Key 2 at t=20ms
		45 * time.Millisecond,  // Key 3 at t=65ms (within 50ms of key 2)
		200 * time.Millisecond, // Key 4 at t=265ms (separate)
	}

	clk.Load(start, deltas)

	chordWindow := FromDuration(50 * time.Millisecond)
	chordStart := clk.Now()

	var chordKeys int
	for clk.HasNext() {
		clk.Advance()
		if clk.Now()-chordStart <= chordWindow {
			chordKeys++
		} else {
			break
		}
	}

	if chordKeys != 2 { // Keys 2 and 3 (key 1 is at chordStart)
		t.Errorf("Expected 2 keys in chord window, got %d", chordKeys)
	}

	t.Logf("✓ Detected chord with %d keys in 50ms window", chordKeys+1) // +1 for initial key
}
