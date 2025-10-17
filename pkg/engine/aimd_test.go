package engine

import (
	"math"
	"testing"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
)

func TestAIMDGovernor_InitialState(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	if gov.State() != StateNormal {
		t.Errorf("Initial state = %s, want NORMAL", gov.State())
	}

	if gov.Scale() != 1.0 {
		t.Errorf("Initial scale = %.2f, want 1.0", gov.Scale())
	}
}

func TestAIMDGovernor_NormalToDegraded(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Update with pressure below threshold - should stay normal
	gov.Update(0.65)
	if gov.State() != StateNormal {
		t.Errorf("State at 65%% pressure = %s, want NORMAL", gov.State())
	}
	if gov.Scale() != 1.0 {
		t.Errorf("Scale at 65%% pressure = %.2f, want 1.0", gov.Scale())
	}

	// Update with pressure above threshold - should enter degraded
	gov.Update(0.75)
	if gov.State() != StateDegraded {
		t.Errorf("State at 75%% pressure = %s, want DEGRADED", gov.State())
	}

	// Scale should be reduced by factor (×0.5)
	expectedScale := 0.5
	if math.Abs(gov.Scale()-expectedScale) > 0.01 {
		t.Errorf("Scale at 75%% pressure = %.2f, want %.2f", gov.Scale(), expectedScale)
	}
}

func TestAIMDGovernor_DegradedToRecovering(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Force into degraded state
	gov.Update(0.75)
	if gov.State() != StateDegraded {
		t.Fatalf("Setup failed: not in DEGRADED state")
	}

	// Update with pressure below exit threshold
	gov.Update(0.50)
	if gov.State() != StateRecovering {
		t.Errorf("State at 50%% pressure = %s, want RECOVERING", gov.State())
	}

	// Scale should remain at 0.5 (transition doesn't change scale)
	if math.Abs(gov.Scale()-0.5) > 0.01 {
		t.Errorf("Scale at transition = %.2f, want 0.5", gov.Scale())
	}
}

func TestAIMDGovernor_RecoveringToNormal(t *testing.T) {
	fakeClk := newTestClock()
	gov := NewDefaultAIMDGovernor(fakeClk, 30*time.Second)

	// Force into recovering state at scale=0.9
	gov.state = StateRecovering
	gov.scale = 0.9
	gov.lastScaleChange = fakeClk.Now() // Set initial time

	// Advance time past cooldown
	fakeClk.Advance(31 * time.Second)

	// Update with pressure below exit threshold
	gov.Update(0.50)

	// Should increase by incrStep (0.05)
	expectedScale := 0.95
	if math.Abs(gov.Scale()-expectedScale) > 0.01 {
		t.Errorf("Scale after +0.05 = %.2f, want %.2f", gov.Scale(), expectedScale)
	}

	// Advance time again
	fakeClk.Advance(31 * time.Second)

	// Update again - should reach 1.0 and transition to Normal
	gov.Update(0.50)
	if gov.State() != StateNormal {
		t.Errorf("State after recovery = %s, want NORMAL", gov.State())
	}
	if gov.Scale() != 1.0 {
		t.Errorf("Scale after recovery = %.2f, want 1.0", gov.Scale())
	}
}

func TestAIMDGovernor_RecoveringBackToDegraded(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Force into recovering state
	gov.state = StateRecovering
	gov.scale = 0.7

	// Update with pressure above enter threshold
	gov.Update(0.75)

	if gov.State() != StateDegraded {
		t.Errorf("State when pressure returns = %s, want DEGRADED", gov.State())
	}

	// Scale should be decreased (0.7 × 0.5 = 0.35)
	expectedScale := 0.35
	if math.Abs(gov.Scale()-expectedScale) > 0.01 {
		t.Errorf("Scale when pressure returns = %.2f, want %.2f", gov.Scale(), expectedScale)
	}
}

func TestAIMDGovernor_CriticalPressure(t *testing.T) {
	fakeClk := newTestClock()
	gov := NewDefaultAIMDGovernor(fakeClk, 30*time.Second)

	// Force into degraded state at scale=0.5
	gov.state = StateDegraded
	gov.scale = 0.5
	gov.lastScaleChange = fakeClk.Now() // Set initial time

	// Advance time past cooldown
	fakeClk.Advance(31 * time.Second)

	// Update with critical pressure (>90%)
	gov.Update(0.95)

	// Should decrease again (0.5 × 0.5 = 0.25)
	expectedScale := 0.25
	if math.Abs(gov.Scale()-expectedScale) > 0.01 {
		t.Errorf("Scale at critical pressure = %.2f, want %.2f", gov.Scale(), expectedScale)
	}

	// State should remain degraded
	if gov.State() != StateDegraded {
		t.Errorf("State at critical pressure = %s, want DEGRADED", gov.State())
	}
}

func TestAIMDGovernor_MinimumScale(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Force scale below minimum
	gov.scale = 0.05

	// Update should clamp to minimum (0.1)
	gov.Update(0.50)

	if gov.Scale() < 0.1 {
		t.Errorf("Scale below minimum = %.2f, want >= 0.1", gov.Scale())
	}
}

func TestAIMDGovernor_MaximumScale(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Force scale above maximum
	gov.scale = 1.5

	// Update should clamp to maximum (1.0)
	gov.Update(0.50)

	if gov.Scale() > 1.0 {
		t.Errorf("Scale above maximum = %.2f, want <= 1.0", gov.Scale())
	}
}

