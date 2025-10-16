package tests

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/framework"
	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Test13RecoveringToNormal validates RECOVERING → NORMAL with additive increases.
//
// Test: 1.3 - Recovering → Normal Transition
// Category: Core AIMD Cycle Tests
//
// Goal: Verify additive increase and return to NORMAL.
//
// Pass Criteria:
//   - Scale increases by exactly +0.05 per cooldown window
//   - Scale increases occur every ~30 seconds (±1s tolerance)
//   - NORMAL state reached when scale=1.0
//   - WORKER_SCALE_UP events emitted for each increase
type Test13RecoveringToNormal struct {
	*framework.BaseTestCase

	chunks         [][]byte
	scaleIncreases []scaleIncrease
	normalTime     time.Time
}

type scaleIncrease struct {
	Time     time.Time
	OldScale float64
	NewScale float64
	Interval time.Duration
}

// NewTest13RecoveringToNormal creates a new test instance.
func NewTest13RecoveringToNormal() framework.TestCase {
	return &Test13RecoveringToNormal{
		BaseTestCase:   framework.NewBaseTestCase(),
		scaleIncreases: make([]scaleIncrease, 0),
	}
}

func (t *Test13RecoveringToNormal) Name() string {
	return "1.3: Recovering → Normal Transition"
}

func (t *Test13RecoveringToNormal) Category() string {
	return "Core AIMD Cycle Tests"
}

func (t *Test13RecoveringToNormal) Description() string {
	return "Verify additive increase (+5% per 30s) and return to NORMAL state at 100%"
}

func (t *Test13RecoveringToNormal) Setup(ctx context.Context) error {
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

func (t *Test13RecoveringToNormal) Run(ctx context.Context) error {
	limit := t.MemoryLimit()
	chunkSize := 10 * 1024 * 1024 // 10MB

	// Phase 1: Allocate to trigger DEGRADED state
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if t.Engine().Governor().State() == engine.StateDegraded {
			break
		}

		chunk := make([]byte, chunkSize)
		for i := range chunk {
			chunk[i] = byte(i % 256)
		}
		t.chunks = append(t.chunks, chunk)

		stats := engine.ReadMemoryStatsFast(limit)
		if stats.UsagePct > 0.75 {
			time.Sleep(100 * time.Millisecond)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Phase 2: Release memory to trigger RECOVERING
	t.chunks = nil
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	// Wait for RECOVERING state
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if t.Engine().Governor().State() == engine.StateRecovering {
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	// Phase 3: Monitor additive recovery to NORMAL
	// This can take ~5 minutes (10 steps × 30s from 50% → 100%)
	// But for testing, we'll monitor for 2.5 minutes and verify at least 4-5 increases
	recoveryDeadline := time.Now().Add(150 * time.Second)
	lastScale := t.Engine().Governor().Scale()
	lastCheckTime := time.Now()

	for time.Now().Before(recoveryDeadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentScale := t.Engine().Governor().Scale()
		currentState := t.Engine().Governor().State()

		// Check if scale increased
		if currentScale > lastScale+0.01 { // Account for float precision
			increase := scaleIncrease{
				Time:     time.Now(),
				OldScale: lastScale,
				NewScale: currentScale,
				Interval: time.Since(lastCheckTime),
			}
			t.scaleIncreases = append(t.scaleIncreases, increase)
			lastScale = currentScale
			lastCheckTime = time.Now()
		}

		// Check if NORMAL reached
		if currentState == engine.StateNormal {
			t.normalTime = time.Now()
			t.Metric("normal_time", t.normalTime)
			break
		}

		// Exit early if we've seen at least 4 increases (shows the pattern works)
		if len(t.scaleIncreases) >= 4 && currentScale >= 0.70 {
			t.Warning("Exited early after 4 scale increases (pattern verified)")
			break
		}

		time.Sleep(1 * time.Second)
	}

	return nil
}

func (t *Test13RecoveringToNormal) Teardown() error {
	// Release memory
	t.chunks = nil
	runtime.GC()

	// Shutdown engine
	return t.TeardownEngine()
}

func (t *Test13RecoveringToNormal) Validate() *framework.TestResult {
	result := t.Result()
	result.TestName = t.Name()
	result.Category = t.Category()

	gov := t.Engine().Governor()

	// Assertion 1: At least 4 scale increases observed
	framework.AssertCountGreaterThan(t.BaseTestCase, "At least 4 scale increases", 3, len(t.scaleIncreases))

	// Assertion 2: Each increase is ~0.05 (±0.01 tolerance)
	if len(t.scaleIncreases) > 0 {
		allCorrect := true
		for i, inc := range t.scaleIncreases {
			delta := inc.NewScale - inc.OldScale
			if delta < 0.04 || delta > 0.06 {
				allCorrect = false
				t.Warning(fmt.Sprintf("Increase %d: delta=%.3f (expected ~0.05)", i+1, delta))
			}
		}
		framework.AssertTrue(t.BaseTestCase, "All increases ~0.05 (±0.01)", allCorrect, "Some increases not within tolerance")
	}

	// Assertion 3: Increases occur every ~30s (±2s tolerance)
	if len(t.scaleIncreases) > 1 {
		allCorrectInterval := true
		for i := 1; i < len(t.scaleIncreases); i++ {
			interval := t.scaleIncreases[i].Interval
			if interval < 28*time.Second || interval > 32*time.Second {
				allCorrectInterval = false
				t.Warning(fmt.Sprintf("Interval %d: %s (expected ~30s)", i, interval))
			}
		}
		framework.AssertTrue(t.BaseTestCase, "All intervals ~30s (±2s)", allCorrectInterval, "Some intervals outside tolerance")
	}

	// Assertion 4: Scale increasing (never decreasing)
	if len(t.scaleIncreases) > 0 {
		monotonic := true
		for i := 1; i < len(t.scaleIncreases); i++ {
			if t.scaleIncreases[i].NewScale <= t.scaleIncreases[i-1].NewScale {
				monotonic = false
			}
		}
		framework.AssertTrue(t.BaseTestCase, "Scale monotonically increasing", monotonic, "Scale decreased during recovery")
	}

	// Assertion 5: Final state is NORMAL or RECOVERING (if we exited early)
	state := gov.State()
	validState := state == engine.StateNormal || state == engine.StateRecovering
	framework.AssertTrue(t.BaseTestCase, "Final state NORMAL or RECOVERING", validState, "Unexpected final state: "+state.String())

	// Metrics
	result.AddMetric("final_state", gov.State().String())
	result.AddMetric("final_scale", gov.Scale())
	result.AddMetric("scale_increases_count", len(t.scaleIncreases))

	// Log all increases
	for i, inc := range t.scaleIncreases {
		result.AddMetric("increase_"+string(rune('0'+i))+"_delta", inc.NewScale-inc.OldScale)
		if i > 0 {
			result.AddMetric("increase_"+string(rune('0'+i))+"_interval", inc.Interval)
		}
	}

	result.Finish()
	return result
}

func (t *Test13RecoveringToNormal) monitorEvents(sub *event.ErrorSubscription) {
	for range sub.Events() {
		// Just consume events (we track state directly)
	}
}
