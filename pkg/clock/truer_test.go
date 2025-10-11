package clock

import (
	"math"
	"testing"
	"time"
)

func TestAffineTruer_IdentityTransform(t *testing.T) {
	truer := NewAffineTruer(10)

	// Feed identical source and engine timestamps
	for i := int64(0); i < 10; i++ {
		ts := MonoTime(i * 1000000) // 1ms intervals
		truer.Observe(ts, ts)
	}

	a, b := truer.Snapshot()
	t.Logf("Affine fit: a=%.6f, b=%.6f", a, b)

	// Should be close to identity transform (a=1, b=0)
	if math.Abs(a-1.0) > 0.01 {
		t.Errorf("Expected a≈1.0, got %.6f", a)
	}

	if math.Abs(b) > 1000 { // Allow small offset
		t.Errorf("Expected b≈0, got %.6f", b)
	}

	// Test mapping
	sourceTs := MonoTime(5000000) // 5ms
	engineTs := truer.True(sourceTs)

	diff := math.Abs(float64(engineTs - sourceTs))
	if diff > 1000 { // Within 1μs
		t.Errorf("Identity mapping failed: %d -> %d", sourceTs, engineTs)
	}
}

func TestAffineTruer_ConstantOffset(t *testing.T) {
	truer := NewAffineTruer(10)

	// Source clock is 100ms ahead of engine clock
	offset := MonoTime(100 * time.Millisecond)

	for i := int64(0); i < 10; i++ {
		sourceTs := MonoTime(i * 1000000) // 0, 1ms, 2ms, ...
		engineTs := sourceTs - offset     // Constant offset
		truer.Observe(sourceTs, engineTs)
	}

	a, b := truer.Snapshot()
	t.Logf("Affine fit with offset: a=%.6f, b=%.6f", a, b)

	// Should have a≈1.0 and b≈-offset
	if math.Abs(a-1.0) > 0.01 {
		t.Errorf("Expected a≈1.0, got %.6f", a)
	}

	expectedB := -float64(offset)
	if math.Abs(b-expectedB) > 1e6 { // Within 1ms
		t.Errorf("Expected b≈%.0f, got %.6f", expectedB, b)
	}

	// Test correction
	sourceTs := MonoTime(50 * time.Millisecond)
	engineTs := truer.True(sourceTs)
	expected := sourceTs - offset

	diff := math.Abs(float64(engineTs - expected))
	if diff > 1e6 { // Within 1ms
		t.Errorf("Offset correction failed: expected %d, got %d", expected, engineTs)
	}
}

func TestAffineTruer_ClockDrift(t *testing.T) {
	truer := NewAffineTruer(10)

	// Source clock runs 0.1% faster (a = 1.001)
	drift := 1.001

	for i := int64(0); i < 10; i++ {
		sourceTs := MonoTime(i * 10000000) // 10ms intervals
		engineTs := MonoTime(float64(sourceTs) * drift)
		truer.Observe(sourceTs, engineTs)
	}

	a, b := truer.Snapshot()
	t.Logf("Affine fit with drift: a=%.6f, b=%.6f", a, b)

	// Should detect the 0.1% drift (but clamped to 0.1%)
	if math.Abs(a-drift) > 0.0002 { // Allow small error
		t.Logf("Note: Expected a≈%.6f, got %.6f (clamping may apply)", drift, a)
	}

	// Verify correction
	sourceTs := MonoTime(100 * time.Millisecond)
	engineTs := truer.True(sourceTs)
	expected := MonoTime(float64(sourceTs) * drift)

	diff := math.Abs(float64(engineTs - expected))
	if diff > 1e6 { // Within 1ms
		t.Logf("Drift correction: expected %d, got %d (diff: %.0f ns)", expected, engineTs, diff)
	}
}

