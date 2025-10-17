package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// ControlLab orchestrates adaptive control based on analysis of system state.
//
// Architecture: Control Lab receives signals from ErrorBus (observability input) and
// sends control commands to InternalBus (control output). It's a laboratory that
// analyzes system state and produces control decisions, not a loop.
//
// Event Flow:
//   - Input: Reads memory stats directly (no event subscription)
//   - Analysis: Calculates desired governor scale based on AIMD algorithm
//   - Output: Publishes GovernorScaleCommand to InternalBus
//   - Observability: Publishes state changes to ErrorBus
//
// The control lab manages:
//   - AIMD Governor: Scales based on memory pressure (governor handles cooldown internally)
//   - RED Dropper: Tracks buffer saturation (future integration)
//
// It emits control events when state changes occur (e.g., entering degraded mode).
type ControlLab struct {
	// Components
	errorBus    *event.ErrorBus       // Write-only (emit observability events)
	internalBus *event.InMemoryBus    // Write-only (emit control commands)
	governor    *AIMDGovernor
	red         *REDDropper

	// Time source
	clock clock.Clock // Injected clock for testability

	// State for polling
	memoryLimit  uint64        // Memory limit for polling ReadMemoryStatsFast
	pollInterval time.Duration // How often to poll and update (e.g., 50ms)

	// State tracking
	lastState GovernorState
	lastScale float64
}

// NewControlLab creates a new control lab.
//
// Parameters:
//   - clk: Clock for time tracking (use engine's clock for consistency)
//   - errorBus: Error bus for emitting observability events (write-only)
//   - internalBus: Internal bus for emitting control commands (write-only)
//   - governor: AIMD governor to read state (governor subscribes to internalBus separately)
//   - red: RED dropper for future integration
//   - memoryLimit: Memory limit for polling state
//   - pollInterval: How often to poll and update (e.g., 50ms)
func NewControlLab(clk clock.Clock, errorBus *event.ErrorBus, internalBus *event.InMemoryBus, governor *AIMDGovernor, red *REDDropper, memoryLimit uint64, pollInterval time.Duration) *ControlLab {
	return &ControlLab{
		clock:        clk,
		errorBus:     errorBus,
		internalBus:  internalBus,
		governor:     governor,
		red:          red,
		memoryLimit:  memoryLimit,
		pollInterval: pollInterval,
		lastState:    governor.State(),
		lastScale:    governor.Scale(),
	}
}

// Start begins the control lab's analysis in a background goroutine.
//
// The lab:
//  1. Polls memory state directly (no event subscription)
//  2. Updates governor based on current pressure (governor handles cooldown internally)
//  3. Emits observability events when state changes (one-way out)
//
// Stops when context is cancelled.
func (cl *ControlLab) Start(ctx context.Context) {
	// Emit startup event (observability)
	cl.errorBus.Publish(event.NewErrorEvent(
		event.InfoSeverity,
		event.CodeHealthCheck,
		"control-lab",
		"Control lab started",
	).WithContext("poll_interval", cl.pollInterval.String()))

	// Start periodic governor updates
	go cl.runGovernor(ctx)
}

// runGovernor periodically polls state and updates the AIMD governor.
func (cl *ControlLab) runGovernor(ctx context.Context) {
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

// updateGovernor polls memory state, calculates desired scale, and publishes control commands.
//
// Hybrid approach (Phase 3 transition):
//  1. Read memory pressure
//  2. Calculate desired scale using AIMD logic
//  3. Publish GovernorScaleCommand to InternalBus (event-driven)
//  4. Also call governor.Update() directly (for backward compatibility during transition)
//  5. Emit observability events when state changes
//
// TODO: Remove direct governor.Update() call once fully event-driven (Phase 4+)
func (cl *ControlLab) updateGovernor() {
	// Poll memory state directly (no events)
	stats := ReadMemoryStatsFast(cl.memoryLimit)
	memPressure := stats.UsagePct

	// Save previous state/scale to detect changes
	prevScale := cl.governor.Scale()

	// Update governor directly (backward compatibility - will be removed in Phase 4+)
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

		// Publish scale command to InternalBus (event-driven control)
		// This enables observability and allows other components to react
		cmd := event.GovernorScaleCommand{
			Scale:     currentScale,
			Reason:    fmt.Sprintf("Memory pressure %.1f%% (state: %s)", memPressure*100, currentState),
			Source:    "control-lab",
			Timestamp: time.Now(),
		}

		evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
		if err := cl.internalBus.Publish(context.Background(), evt); err != nil {
			// Log error via error bus but don't crash
			cl.errorBus.Publish(event.NewErrorEvent(
				event.WarningSeverity,
				event.CodeHealthCheck,
				"control-lab",
				fmt.Sprintf("Failed to publish governor scale command: %v", err),
			))
		}
	}
}

// emitStateChange emits an event when governor state changes.
func (cl *ControlLab) emitStateChange(state GovernorState, scale, pressure float64) {
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
		"control-lab:governor",
		message,
	).WithSignal(signal).
		WithContext("state", state.String()).
		WithContext("scale", fmt.Sprintf("%.2f", scale)).
		WithContext("pressure", fmt.Sprintf("%.1f%%", pressure*100)))
}

// emitScaleChange emits an event when scale changes significantly.
func (cl *ControlLab) emitScaleChange(scale, change, pressure float64) {
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
		"control-lab:governor",
		message,
	).WithContext("scale", fmt.Sprintf("%.2f", scale)).
		WithContext("change", fmt.Sprintf("%+.2f", change)).
		WithContext("pressure", fmt.Sprintf("%.1f%%", pressure*100)))
}

// Governor returns the AIMD governor.
func (cl *ControlLab) Governor() *AIMDGovernor {
	return cl.governor
}

// RED returns the RED dropper.
func (cl *ControlLab) RED() *REDDropper {
	return cl.red
}
