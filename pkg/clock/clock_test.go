package clock

import (
	"testing"
	"time"
)

func TestMonoTime_Conversions(t *testing.T) {
	// Test duration conversion
	d := 100 * time.Millisecond
	mono := FromDuration(d)
	back := ToDuration(mono)

	if back != d {
		t.Errorf("Round-trip conversion failed: %v -> %v -> %v", d, mono, back)
	}

	// Test nanosecond precision
	nanos := int64(123456789)
	mono = FromUnixNano(nanos)
	if ToUnixNano(mono) != nanos {
		t.Error("Unix nano conversion failed")
	}
}

func TestSystemClock_Now(t *testing.T) {
	clk := NewSystemClock()

	t1 := clk.Now()
	time.Sleep(10 * time.Millisecond)
	t2 := clk.Now()

	if t2 <= t1 {
		t.Error("Clock should advance monotonically")
	}

	elapsed := t2 - t1
	if elapsed < FromDuration(10*time.Millisecond) {
		t.Errorf("Expected at least 10ms elapsed, got %v", ToDuration(elapsed))
	}
}

func TestSystemClock_Since(t *testing.T) {
	clk := NewSystemClock()

	start := clk.Now()
	time.Sleep(20 * time.Millisecond)
	elapsed := clk.Since(start)

	if elapsed < 20*time.Millisecond {
		t.Errorf("Expected at least 20ms, got %v", elapsed)
	}

	if elapsed > 100*time.Millisecond {
		t.Errorf("Expected less than 100ms, got %v", elapsed)
	}
}

func TestSystemClock_MonotonicBehavior(t *testing.T) {
	clk := NewSystemClock()

	// Capture many timestamps rapidly
	const iterations = 1000
	timestamps := make([]MonoTime, iterations)

	for i := 0; i < iterations; i++ {
		timestamps[i] = clk.Now()
	}

	// Verify monotonically increasing
	for i := 1; i < len(timestamps); i++ {
		if timestamps[i] < timestamps[i-1] {
			t.Errorf("Non-monotonic at index %d: %d -> %d",
				i, timestamps[i-1], timestamps[i])
		}
	}

	t.Logf("Captured %d monotonic timestamps successfully", iterations)
}

func TestSystemClock_SubMillisecondPrecision(t *testing.T) {
	clk := NewSystemClock()

	t1 := clk.Now()
	// Spin for a tiny bit
	for i := 0; i < 1000; i++ {
		_ = i * i
	}
	t2 := clk.Now()

	diff := t2 - t1
	t.Logf("Tiny operation took: %v", ToDuration(diff))

	// Should be able to measure sub-millisecond intervals
	if diff <= 0 {
		t.Error("Expected to measure non-zero time for operation")
	}
}

func TestMonoTime_Arithmetic(t *testing.T) {
	clk := NewSystemClock()

	t1 := clk.Now()
	delta := FromDuration(50 * time.Millisecond)
	t2 := t1 + delta

	if t2 != t1+delta {
		t.Error("MonoTime arithmetic failed")
	}

	diff := t2 - t1
	if ToDuration(diff) != 50*time.Millisecond {
		t.Errorf("Expected 50ms diff, got %v", ToDuration(diff))
	}
}

func TestMonoTime_ZeroValue(t *testing.T) {
	var zero MonoTime

	if zero != 0 {
		t.Errorf("Zero MonoTime should be 0, got %d", zero)
	}

	if ToDuration(zero) != 0 {
		t.Error("Zero MonoTime should convert to 0 duration")
	}
}

func TestSystemClock_MultipleInstances(t *testing.T) {
	clk1 := NewSystemClock()
	time.Sleep(5 * time.Millisecond)
	clk2 := NewSystemClock()

	// Each clock has its own epoch, so times aren't directly comparable
	t1 := clk1.Now()
	t2 := clk2.Now()

	t.Logf("Clock1 time: %d, Clock2 time: %d", t1, t2)

	// But both should advance from their respective epochs
	time.Sleep(10 * time.Millisecond)

	if clk1.Since(t1) < 10*time.Millisecond {
		t.Error("Clock1 didn't advance properly")
	}

	if clk2.Since(t2) < 10*time.Millisecond {
		t.Error("Clock2 didn't advance properly")
	}
}
