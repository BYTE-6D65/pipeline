package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// ControlLoop orchestrates adaptive control based on direct state polling.
//
// Architecture: Error bus is ONE-WAY (observability only).
// The control loop polls memory state directly (no event subscription).
//
// The control loop manages:
//   - AIMD Governor: Scales based on memory pressure (governor handles cooldown internally)
//   - RED Dropper: Tracks buffer saturation (future integration)
//
// It emits control events when state changes occur (e.g., entering degraded mode).
type ControlLoop struct {
	// Components
	errorBus *event.ErrorBus // Write-only (emit observability events)
	governor *AIMDGovernor
	red      *REDDropper

	// Time source
	clock clock.Clock // Injected clock for testability

	// State for polling
	memoryLimit  uint64        // Memory limit for polling ReadMemoryStatsFast
	pollInterval time.Duration // How often to poll and update (e.g., 50ms)

	// State tracking
	lastState GovernorState
	lastScale float64
}

// NewControlLoop creates a new control loop.
//
// Parameters:
//   - clk: Clock for time tracking (use engine's clock for consistency)
//   - errorBus: Error bus for emitting observability events (write-only)
//   - governor: AIMD governor to update (governor handles cooldown internally)
//   - red: RED dropper for future integration
//   - memoryLimit: Memory limit for polling state
//   - pollInterval: How often to poll and update (e.g., 50ms)
func NewControlLoop(clk clock.Clock, errorBus *event.ErrorBus, governor *AIMDGovernor, red *REDDropper, memoryLimit uint64, pollInterval time.Duration) *ControlLoop {
	return &ControlLoop{
		clock:        clk,
		errorBus:     errorBus,
		governor:     governor,
		red:          red,
		memoryLimit:  memoryLimit,
		pollInterval: pollInterval,
		lastState:    governor.State(),
		lastScale:    governor.Scale(),
	}
}

// Start begins the control loop in a background goroutine.
//
// The loop:
//  1. Polls memory state directly (no event subscription)
//  2. Updates governor based on current pressure (governor handles cooldown internally)
//  3. Emits observability events when state changes (one-way out)
//
// Stops when context is cancelled.
func (cl *ControlLoop) Start(ctx context.Context) {
	// Emit startup event (observability)
	cl.errorBus.Publish(event.NewErrorEvent(
		event.InfoSeverity,
		event.CodeHealthCheck,
		"control-loop",
		"Control loop started",
	).WithContext("poll_interval", cl.pollInterval.String()))

	// Start periodic governor updates
	go cl.runGovernor(ctx)
}

// runGovernor periodically polls state and updates the AIMD governor.
func (cl *ControlLoop) runGovernor(ctx context.Context) {
	ticker := time.NewTicker(cl.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cl.updateGovernor()
		}
	}
}

// updateGovernor polls memory state, updates governor, and emits events.
func (cl *ControlLoop) updateGovernor() {
	// Poll memory state directly (no events)
	stats := ReadMemoryStatsFast(cl.memoryLimit)
	memPressure := stats.UsagePct

	// Save previous state/scale to detect changes
	prevScale := cl.governor.Scale()

	// Update governor (governor handles cooldown internally)
	cl.governor.Update(memPressure)

	// Check for changes
	currentState := cl.governor.State()
	currentScale := cl.governor.Scale()

	// Emit event on state transition (observability - one-way out)
	if currentState != cl.lastState {
		cl.emitStateChange(currentState, currentScale, memPressure)
		cl.lastState = currentState
	}

	// Emit event on significant scale change (>5%)
	scaleChange := currentScale - prevScale
	if scaleChange > 0.05 || scaleChange < -0.05 {
		cl.emitScaleChange(currentScale, scaleChange, memPressure)
		cl.lastScale = currentScale
	}
}

// emitStateChange emits an event when governor state changes.
func (cl *ControlLoop) emitStateChange(state GovernorState, scale, pressure float64) {
	var severity event.ErrorSeverity
	var signal event.ControlSignal
	var message string

	switch state {
	case StateNormal:
		severity = event.InfoSeverity
		signal = event.SignalRecovered
		message = "Governor recovered to normal operation"

	case StateDegraded:
		severity = event.WarningSeverity
		signal = event.SignalThrottle
		message = "Governor entered degraded mode - reducing scale"

	case StateRecovering:
		severity = event.InfoSeverity
		signal = event.SignalRecovered
		message = "Governor entering recovery - gradually increasing scale"
	}

	cl.errorBus.Publish(event.NewErrorEvent(
		severity,
		event.CodeDegradedMode,
		"control-loop:governor",
		message,
	).WithSignal(signal).
		WithContext("state", state.String()).
		WithContext("scale", fmt.Sprintf("%.2f", scale)).
		WithContext("pressure", fmt.Sprintf("%.1f%%", pressure*100)))
}

// emitScaleChange emits an event when scale changes significantly.
func (cl *ControlLoop) emitScaleChange(scale, change, pressure float64) {
	var code string
	var message string

	if change > 0 {
		code = event.CodeWorkerScaleUp
		message = fmt.Sprintf("Governor scale increased to %.0f%%", scale*100)
	} else {
		code = event.CodeWorkerScaleDown
		message = fmt.Sprintf("Governor scale decreased to %.0f%%", scale*100)
	}

	cl.errorBus.Publish(event.NewErrorEvent(
		event.InfoSeverity,
		code,
		"control-loop:governor",
		message,
	).WithContext("scale", fmt.Sprintf("%.2f", scale)).
		WithContext("change", fmt.Sprintf("%+.2f", change)).
		WithContext("pressure", fmt.Sprintf("%.1f%%", pressure*100)))
}

// Governor returns the AIMD governor.
func (cl *ControlLoop) Governor() *AIMDGovernor {
	return cl.governor
}

// RED returns the RED dropper.
func (cl *ControlLoop) RED() *REDDropper {
	return cl.red
}
