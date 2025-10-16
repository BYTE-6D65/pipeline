package engine

import (
	"math"
	"testing"
)

func TestREDDropper_DropProbability(t *testing.T) {
	red := NewDefaultREDDropper()

	tests := []struct {
		fill     float64
		expected float64
		desc     string
	}{
		{0.0, 0.0, "empty buffer"},
		{0.5, 0.0, "below minimum threshold"},
		{0.6, 0.0, "exactly at minimum threshold"},
		{0.7, 0.075, "25% into range (0.6-1.0)"},
		{0.8, 0.15, "50% into range"},
		{0.9, 0.225, "75% into range"},
		{1.0, 0.3, "at maximum threshold"},
		{1.1, 0.3, "above maximum threshold (clamped)"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			prob := red.DropProbability(tt.fill)
			if math.Abs(prob-tt.expected) > 0.0001 {
				t.Errorf("DropProbability(%.1f) = %.3f, want %.3f",
					tt.fill, prob, tt.expected)
			}
		})
	}
}

func TestREDDropper_CustomThresholds(t *testing.T) {
	// Custom: start at 50%, max at 90%, max drop prob 50%
	red := NewREDDropper(0.5, 0.9, 0.5)

	tests := []struct {
		fill     float64
		expected float64
	}{
		{0.4, 0.0},    // Below min
		{0.5, 0.0},    // At min
		{0.7, 0.25},   // 50% into (0.5-0.9) range
		{0.9, 0.5},    // At max
		{1.0, 0.5},    // Above max (clamped)
	}

	for _, tt := range tests {
		prob := red.DropProbability(tt.fill)
		if math.Abs(prob-tt.expected) > 0.0001 {
			t.Errorf("DropProbability(%.1f) = %.3f, want %.3f",
				tt.fill, prob, tt.expected)
		}
	}
}

func TestREDDropper_ShouldDrop_BelowThreshold(t *testing.T) {
	red := NewDefaultREDDropper()

	// Below minimum threshold - should NEVER drop
	for i := 0; i < 1000; i++ {
		if red.ShouldDrop(0.5) {
			t.Fatal("ShouldDrop(0.5) returned true, expected always false below threshold")
		}
	}
}

func TestREDDropper_ShouldDrop_AtMax(t *testing.T) {
	red := NewDefaultREDDropper()

	// At maximum threshold - should drop ~30% of the time
	drops := 0
	trials := 10000

	for i := 0; i < trials; i++ {
		if red.ShouldDrop(1.0) {
			drops++
		}
	}

	dropRate := float64(drops) / float64(trials)
	expectedRate := 0.3

	// Allow 5% variance for statistical fluctuation
	if math.Abs(dropRate-expectedRate) > 0.05 {
		t.Errorf("Drop rate at 100%% fill = %.3f, want ~%.3f (±0.05)",
			dropRate, expectedRate)
	}
}

func TestREDDropper_ShouldDrop_Midpoint(t *testing.T) {
	red := NewDefaultREDDropper()

	// At midpoint (80%) - should drop ~15% of the time
	drops := 0
	trials := 10000

	for i := 0; i < trials; i++ {
		if red.ShouldDrop(0.8) {
			drops++
		}
	}

	dropRate := float64(drops) / float64(trials)
	expectedRate := 0.15

	// Allow 5% variance
	if math.Abs(dropRate-expectedRate) > 0.05 {
		t.Errorf("Drop rate at 80%% fill = %.3f, want ~%.3f (±0.05)",
			dropRate, expectedRate)
	}
}

func TestREDDropper_LinearRamp(t *testing.T) {
	red := NewDefaultREDDropper()

	// Verify that drop probability increases linearly
	fills := []float64{0.6, 0.7, 0.8, 0.9, 1.0}
	probs := make([]float64, len(fills))

	for i, fill := range fills {
		probs[i] = red.DropProbability(fill)
	}

	// Check that each step increases by the same amount
	expectedStep := 0.075 // (0.3 - 0.0) / 4 steps

	for i := 1; i < len(probs); i++ {
		actualStep := probs[i] - probs[i-1]
		if math.Abs(actualStep-expectedStep) > 0.0001 {
			t.Errorf("Step %d: probability increase = %.4f, want %.4f (linear ramp)",
				i, actualStep, expectedStep)
		}
	}
}

func TestREDDropper_Getters(t *testing.T) {
	red := NewREDDropper(0.5, 0.9, 0.4)

	if red.MinThreshold() != 0.5 {
		t.Errorf("MinThreshold() = %.2f, want 0.5", red.MinThreshold())
	}

	if red.MaxThreshold() != 0.9 {
		t.Errorf("MaxThreshold() = %.2f, want 0.9", red.MaxThreshold())
	}

	if red.MaxDropProb() != 0.4 {
		t.Errorf("MaxDropProb() = %.2f, want 0.4", red.MaxDropProb())
	}
}

func TestREDDropper_DefaultValues(t *testing.T) {
	red := NewDefaultREDDropper()

	if red.MinThreshold() != 0.6 {
		t.Errorf("Default MinThreshold = %.2f, want 0.6", red.MinThreshold())
	}

	if red.MaxThreshold() != 1.0 {
		t.Errorf("Default MaxThreshold = %.2f, want 1.0", red.MaxThreshold())
	}

	if red.MaxDropProb() != 0.3 {
		t.Errorf("Default MaxDropProb = %.2f, want 0.3", red.MaxDropProb())
	}
}

func TestREDDropper_ZeroRange(t *testing.T) {
	// Edge case: min == max (shouldn't happen in practice, but test for safety)
	red := NewREDDropper(0.8, 0.8, 0.5)

	// Below threshold
	if prob := red.DropProbability(0.7); prob != 0.0 {
		t.Errorf("DropProbability(0.7) with min=max=0.8 = %.2f, want 0.0", prob)
	}

	// Above threshold
	if prob := red.DropProbability(0.9); prob != 0.5 {
		t.Errorf("DropProbability(0.9) with min=max=0.8 = %.2f, want 0.5", prob)
	}
}

// Benchmark probabilistic drop decision
func BenchmarkREDDropper_ShouldDrop(b *testing.B) {
	red := NewDefaultREDDropper()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		red.ShouldDrop(0.8)
	}
}

// Benchmark drop probability calculation
func BenchmarkREDDropper_DropProbability(b *testing.B) {
	red := NewDefaultREDDropper()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		red.DropProbability(0.8)
	}
}