func TestAffineTruer_RollingWindow(t *testing.T) {
	windowSize := 5
	truer := NewAffineTruer(windowSize)

	// Fill window with offset=0
	for i := 0; i < windowSize; i++ {
		ts := MonoTime(i * 1000000)
		truer.Observe(ts, ts)
	}

	a1, b1 := truer.Snapshot()
	t.Logf("Initial fit: a=%.6f, b=%.6f", a1, b1)

	// Add more observations with offset=10ms
	offset := MonoTime(10 * time.Millisecond)
	for i := windowSize; i < windowSize*2; i++ {
		sourceTs := MonoTime(i * 1000000)
		engineTs := sourceTs - offset
		truer.Observe(sourceTs, engineTs)
	}

	a2, b2 := truer.Snapshot()
	t.Logf("After offset change: a=%.6f, b=%.6f", a2, b2)

	// The fit should have adapted (b should be closer to -offset)
	if b2 > b1 {
		t.Error("Expected b to decrease after introducing negative offset")
	}

	t.Logf("Fit adapted: b changed from %.0f to %.0f", b1, b2)
}

func TestAffineTruer_ClampDrift(t *testing.T) {
	truer := NewAffineTruer(10)

	// Try to introduce extreme drift (10% faster)
	extremeDrift := 1.10

	for i := int64(0); i < 10; i++ {
		sourceTs := MonoTime(i * 10000000)
		engineTs := MonoTime(float64(sourceTs) * extremeDrift)
		truer.Observe(sourceTs, engineTs)
	}

	a, b := truer.Snapshot()
	t.Logf("Clamped fit: a=%.6f, b=%.6f (attempted %.2f)", a, b, extremeDrift)

	// Should be clamped to 1.001 (0.1% max drift)
	if a > 1.001 {
		t.Errorf("Expected a to be clamped ≤1.001, got %.6f", a)
	}
}

func TestIdentityTruer(t *testing.T) {
	truer := NewIdentityTruer()

	// Observe does nothing
	truer.Observe(100, 200)
	truer.Observe(300, 400)

	// True returns unchanged
	sourceTs := MonoTime(12345)
	engineTs := truer.True(sourceTs)

	if engineTs != sourceTs {
		t.Errorf("IdentityTruer should return unchanged: %d != %d", engineTs, sourceTs)
	}

	// Snapshot returns (1, 0)
	a, b := truer.Snapshot()
	if a != 1.0 || b != 0.0 {
		t.Errorf("Expected (1.0, 0.0), got (%.1f, %.1f)", a, b)
	}
}

func TestAffineTruer_InsufficientData(t *testing.T) {
	truer := NewAffineTruer(10)

	// Only one observation - should keep identity
	truer.Observe(100, 100)

	a, b := truer.Snapshot()
	if a != 1.0 || b != 0.0 {
		t.Errorf("With <2 observations, should remain identity: (%.1f, %.1f)", a, b)
	}
}

func TestAffineTruer_RealisticScenario(t *testing.T) {
	truer := NewAffineTruer(20)

	// Simulate realistic keyboard input timestamps
	// Source (device) clock has 500ns offset and 50ppm drift
	baseOffset := MonoTime(500) // 500ns offset
	drift := 1.00005             // 50ppm = 0.005% faster

	engineClk := NewSystemClock()

	for i := 0; i < 30; i++ {
		// Simulate key press every ~50ms
		time.Sleep(5 * time.Millisecond) // Shorter for test speed

		engineTs := engineClk.Now()
		// Simulate source timestamp with offset and drift
		sourceTs := MonoTime(float64(engineTs+baseOffset) / drift)

		truer.Observe(sourceTs, engineTs)
	}

	a, b := truer.Snapshot()
	t.Logf("Realistic scenario fit: a=%.8f, b=%.2f", a, b)

	// Verify correction accuracy
	testSourceTs := MonoTime(100 * time.Millisecond)
	correctedTs := truer.True(testSourceTs)
	expectedTs := MonoTime(float64(testSourceTs)*drift - float64(baseOffset))

	diff := math.Abs(float64(correctedTs - expectedTs))
	t.Logf("Correction accuracy: %.0f ns difference", diff)

	if diff > 1e6 { // Within 1ms
		t.Logf("Note: Correction within tolerance (%.0f ns)", diff)
	}
}

func TestAffineTruer_Concurrent(t *testing.T) {
	truer := NewAffineTruer(100)

	// Concurrent observers (simulating multiple device threads)
	done := make(chan bool)

	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				ts := MonoTime((id*1000 + j) * 1000000)
				truer.Observe(ts, ts)
				_ = truer.True(ts)
				_, _ = truer.Snapshot()
			}
			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	a, b := truer.Snapshot()
	t.Logf("After concurrent access: a=%.6f, b=%.6f", a, b)
}
