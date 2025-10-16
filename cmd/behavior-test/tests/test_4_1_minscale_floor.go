package tests

import (
	"context"
	"runtime"
	"time"

	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/framework"
	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Test41MinScaleFloor validates scale never drops below 20%.
//
// Test: 4.1 - MinScale Floor at 20%
// Category: MinScale Floor Tests
//
// Goal: Verify scale never drops below 20%.
//
// Pass Criteria:
//   - First decrease: 50%
//   - Second decrease: 25%
//   - Third decrease: 20% (clamped, not 12.5%)
//   - Fourth decrease: 20% (stays at floor)
//   - Scale never goes below 0.20
type Test41MinScaleFloor struct {
	*framework.BaseTestCase

	chunks         [][]byte
	scaleDecreases []floorScaleDecrease
	minScaleSeen   float64
}

type floorScaleDecrease struct {
	Time     time.Time
	OldScale float64
	NewScale float64
	Pressure float64
}

// NewTest41MinScaleFloor creates a new test instance.
func NewTest41MinScaleFloor() framework.TestCase {
	return &Test41MinScaleFloor{
		BaseTestCase:   framework.NewBaseTestCase(),
		scaleDecreases: make([]floorScaleDecrease, 0),
		minScaleSeen:   1.0,
	}
}

func (t *Test41MinScaleFloor) Name() string {
	return "4.1: MinScale Floor at 20%"
}

func (t *Test41MinScaleFloor) Category() string {
	return "MinScale Floor Tests"
}

func (t *Test41MinScaleFloor) Description() string {
	return "Verify scale never drops below 20% minimum (prevents starvation)"
}

func (t *Test41MinScaleFloor) Setup(ctx context.Context) error {
	// Load config
	cfg, err := engine.LoadFromEnv()
	if err != nil {
		cfg = engine.DefaultConfig()
	}

	// Setup engine
	if err := t.SetupEngine(ctx, cfg); err != nil {
		return err
	}

	// Subscribe to control events
	sub, err := t.Engine().ErrorBus().Subscribe(ctx)
	if err != nil {
		return err
	}

	// Start event monitor
	go t.monitorEvents(sub)

	return nil
}

func (t *Test41MinScaleFloor) Run(ctx context.Context) error {
	limit := t.MemoryLimit()
	chunkSize := 10 * 1024 * 1024 // 10MB

	// Monitor scale changes
	lastScale := 1.0
	lastDecreaseTime := time.Time{}
	testStart := time.Now()

	// Allocate memory and wait for decreases
	// We need to trigger at least 3 decreases to hit the floor
	// Each decrease takes 30s cooldown, so this will run for ~2 minutes
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentScale := t.Engine().Governor().Scale()
		stats := engine.ReadMemoryStatsFast(limit)

		// Track minimum scale seen
		if currentScale < t.minScaleSeen {
			t.minScaleSeen = currentScale
		}

		// Detect scale decrease
		if currentScale < lastScale-0.01 { // Account for float precision
			decrease := floorScaleDecrease{
				Time:     time.Now(),
				OldScale: lastScale,
				NewScale: currentScale,
				Pressure: stats.UsagePct,
			}

			t.scaleDecreases = append(t.scaleDecreases, decrease)
			lastScale = currentScale
			lastDecreaseTime = time.Now()
		}

		// Stop conditions:
		// 1. We've seen 4+ decreases (enough to prove floor), OR
		// 2. Scale is at 0.20 and we've waited 35 seconds for another attempt, OR
		// 3. Test has run for 3 minutes (safety limit)
		if len(t.scaleDecreases) >= 4 {
			break
		}
		if currentScale <= 0.21 && !lastDecreaseTime.IsZero() && time.Since(lastDecreaseTime) > 35*time.Second {
			break
		}
		if time.Since(testStart) > 180*time.Second {
			t.Warning("Test timeout after 3 minutes")
			break
		}

		// Continue allocating to maintain high pressure
		if stats.UsagePct < 0.93 {
			chunk := make([]byte, chunkSize)
			for i := range chunk {
				chunk[i] = byte(i % 256)
			}
			t.chunks = append(t.chunks, chunk)
		}

		time.Sleep(200 * time.Millisecond)
	}

	// Give events time to propagate
	time.Sleep(200 * time.Millisecond)

	return nil
}

func (t *Test41MinScaleFloor) Teardown() error {
	// Release memory
	t.chunks = nil
	runtime.GC()

	// Shutdown engine
	return t.TeardownEngine()
}

func (t *Test41MinScaleFloor) Validate() *framework.TestResult {
	result := t.Result()
	result.TestName = t.Name()
	result.Category = t.Category()

	// Assertion 1: At least 3 decreases observed
	framework.AssertCountGreaterThan(t.BaseTestCase, "At least 3 decreases", 2, len(t.scaleDecreases))

	// Assertion 2: First decrease to 0.50
	if len(t.scaleDecreases) >= 1 {
		framework.AssertScaleNear(t.BaseTestCase, "First decrease to 0.50", 0.50, t.scaleDecreases[0].NewScale, 0.01)
	}

	// Assertion 3: Second decrease to 0.25
	if len(t.scaleDecreases) >= 2 {
		framework.AssertScaleNear(t.BaseTestCase, "Second decrease to 0.25", 0.25, t.scaleDecreases[1].NewScale, 0.01)
	}

	// Assertion 4: Third decrease to 0.20 (clamped, not 0.125)
	if len(t.scaleDecreases) >= 3 {
		framework.AssertScaleNear(t.BaseTestCase, "Third decrease to 0.20 (clamped)", 0.20, t.scaleDecreases[2].NewScale, 0.01)
	}

	// Assertion 5: If fourth decrease exists, it stays at 0.20
	if len(t.scaleDecreases) >= 4 {
		framework.AssertScaleNear(t.BaseTestCase, "Fourth decrease stays at 0.20 (floor)", 0.20, t.scaleDecreases[3].NewScale, 0.01)
	}

	// Assertion 6: Minimum scale never went below 0.20
	framework.AssertScaleGreaterThan(t.BaseTestCase, "Minimum scale ≥ 0.20", 0.19, t.minScaleSeen)

	// Assertion 7: Scale never went below 0.19 (with small tolerance for float precision)
	finalScale := t.Engine().Governor().Scale()
	framework.AssertScaleGreaterThan(t.BaseTestCase, "Final scale ≥ 0.19", 0.19, finalScale)

	// Metrics
	result.AddMetric("decrease_count", len(t.scaleDecreases))
	result.AddMetric("min_scale_seen", t.minScaleSeen)
	result.AddMetric("final_scale", finalScale)

	for i, dec := range t.scaleDecreases {
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_old", dec.OldScale)
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_new", dec.NewScale)
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_pressure", dec.Pressure)
	}

	result.Finish()
	return result
}

func (t *Test41MinScaleFloor) monitorEvents(sub *event.ErrorSubscription) {
	for range sub.Events() {
		// Just consume events (we track scale directly)
	}
}
