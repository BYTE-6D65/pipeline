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

// Test21PanicSawPrevention validates cooldown prevents multiple rapid decreases.
//
// Test: 2.1 - No Panic-Saw Under Sustained Pressure
// Category: Cooldown Enforcement Tests
//
// Goal: Verify only ONE scale decrease per cooldown window.
//
// Pass Criteria:
//   - First decrease: 100% → 50% at 70% pressure
//   - Second decrease: 50% → 25% at 90% pressure (after 30s cooldown)
//   - NO decreases between 0-30 seconds after first decrease
//   - Maximum 2 decreases in first 60 seconds
type Test21PanicSawPrevention struct {
	*framework.BaseTestCase

	chunks         [][]byte
	scaleDecreases []scaleDecrease
}

type scaleDecrease struct {
	Time      time.Time
	OldScale  float64
	NewScale  float64
	Pressure  float64
	TimeSince time.Duration // Time since last decrease
}

// NewTest21PanicSawPrevention creates a new test instance.
func NewTest21PanicSawPrevention() framework.TestCase {
	return &Test21PanicSawPrevention{
		BaseTestCase:   framework.NewBaseTestCase(),
		scaleDecreases: make([]scaleDecrease, 0),
	}
}

func (t *Test21PanicSawPrevention) Name() string {
	return "2.1: No Panic-Saw Under Sustained Pressure"
}

func (t *Test21PanicSawPrevention) Category() string {
	return "Cooldown Enforcement Tests"
}

func (t *Test21PanicSawPrevention) Description() string {
	return "Verify cooldown prevents multiple rapid scale decreases (panic-saw bug)"
}

func (t *Test21PanicSawPrevention) Setup(ctx context.Context) error {
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

func (t *Test21PanicSawPrevention) Run(ctx context.Context) error {
	limit := t.MemoryLimit()
	chunkSize := 10 * 1024 * 1024 // 10MB

	// Monitor scale changes
	lastScale := 1.0
	lastDecreaseTime := time.Time{}
	testStart := time.Now()

	// Phase 1: Allocate to 70% to trigger first decrease
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		currentScale := t.Engine().Governor().Scale()
		stats := engine.ReadMemoryStatsFast(limit)

		// Detect scale decrease
		if currentScale < lastScale-0.01 { // Account for float precision
			decrease := scaleDecrease{
				Time:     time.Now(),
				OldScale: lastScale,
				NewScale: currentScale,
				Pressure: stats.UsagePct,
			}

			if !lastDecreaseTime.IsZero() {
				decrease.TimeSince = time.Since(lastDecreaseTime)
			}

			t.scaleDecreases = append(t.scaleDecreases, decrease)
			lastScale = currentScale
			lastDecreaseTime = time.Now()
		}

		// Stop after 60 seconds or if we have 2+ decreases
		if time.Since(testStart) > 60*time.Second || len(t.scaleDecreases) >= 2 {
			break
		}

		// Continue allocating to maintain/increase pressure
		if stats.UsagePct < 0.92 {
			chunk := make([]byte, chunkSize)
			for i := range chunk {
				chunk[i] = byte(i % 256)
			}
			t.chunks = append(t.chunks, chunk)
		}

		time.Sleep(100 * time.Millisecond)
	}

	// Give events time to propagate
	time.Sleep(200 * time.Millisecond)

	return nil
}

func (t *Test21PanicSawPrevention) Teardown() error {
	// Release memory
	t.chunks = nil
	runtime.GC()

	// Shutdown engine
	return t.TeardownEngine()
}

func (t *Test21PanicSawPrevention) Validate() *framework.TestResult {
	result := t.Result()
	result.TestName = t.Name()
	result.Category = t.Category()

	// Assertion 1: First decrease happened (100% → 50%)
	if len(t.scaleDecreases) >= 1 {
		firstDecrease := t.scaleDecreases[0]
		framework.AssertScaleNear(t.BaseTestCase, "First decrease to 0.50", 0.50, firstDecrease.NewScale, 0.01)
		framework.AssertTrue(t.BaseTestCase, "First decrease at ~70% pressure", firstDecrease.Pressure >= 0.68 && firstDecrease.Pressure <= 0.75, "Pressure outside expected range")
	} else {
		t.Error(nil)
		t.Warning("No scale decreases observed!")
	}

	// Assertion 2: If second decrease happened, it was after cooldown
	if len(t.scaleDecreases) >= 2 {
		secondDecrease := t.scaleDecreases[1]
		framework.AssertDurationInRange(t.BaseTestCase, "Second decrease after 28-32s cooldown", 28*time.Second, 32*time.Second, secondDecrease.TimeSince)
		framework.AssertScaleNear(t.BaseTestCase, "Second decrease to 0.25", 0.25, secondDecrease.NewScale, 0.01)
	}

	// Assertion 3: No more than 2 decreases in 60 seconds (proves cooldown works)
	framework.AssertCountInRange(t.BaseTestCase, "Max 2 decreases in 60s", 1, 2, len(t.scaleDecreases))

	// Assertion 4: NO decreases within first 30 seconds after initial decrease
	if len(t.scaleDecreases) > 1 {
		noPanicSaw := true
		for i := 1; i < len(t.scaleDecreases); i++ {
			if t.scaleDecreases[i].TimeSince < 28*time.Second {
				noPanicSaw = false
				t.Warning(fmt.Sprintf("Decrease %d happened after only %s (panic-saw!)", i+1, t.scaleDecreases[i].TimeSince))
			}
		}
		framework.AssertTrue(t.BaseTestCase, "No panic-saw (all decreases ≥28s apart)", noPanicSaw, "Decreases happened too rapidly")
	}

	// Assertion 5: Scale never dropped below 20% (MinScale floor)
	minScale := 1.0
	for _, dec := range t.scaleDecreases {
		if dec.NewScale < minScale {
			minScale = dec.NewScale
		}
	}
	framework.AssertScaleGreaterThan(t.BaseTestCase, "Scale ≥ 0.20 (MinScale floor)", 0.19, minScale)

	// Metrics
	result.AddMetric("decrease_count", len(t.scaleDecreases))
	result.AddMetric("final_scale", t.Engine().Governor().Scale())

	for i, dec := range t.scaleDecreases {
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_time", dec.Time)
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_old_scale", dec.OldScale)
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_new_scale", dec.NewScale)
		result.AddMetric("decrease_"+string(rune('0'+i+1))+"_pressure", dec.Pressure)
		if i > 0 {
			result.AddMetric("decrease_"+string(rune('0'+i+1))+"_interval", dec.TimeSince)
		}
	}

	result.Finish()
	return result
}

func (t *Test21PanicSawPrevention) monitorEvents(sub *event.ErrorSubscription) {
	for range sub.Events() {
		// Just consume events (we track scale directly)
	}
}
