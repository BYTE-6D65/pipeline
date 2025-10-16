package framework

import (
	"fmt"
	"math"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/engine"
)

// AssertStateEquals checks if governor state matches expected.
func AssertStateEquals(tc *BaseTestCase, name string, expected, actual engine.GovernorState) {
	passed := expected == actual
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected state %s, got %s", expected, actual)
	}
	tc.Assert(name, expected.String(), actual.String(), passed, message)
}

// AssertScaleEquals checks if governor scale matches expected (exact).
func AssertScaleEquals(tc *BaseTestCase, name string, expected, actual float64) {
	passed := expected == actual
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected scale %.2f, got %.2f", expected, actual)
	}
	tc.Assert(name, expected, actual, passed, message)
}

// AssertScaleNear checks if governor scale is within tolerance of expected.
func AssertScaleNear(tc *BaseTestCase, name string, expected, actual, tolerance float64) {
	diff := math.Abs(expected - actual)
	passed := diff <= tolerance
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected scale %.2f (±%.2f), got %.2f (diff: %.2f)", expected, tolerance, actual, diff)
	}
	tc.Assert(name, fmt.Sprintf("%.2f ± %.2f", expected, tolerance), actual, passed, message)
}

// AssertScaleGreaterThan checks if scale is greater than minimum.
func AssertScaleGreaterThan(tc *BaseTestCase, name string, min, actual float64) {
	passed := actual > min
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected scale > %.2f, got %.2f", min, actual)
	}
	tc.Assert(name, fmt.Sprintf("> %.2f", min), actual, passed, message)
}

// AssertScaleLessThan checks if scale is less than maximum.
func AssertScaleLessThan(tc *BaseTestCase, name string, max, actual float64) {
	passed := actual < max
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected scale < %.2f, got %.2f", max, actual)
	}
	tc.Assert(name, fmt.Sprintf("< %.2f", max), actual, passed, message)
}

// AssertScaleInRange checks if scale is within range [min, max].
func AssertScaleInRange(tc *BaseTestCase, name string, min, max, actual float64) {
	passed := actual >= min && actual <= max
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected scale in [%.2f, %.2f], got %.2f", min, max, actual)
	}
	tc.Assert(name, fmt.Sprintf("[%.2f, %.2f]", min, max), actual, passed, message)
}

// AssertDurationLessThan checks if duration is less than max.
func AssertDurationLessThan(tc *BaseTestCase, name string, max, actual time.Duration) {
	passed := actual < max
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected duration < %s, got %s", max, actual)
	}
	tc.Assert(name, fmt.Sprintf("< %s", max), actual.String(), passed, message)
}

// AssertDurationInRange checks if duration is within range.
func AssertDurationInRange(tc *BaseTestCase, name string, min, max, actual time.Duration) {
	passed := actual >= min && actual <= max
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected duration in [%s, %s], got %s", min, max, actual)
	}
	tc.Assert(name, fmt.Sprintf("[%s, %s]", min, max), actual.String(), passed, message)
}

// AssertMemoryPressureGreaterThan checks if memory pressure exceeds threshold.
func AssertMemoryPressureGreaterThan(tc *BaseTestCase, name string, threshold float64, limit uint64) {
	stats := engine.ReadMemoryStatsFast(limit)
	actual := stats.UsagePct
	passed := actual >= threshold
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected memory pressure >= %.1f%%, got %.1f%%", threshold*100, actual*100)
	}
	tc.Assert(name, fmt.Sprintf(">= %.1f%%", threshold*100), fmt.Sprintf("%.1f%%", actual*100), passed, message)
}

// AssertMemoryPressureLessThan checks if memory pressure is below threshold.
func AssertMemoryPressureLessThan(tc *BaseTestCase, name string, threshold float64, limit uint64) {
	stats := engine.ReadMemoryStatsFast(limit)
	actual := stats.UsagePct
	passed := actual < threshold
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected memory pressure < %.1f%%, got %.1f%%", threshold*100, actual*100)
	}
	tc.Assert(name, fmt.Sprintf("< %.1f%%", threshold*100), fmt.Sprintf("%.1f%%", actual*100), passed, message)
}

// AssertTrue checks if condition is true.
func AssertTrue(tc *BaseTestCase, name string, condition bool, message string) {
	tc.Assert(name, true, condition, condition, message)
}

// AssertFalse checks if condition is false.
func AssertFalse(tc *BaseTestCase, name string, condition bool, message string) {
	tc.Assert(name, false, condition, !condition, message)
}

// AssertEquals checks if two values are equal.
func AssertEquals(tc *BaseTestCase, name string, expected, actual interface{}) {
	passed := expected == actual
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected %v, got %v", expected, actual)
	}
	tc.Assert(name, expected, actual, passed, message)
}

// AssertNotEquals checks if two values are not equal.
func AssertNotEquals(tc *BaseTestCase, name string, notExpected, actual interface{}) {
	passed := notExpected != actual
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected not %v, but got %v", notExpected, actual)
	}
	tc.Assert(name, fmt.Sprintf("!= %v", notExpected), actual, passed, message)
}

// AssertCountEquals checks if count matches expected.
func AssertCountEquals(tc *BaseTestCase, name string, expected, actual int) {
	passed := expected == actual
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected %d, got %d", expected, actual)
	}
	tc.Assert(name, expected, actual, passed, message)
}

// AssertCountGreaterThan checks if count is greater than minimum.
func AssertCountGreaterThan(tc *BaseTestCase, name string, min, actual int) {
	passed := actual > min
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected count > %d, got %d", min, actual)
	}
	tc.Assert(name, fmt.Sprintf("> %d", min), actual, passed, message)
}

// AssertCountInRange checks if count is within range.
func AssertCountInRange(tc *BaseTestCase, name string, min, max, actual int) {
	passed := actual >= min && actual <= max
	message := ""
	if !passed {
		message = fmt.Sprintf("Expected count in [%d, %d], got %d", min, max, actual)
	}
	tc.Assert(name, fmt.Sprintf("[%d, %d]", min, max), actual, passed, message)
}
