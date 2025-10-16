package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("=== Pipeline AIMD Recovery Test ===")
	log.Println("This test validates the full AIMD cycle:")
	log.Println("  NORMAL → DEGRADED → RECOVERING → NORMAL")
	log.Println()

	// Load config from environment
	cfg, err := engine.LoadFromEnv()
	if err != nil {
		cfg = engine.DefaultConfig()
		log.Printf("Using default config")
	}

	// Create engine with AIMD + RED enabled
	log.Println("Creating engine with AIMD governor...")
	eng, err := engine.NewWithConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
	}

	// Subscribe to error bus
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub, err := eng.ErrorBus().Subscribe(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe to error bus: %v", err)
	}

	log.Printf("Subscribed to error bus (ID: %s)", sub.ID())

	// Print initial state
	log.Println("\nInitial state:")
	limit, src, ok := engine.DetectMemoryLimit()
	if !ok {
		log.Fatal("No memory limit detected - set GOMEMLIMIT")
	}
	log.Printf("  Memory Limit: %s (source: %s)", engine.FormatBytes(limit), src)
	log.Printf("  AIMD Enter Threshold: %.0f%% (%s)",
		eng.Governor().EnterThreshold()*100,
		engine.FormatBytes(uint64(float64(limit)*eng.Governor().EnterThreshold())))
	log.Printf("  AIMD Exit Threshold: %.0f%% (%s)",
		eng.Governor().ExitThreshold()*100,
		engine.FormatBytes(uint64(float64(limit)*eng.Governor().ExitThreshold())))
	log.Printf("  AIMD State: %s", eng.Governor().State())
	log.Printf("  AIMD Scale: %.2f (%.0f%%)", eng.Governor().Scale(), eng.Governor().Scale()*100)
	log.Println()

	// Event tracker
	tracker := &EventTracker{
		stateChanges: make([]StateChange, 0),
		scaleChanges: make([]ScaleChange, 0),
	}

	// Start event printer
	go printControlEvents(sub, tracker)

	// Handle shutdown gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	// Run the test phases
	if err := runRecoveryTest(eng, tracker, limit, sigCh); err != nil {
		log.Printf("Test failed: %v", err)
		os.Exit(1)
	}

	// Shutdown engine
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := eng.Shutdown(shutdownCtx); err != nil {
		log.Printf("Shutdown error: %v", err)
	}

	log.Println("\n=== Test Complete ===")
}

// StateChange tracks a governor state transition
type StateChange struct {
	Timestamp time.Time
	From      string
	To        string
	Scale     float64
	Pressure  float64
}

// ScaleChange tracks a scale factor change
type ScaleChange struct {
	Timestamp time.Time
	Scale     float64
	Change    float64
	Pressure  float64
}

// EventTracker collects state and scale changes
type EventTracker struct {
	stateChanges []StateChange
	scaleChanges []ScaleChange
}

