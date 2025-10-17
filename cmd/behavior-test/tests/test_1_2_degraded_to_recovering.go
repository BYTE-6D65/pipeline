package tests

import (
	"context"
	"runtime"
	"time"

	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/framework"
	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Test12DegradedToRecovering validates DEGRADED → RECOVERING transition when pressure drops.
//
// Test: 1.2 - Degraded → Recovering Transition
// Category: Core AIMD Cycle Tests
//
// Goal: Verify governor exits degraded mode when pressure drops below 55%.
//
// Pass Criteria:
//   - RECOVERING state reached within 2 poll intervals (100ms)
//   - Scale remains at last degraded value (no change on transition)
//   - State change event emitted
type Test12DegradedToRecovering struct {
	*framework.BaseTestCase

	chunks            [][]byte
	degradedScale     float64
	reliefTime        time.Time
	recoveringTime    time.Time
	recoveringModeSeen bool
}

// NewTest12DegradedToRecovering creates a new test instance.
func NewTest12DegradedToRecovering() framework.TestCase {
	return &Test12DegradedToRecovering{
		BaseTestCase: framework.NewBaseTestCase(),
	}
}

func (t *Test12DegradedToRecovering) Name() string {
	return "1.2: Degraded → Recovering Transition"
}

func (t *Test12DegradedToRecovering) Category() string {
	return "Core AIMD Cycle Tests"
}

func (t *Test12DegradedToRecovering) Description() string {
	return "Verify governor exits degraded mode when memory pressure drops below 55%"
}

func (t *Test12DegradedToRecovering) Setup(ctx context.Context) error {
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

func (t *Test12DegradedToRecovering) Run(ctx context.Context) error {
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
			t.degradedScale = t.Engine().Governor().Scale()
			t.Metric("degraded_scale", t.degradedScale)
			break
		}

		// Allocate another chunk
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
	runtime.GC() // Second GC for thoroughness

	// Record relief time
	stats := engine.ReadMemoryStatsFast(limit)
	if stats.UsagePct < 0.55 {
		t.reliefTime = time.Now()
		t.Metric("relief_time", t.reliefTime)
		t.Metric("relief_pressure_pct", stats.UsagePct)
	}

	// Phase 3: Wait for RECOVERING state
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if t.Engine().Governor().State() == engine.StateRecovering {
			t.recoveringTime = time.Now()
			t.Metric("recovering_time", t.recoveringTime)
			break
		}

		time.Sleep(10 * time.Millisecond)
	}

	// Give events time to propagate
	time.Sleep(200 * time.Millisecond)

	return nil
}

func (t *Test12DegradedToRecovering) Teardown() error {
	// Release any remaining memory
	t.chunks = nil
	runtime.GC()

	// Shutdown engine
	return t.TeardownEngine()
}

func (t *Test12DegradedToRecovering) Validate() *framework.TestResult {
	result := t.Result()
	result.TestName = t.Name()
	result.Category = t.Category()

	gov := t.Engine().Governor()

	// Assertion 1: RECOVERING state reached
	framework.AssertStateEquals(t.BaseTestCase, "Governor entered RECOVERING state", engine.StateRecovering, gov.State())

	// Assertion 2: Scale unchanged from DEGRADED (no change on state transition)
	if t.degradedScale > 0 {
		framework.AssertScaleEquals(t.BaseTestCase, "Scale unchanged on transition", t.degradedScale, gov.Scale())
	} else {
		t.Warning("Could not capture degraded scale")
	}

	// Assertion 3: Transition within 100ms (2 poll intervals @ 50ms)
	if !t.reliefTime.IsZero() && !t.recoveringTime.IsZero() {
		transitionDuration := t.recoveringTime.Sub(t.reliefTime)
		result.AddMetric("transition_duration", transitionDuration)
		framework.AssertDurationLessThan(t.BaseTestCase, "Transition within 100ms", 100*time.Millisecond, transitionDuration)
	} else {
		t.Warning("Could not measure transition duration")
	}

	// Assertion 4: RECOVERING event emitted
	framework.AssertTrue(t.BaseTestCase, "RECOVERING event emitted", t.recoveringModeSeen, "No RECOVERING state change event observed")

	// Metrics
	result.AddMetric("final_state", gov.State().String())
	result.AddMetric("final_scale", gov.Scale())

	result.Finish()
	return result
}

func (t *Test12DegradedToRecovering) monitorEvents(sub *event.ErrorSubscription) {
	for evt := range sub.Events() {
		// Track RECOVERING state change
		if evt.Code == event.CodeDegradedMode {
			if state, ok := evt.Context["state"].(string); ok && state == "RECOVERING" {
				t.recoveringModeSeen = true
			}
		}
	}
}
