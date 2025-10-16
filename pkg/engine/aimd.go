package engine

import (
	"fmt"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
)

// GovernorState represents the current state of the AIMD governor.
type GovernorState int

const (
	// StateNormal indicates normal operation at full scale (1.0).
	StateNormal GovernorState = iota

	// StateDegraded indicates memory pressure detected, scale reduced.
	StateDegraded

	// StateRecovering indicates pressure relieved, gradually increasing scale.
	StateRecovering
)

func (s GovernorState) String() string {
	switch s {
	case StateNormal:
		return "NORMAL"
	case StateDegraded:
		return "DEGRADED"
	case StateRecovering:
		return "RECOVERING"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// AIMDGovernor implements Additive Increase, Multiplicative Decrease rate control.
//
// This is a classic TCP congestion control algorithm adapted for memory pressure:
//   - Multiplicative Decrease: On pressure (>70%), scale ×0.5 (fast response)
//   - Additive Increase: On relief (<55%), scale +0.05/tick (slow recovery)
//
// State machine:
//
//	Normal (scale=1.0)
//	  ├─ pressure > enterThreshold → Degraded (scale ×decrFactor)
//
//	Degraded (scale=0.2-1.0)
//	  ├─ pressure < exitThreshold → Recovering
//	  └─ pressure > criticalThreshold → More decrease (×decrFactor)
//
//	Recovering (scale increasing)
//	  ├─ pressure < exitThreshold → +incrStep/tick until scale=1.0 → Normal
//	  └─ pressure > enterThreshold → Degraded
//
// Hysteresis: 15% gap between enter (70%) and exit (55%) prevents oscillation.
type AIMDGovernor struct {
	// Configuration
	enterThreshold    float64 // Enter degraded mode (e.g., 0.70)
	exitThreshold     float64 // Exit degraded mode (e.g., 0.55)
	criticalThreshold float64 // Critical pressure level (e.g., 0.90)
	incrStep          float64 // Additive increase per tick (e.g., 0.05)
	decrFactor        float64 // Multiplicative decrease factor (e.g., 0.5)
	minScale          float64 // Minimum scale factor (e.g., 0.2 = 20%)
	maxScale          float64 // Maximum scale factor (always 1.0)
	cooldown          time.Duration // Min time between scale changes (e.g., 30s)

	// Time source
	clock clock.Clock // Injected clock for testability

	// State
	scale          float64       // Current scale factor (0.0-1.0)
	state          GovernorState // Current state
	lastScaleChange clock.MonoTime // Last time scale changed (for rate limiting)
}

// NewAIMDGovernor creates an AIMD governor with the given thresholds.
//
// Parameters:
//   - clk: Clock for time-aware rate limiting (use engine's clock for consistency)
//   - enterThreshold: Memory pressure to enter degraded mode (0.0-1.0)
//   - exitThreshold: Memory pressure to exit degraded mode (0.0-1.0)
//   - incrStep: Additive increase per tick (e.g., 0.05 = 5%/tick)
//   - decrFactor: Multiplicative decrease (e.g., 0.5 = half speed)
//   - cooldown: Min time between scale changes (e.g., 30s)
//
// Example:
//
//	governor := NewAIMDGovernor(engine.Clock(), 0.70, 0.55, 0.05, 0.5, 30*time.Second)
//	// Enter degraded at 70%, exit at 55%
//	// Recover by +5% per 30s cooldown, decrease by ×0.5
func NewAIMDGovernor(clk clock.Clock, enterThreshold, exitThreshold, incrStep, decrFactor float64, cooldown time.Duration) *AIMDGovernor {
	return &AIMDGovernor{
		enterThreshold:    enterThreshold,
		exitThreshold:     exitThreshold,
		criticalThreshold: 0.90, // Fixed critical threshold
		incrStep:          incrStep,
		decrFactor:        decrFactor,
		minScale:          0.2, // Never go below 20% (prevents starvation)
		maxScale:          1.0, // Never exceed 100%
		cooldown:          cooldown,
		clock:             clk,
		scale:             1.0, // Start at full speed
		state:             StateNormal,
		lastScaleChange:   clk.Now(), // Initialize to now
	}
}

// NewDefaultAIMDGovernor creates an AIMD governor with sensible defaults.
//
// Parameters:
//   - clk: Clock for time-aware rate limiting (use engine's clock for consistency)
//   - cooldown: Min time between scale changes (e.g., 30s)
//
// Defaults:
//   - enterThreshold: 0.70 (enter degraded at 70% memory)
//   - exitThreshold: 0.55 (exit degraded at 55% memory)
//   - incrStep: 0.05 (increase by 5% per cooldown period)
//   - decrFactor: 0.5 (decrease to half speed)
//
// Hysteresis: 15% gap (70% - 55%) prevents oscillation.
func NewDefaultAIMDGovernor(clk clock.Clock, cooldown time.Duration) *AIMDGovernor {
	return NewAIMDGovernor(clk, 0.70, 0.55, 0.05, 0.5, cooldown)
}

// Update processes a memory pressure reading and updates governor state.
//
// This can be called frequently (e.g., every 50ms) with current memory pressure.
// The governor internally rate-limits scale changes based on cooldown period.
//
// State transitions (NORMAL ↔ DEGRADED ↔ RECOVERING) happen immediately.
// Scale changes are rate-limited to once per cooldown period (e.g., 30s).
//
// Usage:
//
//	memPressure := stats.UsagePct  // 0.0-1.0
//	governor.Update(memPressure)
//	scale := governor.Scale()  // Apply this to publish rate
func (g *AIMDGovernor) Update(memPressure float64) {
	switch g.state {
	case StateNormal:
		g.updateNormal(memPressure)
	case StateDegraded:
		g.updateDegraded(memPressure)
	case StateRecovering:
		g.updateRecovering(memPressure)
	}

	// Clamp scale to [minScale, maxScale]
	if g.scale < g.minScale {
		g.scale = g.minScale
	}
	if g.scale > g.maxScale {
		g.scale = g.maxScale
	}
}

// updateNormal handles state transitions from Normal state.
func (g *AIMDGovernor) updateNormal(memPressure float64) {
	if memPressure >= g.enterThreshold {
		// Pressure detected - enter degraded mode
		g.state = StateDegraded
		// Multiplicative decrease (always allowed from NORMAL state)
		g.scale *= g.decrFactor
		g.lastScaleChange = g.clock.Now()
	}
	// else: stay in normal, scale remains 1.0
}

// updateDegraded handles state transitions from Degraded state.
func (g *AIMDGovernor) updateDegraded(memPressure float64) {
	if memPressure < g.exitThreshold {
		// Pressure relieved - enter recovery (state transition only, no scale change)
		g.state = StateRecovering
	} else if memPressure > g.criticalThreshold {
		// Still high pressure - check if we can decrease more
		if g.clock.Since(g.lastScaleChange) >= g.cooldown {
			g.scale *= g.decrFactor
			g.lastScaleChange = g.clock.Now()
		}
	}
	// else: stay in degraded at current scale
}

// updateRecovering handles state transitions from Recovering state.
func (g *AIMDGovernor) updateRecovering(memPressure float64) {
	if memPressure < g.exitThreshold {
		// Still below exit threshold - additive increase (rate-limited by cooldown)
		if g.clock.Since(g.lastScaleChange) >= g.cooldown {
			g.scale += g.incrStep
			g.lastScaleChange = g.clock.Now()

			// Check if fully recovered
			if g.scale >= g.maxScale {
				g.scale = g.maxScale
				g.state = StateNormal
			}
		}
	} else if memPressure >= g.enterThreshold {
		// Pressure returned - back to degraded (always allowed)
		g.state = StateDegraded
		g.scale *= g.decrFactor
		g.lastScaleChange = g.clock.Now()
	}
	// else: stay in recovering at current scale (pressure between exit and enter)
}

// Scale returns the current scale factor (0.0-1.0).
//
// This can be used to throttle operations:
//
//	scale := governor.Scale()
//	if scale < 1.0 {
//	    delay := baseDelay * (1.0 / scale)
//	    time.Sleep(delay)
//	}
func (g *AIMDGovernor) Scale() float64 {
	return g.scale
}

// State returns the current governor state.
func (g *AIMDGovernor) State() GovernorState {
	return g.state
}

// EnterThreshold returns the configured enter threshold.
func (g *AIMDGovernor) EnterThreshold() float64 {
	return g.enterThreshold
}

// ExitThreshold returns the configured exit threshold.
func (g *AIMDGovernor) ExitThreshold() float64 {
	return g.exitThreshold
}

// IncrStep returns the configured additive increase step.
func (g *AIMDGovernor) IncrStep() float64 {
	return g.incrStep
}

// DecrFactor returns the configured multiplicative decrease factor.
func (g *AIMDGovernor) DecrFactor() float64 {
	return g.decrFactor
}