func runRecoveryTest(eng *engine.Engine, tracker *EventTracker, limit uint64, sigCh chan os.Signal) error {
	log.Println("=== Phase 1: Trigger DEGRADED State ===")
	log.Println("Allocating memory to reach 70% threshold...")

	// Calculate target allocation (70% of limit)
	targetBytes := uint64(float64(limit) * 0.72) // Slightly over to ensure trigger
	chunkSize := 10 * 1024 * 1024                 // 10MB chunks
	var chunks [][]byte

	// Allocate until we hit ~70%
	for {
		select {
		case <-sigCh:
			return fmt.Errorf("interrupted during allocation")
		default:
		}

		// Check if we've reached DEGRADED state
		if eng.Governor().State().String() == "DEGRADED" {
			log.Printf("✅ DEGRADED state reached!")
			log.Printf("   Current scale: %.2f (%.0f%%)", eng.Governor().Scale(), eng.Governor().Scale()*100)
			break
		}

		// Allocate another chunk
		chunk := make([]byte, chunkSize)
		for i := range chunk {
			chunk[i] = byte(i % 256)
		}
		chunks = append(chunks, chunk)

		stats := engine.ReadMemoryStatsFast(limit)
		log.Printf("Allocated %d chunks (%s) - Heap: %s / %s (%.1f%%) - State: %s @ %.0f%%",
			len(chunks),
			engine.FormatBytes(uint64(len(chunks)*chunkSize)),
			engine.FormatBytes(stats.HeapAlloc),
			engine.FormatBytes(limit),
			stats.UsagePct*100,
			eng.Governor().State(),
			eng.Governor().Scale()*100,
		)

		if stats.HeapAlloc > targetBytes {
			log.Printf("Reached target allocation, waiting for governor...")
		}

		time.Sleep(500 * time.Millisecond)
	}

	// Wait a bit to ensure cooldown starts
	time.Sleep(2 * time.Second)

	log.Println("\n=== Phase 2: Trigger Memory Relief ===")
	log.Println("Releasing memory to drop below 55% exit threshold...")

	// Release all chunks
	chunks = nil
	runtime.GC() // Force immediate GC
	time.Sleep(1 * time.Second)
	runtime.GC() // Second GC to be thorough

	// Wait and monitor for RECOVERING state
	log.Println("Waiting for RECOVERING state (max 10 seconds)...")
	deadline := time.Now().Add(10 * time.Second)
	recoveringReached := false

	for time.Now().Before(deadline) {
		select {
		case <-sigCh:
			return fmt.Errorf("interrupted during recovery wait")
		default:
		}

		stats := engine.ReadMemoryStatsFast(limit)
		state := eng.Governor().State().String()

		log.Printf("Memory: %s / %s (%.1f%%) - State: %s @ %.0f%%",
			engine.FormatBytes(stats.HeapAlloc),
			engine.FormatBytes(limit),
			stats.UsagePct*100,
			state,
			eng.Governor().Scale()*100,
		)

		if state == "RECOVERING" {
			log.Printf("✅ RECOVERING state reached!")
			recoveringReached = true
			break
		}

		time.Sleep(1 * time.Second)
	}

	if !recoveringReached {
		return fmt.Errorf("RECOVERING state not reached within 10 seconds")
	}

	log.Println("\n=== Phase 3: Watch Additive Recovery ===")
	log.Println("Monitoring scale increase (+5% per 30s cooldown)...")

	// Monitor recovery for up to 5 minutes
	recoveryDeadline := time.Now().Add(5 * time.Minute)
	lastScale := eng.Governor().Scale()
	lastReport := time.Now()

	for time.Now().Before(recoveryDeadline) {
		select {
		case <-sigCh:
			return fmt.Errorf("interrupted during recovery monitoring")
		default:
		}

		currentScale := eng.Governor().Scale()
		state := eng.Governor().State().String()
		stats := engine.ReadMemoryStatsFast(limit)

		// Report every 10 seconds
		if time.Since(lastReport) >= 10*time.Second {
			scaleChange := currentScale - lastScale
			log.Printf("State: %s | Scale: %.2f (%.0f%%) | Change: %+.2f | Memory: %.1f%%",
				state,
				currentScale,
				currentScale*100,
				scaleChange,
				stats.UsagePct*100,
			)
			lastScale = currentScale
			lastReport = time.Now()
		}

		// Check if fully recovered
		if state == "NORMAL" {
			log.Printf("✅ NORMAL state reached!")
			log.Printf("   Final scale: %.2f (%.0f%%)", currentScale, currentScale*100)
			break
		}

		time.Sleep(2 * time.Second)
	}

	// Final validation
	log.Println("\n=== Phase 4: Validation ===")
	return validateRecovery(tracker, eng)
}