func TestAIMDGovernor_HysteresisGap(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	// Enter degraded at 70%
	gov.Update(0.70)
	// Should be degraded (exactly at threshold triggers transition)
	if gov.State() != StateDegraded {
		t.Errorf("State at 70%% = %s, want DEGRADED", gov.State())
	}

	// Slight decrease to 68% - should stay degraded (not below exit threshold)
	gov.Update(0.68)
	if gov.State() != StateDegraded {
		t.Errorf("State at 68%% = %s, want DEGRADED (hysteresis)", gov.State())
	}

	// Drop to 54% - should enter recovering (below exit threshold)
	gov.Update(0.54)
	if gov.State() != StateRecovering {
		t.Errorf("State at 54%% = %s, want RECOVERING", gov.State())
	}
}

func TestAIMDGovernor_AdditiveIncrease(t *testing.T) {
	fakeClk := newTestClock()
	gov := NewDefaultAIMDGovernor(fakeClk, 30*time.Second)

	// Force into recovering state at scale=0.5
	gov.state = StateRecovering
	gov.scale = 0.5
	gov.lastScaleChange = fakeClk.Now() // Set initial time

	// Track scale increases
	scales := []float64{gov.Scale()}

	for i := 0; i < 5; i++ {
		// Advance time past cooldown before each update
		fakeClk.Advance(31 * time.Second)
		gov.Update(0.50) // Below exit threshold
		scales = append(scales, gov.Scale())
	}

	// Verify additive increases (+0.05 each time)
	for i := 1; i < len(scales); i++ {
		increase := scales[i] - scales[i-1]
		if math.Abs(increase-0.05) > 0.01 {
			t.Errorf("Increase step %d = %.3f, want 0.05 (additive)", i, increase)
		}
	}
}

func TestAIMDGovernor_MultiplicativeDecrease(t *testing.T) {
	fakeClk := newTestClock()
	gov := NewDefaultAIMDGovernor(fakeClk, 30*time.Second)

	// Start at normal (scale=1.0)
	initialScale := gov.Scale()

	// Trigger decrease
	gov.Update(0.75)
	firstDecrease := gov.Scale()

	// Should be halved (×0.5)
	expectedFirst := initialScale * 0.5
	if math.Abs(firstDecrease-expectedFirst) > 0.01 {
		t.Errorf("First decrease: %.2f, want %.2f (×0.5)", firstDecrease, expectedFirst)
	}

	// Advance time past cooldown
	fakeClk.Advance(31 * time.Second)

	// Trigger another decrease
	gov.Update(0.95)
	secondDecrease := gov.Scale()

	// Should be halved again (0.5 × 0.5 = 0.25)
	expectedSecond := firstDecrease * 0.5
	if math.Abs(secondDecrease-expectedSecond) > 0.01 {
		t.Errorf("Second decrease: %.2f, want %.2f (×0.5)", secondDecrease, expectedSecond)
	}
}

func TestAIMDGovernor_CustomThresholds(t *testing.T) {
	// Custom governor: enter at 80%, exit at 60%
	clk := clock.NewSystemClock()
	gov := NewAIMDGovernor(clk, 0.80, 0.60, 0.1, 0.5, 30*time.Second)

	// Should stay normal at 75%
	gov.Update(0.75)
	if gov.State() != StateNormal {
		t.Errorf("State at 75%% with custom thresholds = %s, want NORMAL", gov.State())
	}

	// Should enter degraded at 85%
	gov.Update(0.85)
	if gov.State() != StateDegraded {
		t.Errorf("State at 85%% with custom thresholds = %s, want DEGRADED", gov.State())
	}

	// Should enter recovering at 55%
	gov.Update(0.55)
	if gov.State() != StateRecovering {
		t.Errorf("State at 55%% with custom thresholds = %s, want RECOVERING", gov.State())
	}
}

func TestAIMDGovernor_StateString(t *testing.T) {
	tests := []struct {
		state    GovernorState
		expected string
	}{
		{StateNormal, "NORMAL"},
		{StateDegraded, "DEGRADED"},
		{StateRecovering, "RECOVERING"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("GovernorState(%d).String() = %s, want %s",
				tt.state, tt.state.String(), tt.expected)
		}
	}
}

func TestAIMDGovernor_Getters(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewAIMDGovernor(clk, 0.75, 0.60, 0.08, 0.6, 30*time.Second)

	if gov.EnterThreshold() != 0.75 {
		t.Errorf("EnterThreshold() = %.2f, want 0.75", gov.EnterThreshold())
	}

	if gov.ExitThreshold() != 0.60 {
		t.Errorf("ExitThreshold() = %.2f, want 0.60", gov.ExitThreshold())
	}

	if gov.IncrStep() != 0.08 {
		t.Errorf("IncrStep() = %.2f, want 0.08", gov.IncrStep())
	}

	if gov.DecrFactor() != 0.6 {
		t.Errorf("DecrFactor() = %.2f, want 0.6", gov.DecrFactor())
	}
}

func TestAIMDGovernor_DefaultValues(t *testing.T) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	if gov.EnterThreshold() != 0.70 {
		t.Errorf("Default EnterThreshold = %.2f, want 0.70", gov.EnterThreshold())
	}

	if gov.ExitThreshold() != 0.55 {
		t.Errorf("Default ExitThreshold = %.2f, want 0.55", gov.ExitThreshold())
	}

	if gov.IncrStep() != 0.05 {
		t.Errorf("Default IncrStep = %.2f, want 0.05", gov.IncrStep())
	}

	if gov.DecrFactor() != 0.5 {
		t.Errorf("Default DecrFactor = %.2f, want 0.5", gov.DecrFactor())
	}
}

// Benchmark governor update
func BenchmarkAIMDGovernor_Update(b *testing.B) {
	clk := clock.NewSystemClock()
	gov := NewDefaultAIMDGovernor(clk, 30*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gov.Update(0.75)
	}
}
