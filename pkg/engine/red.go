package engine

import (
	"math/rand"
)

// REDDropper implements Random Early Detection for graceful degradation.
//
// RED prevents hard buffer saturation cliffs by probabilistically dropping
// events BEFORE hitting 100% capacity. This spreads the pain of overload
// across all publishers rather than blocking a few unlucky ones.
//
// Key behavior:
//   - Below minThreshold: Never drop (0% drop rate)
//   - Between min and max: Linear ramp (e.g., 60% → 30% drop prob at 100%)
//   - Above maxThreshold: Drop at maxDropProb (e.g., 30%)
//
// Inspired by TCP RED algorithm for congestion control.
type REDDropper struct {
	minThreshold float64 // Start dropping at this fill level (e.g., 0.6 = 60%)
	maxThreshold float64 // Max fill level for drop probability curve (e.g., 1.0 = 100%)
	maxDropProb  float64 // Maximum drop probability (e.g., 0.3 = 30%)

	rng *rand.Rand // Random number generator
}

// NewREDDropper creates a RED dropper with the given thresholds.
//
// Parameters:
//   - minThreshold: Start dropping at this fill level (0.0-1.0)
//   - maxThreshold: Max fill level for probability curve (0.0-1.0)
//   - maxDropProb: Maximum drop probability (0.0-1.0)
//
// Example:
//
//	red := NewREDDropper(0.6, 1.0, 0.3)
//	// Starts dropping at 60% full
//	// Reaches 30% drop probability at 100% full
func NewREDDropper(minThreshold, maxThreshold, maxDropProb float64) *REDDropper {
	return &REDDropper{
		minThreshold: minThreshold,
		maxThreshold: maxThreshold,
		maxDropProb:  maxDropProb,
		rng:          rand.New(rand.NewSource(rand.Int63())),
	}
}

// NewDefaultREDDropper creates a RED dropper with sensible defaults.
//
// Defaults:
//   - minThreshold: 0.6 (start dropping at 60% full)
//   - maxThreshold: 1.0 (max probability at 100% full)
//   - maxDropProb: 0.3 (drop 30% of events at max)
//
// This configuration provides smooth degradation:
//   - 60% full → 0% drop
//   - 70% full → 7.5% drop
//   - 80% full → 15% drop
//   - 90% full → 22.5% drop
//   - 100% full → 30% drop
func NewDefaultREDDropper() *REDDropper {
	return NewREDDropper(0.6, 1.0, 0.3)
}

// ShouldDrop returns true if an event should be dropped based on current fill level.
//
// This is a probabilistic decision:
//   - Returns false deterministically if fill <= minThreshold
//   - Returns true with probability DropProbability(fill) if fill > minThreshold
//
// Usage:
//
//	fill := float64(len(buffer)) / float64(cap(buffer))
//	if red.ShouldDrop(fill) {
//	    // Drop event
//	    droppedCounter.Inc()
//	    return
//	}
//	// Proceed with normal send
func (rd *REDDropper) ShouldDrop(fill float64) bool {
	prob := rd.DropProbability(fill)
	if prob == 0.0 {
		return false
	}

	return rd.rng.Float64() < prob
}

// DropProbability calculates the drop probability for a given fill level.
//
// Returns a value between 0.0 and maxDropProb based on linear interpolation:
//
//	if fill <= minThreshold: return 0.0
//	if fill >= maxThreshold: return maxDropProb
//	else: return linear_ramp(fill)
//
// The linear ramp formula:
//
//	excess = fill - minThreshold
//	range = maxThreshold - minThreshold
//	prob = (excess / range) * maxDropProb
//
// Example with defaults (min=0.6, max=1.0, maxProb=0.3):
//
//	DropProbability(0.5)  = 0.0    (below min)
//	DropProbability(0.6)  = 0.0    (at min)
//	DropProbability(0.7)  = 0.075  (25% into range)
//	DropProbability(0.8)  = 0.15   (50% into range)
//	DropProbability(0.9)  = 0.225  (75% into range)
//	DropProbability(1.0)  = 0.3    (at max)
//	DropProbability(1.1)  = 0.3    (clamped to max)
func (rd *REDDropper) DropProbability(fill float64) float64 {
	// Below minimum threshold - never drop
	if fill <= rd.minThreshold {
		return 0.0
	}

	// Above maximum threshold - drop at max probability
	if fill >= rd.maxThreshold {
		return rd.maxDropProb
	}

	// Linear ramp between min and max
	rangeSize := rd.maxThreshold - rd.minThreshold
	excess := fill - rd.minThreshold

	return (excess / rangeSize) * rd.maxDropProb
}

// MinThreshold returns the configured minimum threshold.
func (rd *REDDropper) MinThreshold() float64 {
	return rd.minThreshold
}

// MaxThreshold returns the configured maximum threshold.
func (rd *REDDropper) MaxThreshold() float64 {
	return rd.maxThreshold
}

// MaxDropProb returns the configured maximum drop probability.
func (rd *REDDropper) MaxDropProb() float64 {
	return rd.maxDropProb
}