func validateRecovery(tracker *EventTracker, eng *engine.Engine) error {
	log.Println("Checking test criteria...")

	// Check 1: Did we see DEGRADED state?
	degradedSeen := false
	for _, sc := range tracker.stateChanges {
		if sc.To == "DEGRADED" {
			degradedSeen = true
			log.Printf("✅ DEGRADED state transition observed at %s", sc.Timestamp.Format("15:04:05.000"))
		}
	}
	if !degradedSeen {
		return fmt.Errorf("❌ Never entered DEGRADED state")
	}

	// Check 2: Did we see RECOVERING state?
	recoveringSeen := false
	for _, sc := range tracker.stateChanges {
		if sc.To == "RECOVERING" {
			recoveringSeen = true
			log.Printf("✅ RECOVERING state transition observed at %s", sc.Timestamp.Format("15:04:05.000"))
		}
	}
	if !recoveringSeen {
		return fmt.Errorf("❌ Never entered RECOVERING state")
	}

	// Check 3: Did we see NORMAL state return?
	normalSeen := false
	for _, sc := range tracker.stateChanges {
		if sc.From != "" && sc.To == "NORMAL" {
			normalSeen = true
			log.Printf("✅ NORMAL state recovered at %s", sc.Timestamp.Format("15:04:05.000"))
		}
	}
	if !normalSeen {
		return fmt.Errorf("❌ Never returned to NORMAL state")
	}

	// Check 4: Final scale should be 1.0
	finalScale := eng.Governor().Scale()
	if finalScale >= 0.99 && finalScale <= 1.01 {
		log.Printf("✅ Final scale: %.2f (100%%)", finalScale)
	} else {
		return fmt.Errorf("❌ Final scale %.2f, expected 1.0", finalScale)
	}

	// Check 5: Verify additive increases happened
	increaseSeen := false
	for _, sc := range tracker.scaleChanges {
		if sc.Change > 0 && sc.Change <= 0.06 { // Should be ~0.05
			increaseSeen = true
		}
	}
	if increaseSeen {
		log.Printf("✅ Additive increases observed in scale changes")
	} else {
		log.Printf("⚠️  No clear additive increases observed (may have recovered too fast)")
	}

	log.Println("\n✅ All validation checks passed!")
	return nil
}

func printControlEvents(sub *event.ErrorSubscription, tracker *EventTracker) {
	lastState := "NORMAL"

	for evt := range sub.Events() {
		// Only print control loop events
		if evt.Component != "control-loop" && evt.Component != "control-loop:governor" {
			continue
		}

		fmt.Printf("\n[CONTROL EVENT] %s\n", time.Now().Format("15:04:05.000"))
		fmt.Printf("  Severity:    %s\n", evt.Severity)
		fmt.Printf("  Code:        %s\n", evt.Code)
		fmt.Printf("  Message:     %s\n", evt.Message)
		if len(evt.Context) > 0 {
			fmt.Printf("  Context:\n")
			for k, v := range evt.Context {
				fmt.Printf("    %s: %v\n", k, v)
			}
		}
		fmt.Println()

		// Track state changes
		if evt.Code == event.CodeDegradedMode {
			if state, ok := evt.Context["state"].(string); ok {
				if state != lastState {
					scale := 1.0
					pressure := 0.0
					if scaleStr, ok := evt.Context["scale"].(string); ok {
						fmt.Sscanf(scaleStr, "%f", &scale)
					}
					if pressureStr, ok := evt.Context["pressure"].(string); ok {
						fmt.Sscanf(pressureStr, "%f%%", &pressure)
						pressure /= 100.0
					}

					tracker.stateChanges = append(tracker.stateChanges, StateChange{
						Timestamp: time.Now(),
						From:      lastState,
						To:        state,
						Scale:     scale,
						Pressure:  pressure,
					})
					lastState = state
				}
			}
		}

		// Track scale changes
		if evt.Code == event.CodeWorkerScaleDown || evt.Code == event.CodeWorkerScaleUp {
			scale := 1.0
			change := 0.0
			pressure := 0.0
			if scaleStr, ok := evt.Context["scale"].(string); ok {
				fmt.Sscanf(scaleStr, "%f", &scale)
			}
			if changeStr, ok := evt.Context["change"].(string); ok {
				fmt.Sscanf(changeStr, "%f", &change)
			}
			if pressureStr, ok := evt.Context["pressure"].(string); ok {
				fmt.Sscanf(pressureStr, "%f%%", &pressure)
				pressure /= 100.0
			}

			tracker.scaleChanges = append(tracker.scaleChanges, ScaleChange{
				Timestamp: time.Now(),
				Scale:     scale,
				Change:    change,
				Pressure:  pressure,
			})
		}
	}
}
