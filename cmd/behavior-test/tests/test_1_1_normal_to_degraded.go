package tests

import (
	"context"
	"runtime"
	"time"

	"github.com/BYTE-6D65/pipeline/cmd/behavior-test/framework"
	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Test11NormalToDegraded validates NORMAL → DEGRADED transition at 70% memory.
//
// Test: 1.1 - Normal → Degraded Transition
// Category: Core AIMD Cycle Tests
//
// Goal: Verify governor enters degraded mode at 70% memory pressure.
//
// Pass Criteria:
//   - DEGRADED state reached within 500ms of crossing 70%
//   - Scale exactly 0.50 (×0.5 multiplicative decrease)
//   - DEGRADED_MODE event emitted
//   - WORKER_SCALE_DOWN event emitted
type Test11NormalToDegraded struct {
	*framework.BaseTestCase

	chunks           [][]byte
	degradedTime     time.Time
	pressureTime     time.Time
	scaleDownSeen    bool
	degradedModeSeen bool
}

// NewTest11NormalToDegraded creates a new test instance.
func NewTest11NormalToDegraded() framework.TestCase {
	return &Test11NormalToDegraded{
		BaseTestCase: framework.NewBaseTestCase(),
	}
}

func (t *Test11NormalToDegraded) Name() string {
	return "1.1: Normal → Degraded Transition"
}

func (t *Test11NormalToDegraded) Category() string {
	return "Core AIMD Cycle Tests"
}

func (t *Test11NormalToDegraded) Description() string {
	return "Verify governor enters degraded mode at 70% memory pressure with correct scale reduction"
}

func (t *Test11NormalToDegraded) Setup(ctx context.Context) error {
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

func (t *Test11NormalToDegraded) Run(ctx context.Context) error {
	limit := t.MemoryLimit()
	targetPressure := 0.72 // Slightly over 70% to ensure trigger
	targetBytes := uint64(float64(limit) * targetPressure)
	chunkSize := 10 * 1024 * 1024 // 10MB

	// Allocate memory until we reach DEGRADED state
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check if DEGRADED reached
		if t.Engine().Governor().State() == engine.StateDegraded {
			t.degradedTime = time.Now()
			t.Metric("degraded_time", t.degradedTime)
			break
		}

		// Get current pressure
		stats := engine.ReadMemoryStatsFast(limit)

		// Record when we cross 70% threshold
		if stats.UsagePct >= 0.70 && t.pressureTime.IsZero() {
			t.pressureTime = time.Now()
			t.Metric("pressure_time", t.pressureTime)
			t.Metric("pressure_pct", stats.UsagePct)
		}

		// Allocate another chunk
		chunk := make([]byte, chunkSize)
		for i := range chunk {
			chunk[i] = byte(i % 256)
		}
		t.chunks = append(t.chunks, chunk)

		// Safety: stop if we allocated too much
		if stats.HeapAlloc > targetBytes {
			time.Sleep(100 * time.Millisecond) // Give governor time to react
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Give events time to propagate
	time.Sleep(200 * time.Millisecond)

	return nil
}

func (t *Test11NormalToDegraded) Teardown() error {
	// Release memory
	t.chunks = nil
	runtime.GC()

	// Shutdown engine
	return t.TeardownEngine()
}

func (t *Test11NormalToDegraded) Validate() *framework.TestResult {
	result := t.Result()
	result.TestName = t.Name()
	result.Category = t.Category()

	gov := t.Engine().Governor()

	// Assertion 1: DEGRADED state reached
	framework.AssertStateEquals(t.BaseTestCase, "Governor entered DEGRADED state", engine.StateDegraded, gov.State())

	// Assertion 2: Scale is 0.50 (multiplicative decrease)
	framework.AssertScaleEquals(t.BaseTestCase, "Scale reduced to 0.50 (×0.5)", 0.50, gov.Scale())

	// Assertion 3: Transition happened within 500ms of crossing 70%
	if !t.pressureTime.IsZero() && !t.degradedTime.IsZero() {
		transitionDuration := t.degradedTime.Sub(t.pressureTime)
		result.AddMetric("transition_duration", transitionDuration)
		framework.AssertDurationLessThan(t.BaseTestCase, "Transition within 500ms", 500*time.Millisecond, transitionDuration)
	} else {
		t.Warning("Could not measure transition duration")
	}

	// Assertion 4: DEGRADED_MODE event emitted
	framework.AssertTrue(t.BaseTestCase, "DEGRADED_MODE event emitted", t.degradedModeSeen, "No DEGRADED_MODE event observed")

	// Assertion 5: WORKER_SCALE_DOWN event emitted
	framework.AssertTrue(t.BaseTestCase, "WORKER_SCALE_DOWN event emitted", t.scaleDownSeen, "No WORKER_SCALE_DOWN event observed")

	// Metrics
	result.AddMetric("final_state", gov.State().String())
	result.AddMetric("final_scale", gov.Scale())
	result.AddMetric("chunks_allocated", len(t.chunks))
	result.AddMetric("memory_allocated_mb", len(t.chunks)*10)

	result.Finish()
	return result
}

func (t *Test11NormalToDegraded) monitorEvents(sub *event.ErrorSubscription) {
	for evt := range sub.Events() {
		// Track DEGRADED_MODE event
		if evt.Code == event.CodeDegradedMode {
			if state, ok := evt.Context["state"].(string); ok && state == "DEGRADED" {
				t.degradedModeSeen = true
			}
		}

		// Track WORKER_SCALE_DOWN event
		if evt.Code == event.CodeWorkerScaleDown {
			t.scaleDownSeen = true
		}
	}
}
