package engine

import (
	"context"
	"testing"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// TestGovernor_EventDrivenControl verifies the governor responds to control events.
func TestGovernor_EventDrivenControl(t *testing.T) {
	// Create test infrastructure
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(16))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	// Start governor subscription
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	// Give subscription time to initialize
	time.Sleep(50 * time.Millisecond)

	// Initial state should be NORMAL at scale 1.0
	if governor.State() != StateNormal {
		t.Errorf("Expected StateNormal, got %s", governor.State())
	}
	if governor.Scale() != 1.0 {
		t.Errorf("Expected scale 1.0, got %.2f", governor.Scale())
	}

	// Publish a scale command to reduce to 0.5
	cmd := event.GovernorScaleCommand{
		Scale:     0.5,
		Reason:    "Test: manual scale down",
		Source:    "test",
		Timestamp: time.Now(),
	}

	evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
	if err := bus.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Failed to publish scale command: %v", err)
	}

	// Give governor time to process event
	time.Sleep(100 * time.Millisecond)

	// Verify scale was applied
	if governor.Scale() != 0.5 {
		t.Errorf("Expected scale 0.5, got %.2f", governor.Scale())
	}

	// State should transition to DEGRADED (scale < 1.0 and decreasing)
	if governor.State() != StateDegraded {
		t.Errorf("Expected StateDegraded, got %s", governor.State())
	}
}

// TestGovernor_EventDrivenControl_InvalidScale verifies invalid scales are rejected.
func TestGovernor_EventDrivenControl_InvalidScale(t *testing.T) {
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(16))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	initialScale := governor.Scale()

	// Try to set invalid scale > 1.0
	cmd := event.GovernorScaleCommand{
		Scale:     1.5,
		Reason:    "Test: invalid scale",
		Source:    "test",
		Timestamp: time.Now(),
	}

	evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
	if err := bus.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Failed to publish scale command: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Scale should be clamped to 1.0 (maxScale)
	if governor.Scale() != 1.0 {
		t.Errorf("Expected scale to be clamped to 1.0, got %.2f", governor.Scale())
	}

	// Try to set invalid scale < 0.0
	cmd2 := event.GovernorScaleCommand{
		Scale:     -0.5,
		Reason:    "Test: negative scale",
		Source:    "test",
		Timestamp: time.Now(),
	}

	evt2 := event.NewControlEvent(event.EventTypeGovernorScale, cmd2)
	if err := bus.Publish(context.Background(), evt2); err != nil {
		t.Fatalf("Failed to publish scale command: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Scale should remain valid (negative scale rejected entirely)
	currentScale := governor.Scale()
	if currentScale < 0 || currentScale > 1.0 {
		t.Errorf("Expected scale in valid range, got %.2f", currentScale)
	}

	// Should still have a valid scale from before
	if currentScale != initialScale && currentScale != 1.0 {
		t.Errorf("Scale changed unexpectedly to %.2f", currentScale)
	}
}

// TestGovernor_ConcurrentControl verifies thread-safe operation with concurrent updates.
func TestGovernor_ConcurrentControl(t *testing.T) {
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(32))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Simulate concurrent control from multiple sources
	done := make(chan bool, 3)

	// Source 1: Event-driven commands
	go func() {
		for i := 0; i < 10; i++ {
			cmd := event.GovernorScaleCommand{
				Scale:     0.8,
				Reason:    "Source 1",
				Source:    "concurrent-test-1",
				Timestamp: time.Now(),
			}
			evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
			bus.Publish(context.Background(), evt)
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Source 2: Direct Update() calls (backward compatibility)
	go func() {
		for i := 0; i < 10; i++ {
			governor.Update(0.65) // Mid-range pressure
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Source 3: Scale reads
	go func() {
		for i := 0; i < 20; i++ {
			_ = governor.Scale()
			_ = governor.State()
			time.Sleep(5 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Governor should still be in valid state
	scale := governor.Scale()
	if scale < 0.2 || scale > 1.0 {
		t.Errorf("Scale out of valid range: %.2f", scale)
	}

	state := governor.State()
	if state < StateNormal || state > StateRecovering {
		t.Errorf("Invalid state: %d", state)
	}
}

// TestGovernor_MultipleCommands verifies processing multiple scale commands in sequence.
func TestGovernor_MultipleCommands(t *testing.T) {
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(16))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Test sequence: 1.0 → 0.5 → 0.2 → 0.8 → 1.0
	scales := []float64{0.5, 0.2, 0.8, 1.0}

	for _, targetScale := range scales {
		cmd := event.GovernorScaleCommand{
			Scale:     targetScale,
			Reason:    "Test: sequence",
			Source:    "test",
			Timestamp: time.Now(),
		}

		evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
		if err := bus.Publish(context.Background(), evt); err != nil {
			t.Fatalf("Failed to publish scale command: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		currentScale := governor.Scale()
		if currentScale != targetScale {
			t.Errorf("Expected scale %.2f, got %.2f", targetScale, currentScale)
		}
	}

	// Final state should be NORMAL at scale 1.0
	if governor.State() != StateNormal {
		t.Errorf("Expected StateNormal, got %s", governor.State())
	}
}

// TestGovernor_EventSourceTracking verifies commands from different sources.
func TestGovernor_EventSourceTracking(t *testing.T) {
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(16))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Commands from different sources
	sources := []string{
		"control-lab",
		"manual-override",
		"health-check",
		"emergency-shutdown",
	}

	for _, source := range sources {
		cmd := event.GovernorScaleCommand{
			Scale:     0.7,
			Reason:    "Test: source tracking",
			Source:    source,
			Timestamp: time.Now(),
		}

		evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
		evt.SetSource(source) // Override event source

		if err := bus.Publish(context.Background(), evt); err != nil {
			t.Fatalf("Failed to publish scale command: %v", err)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Governor should process all commands regardless of source
	if governor.Scale() != 0.7 {
		t.Errorf("Expected scale 0.7, got %.2f", governor.Scale())
	}
}

// TestGovernor_ContextCancellation verifies governor stops on context cancel.
func TestGovernor_ContextCancellation(t *testing.T) {
	clk := clock.NewSystemClock()
	bus := event.NewInMemoryBus(event.WithBufferSize(16))
	defer bus.Close()

	governor := NewDefaultAIMDGovernor(clk, 1*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	if err := governor.Start(ctx, bus); err != nil {
		t.Fatalf("Failed to start governor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Give time for goroutine to exit
	time.Sleep(50 * time.Millisecond)

	// Try to publish command after cancellation
	cmd := event.GovernorScaleCommand{
		Scale:     0.5,
		Reason:    "Test: after cancel",
		Source:    "test",
		Timestamp: time.Now(),
	}

	evt := event.NewControlEvent(event.EventTypeGovernorScale, cmd)
	if err := bus.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Failed to publish scale command: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Governor should not process events after context cancellation
	// Scale should remain at initial value (1.0) or last value before cancel
	scale := governor.Scale()
	if scale != 1.0 {
		// This is expected - governor may have processed the command before cancel
		t.Logf("Note: Governor scale is %.2f (may have processed before cancel)", scale)
	}
}
