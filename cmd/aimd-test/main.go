package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/engine"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("=== Pipeline AIMD Governor Test ===")

	// Load config from environment
	cfg, err := engine.LoadFromEnv()
	if err != nil {
		cfg = engine.DefaultConfig()
		log.Printf("Using default config")
	}

	// Create engine with AIMD + RED enabled
	log.Println("Creating engine with AIMD governor and RED dropper...")
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
	if limit, src, ok := engine.DetectMemoryLimit(); ok {
		log.Printf("  Memory Limit: %s (source: %s)", engine.FormatBytes(limit), src)
	} else {
		log.Printf("  Memory Limit: None detected")
	}
	log.Printf("  AIMD State: %s", eng.Governor().State())
	log.Printf("  AIMD Scale: %.2f (%.0f%%)", eng.Governor().Scale(), eng.Governor().Scale()*100)
	log.Printf("  RED Min Threshold: %.0f%%", eng.RED().MinThreshold()*100)
	log.Printf("  RED Max Drop Prob: %.0f%%", eng.RED().MaxDropProb()*100)

	// Start governor state printer
	go printGovernorState(eng)

	// Start error event printer
	go func() {
		for evt := range sub.Events() {
			// Print control loop events
			if evt.Component == "control-loop" || evt.Component == "control-loop:governor" {
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
			}
		}
	}()

	// Allocate memory in chunks to trigger governor
	log.Println("\nStarting memory stress test...")
	log.Println("Allocating memory in 10MB chunks every 2 seconds...")
	log.Println("Watch for governor state changes as memory pressure increases!\n")

	var chunks [][]byte
	chunkSize := 10 * 1024 * 1024 // 10MB

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Handle shutdown gracefully
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			// Allocate a chunk
			chunk := make([]byte, chunkSize)
			// Write to it to ensure it's committed
			for i := range chunk {
				chunk[i] = byte(i % 256)
			}
			chunks = append(chunks, chunk)

			// Get current memory stats
			limit, _, ok := engine.DetectMemoryLimit()
			if ok {
				stats := engine.ReadMemoryStatsFast(limit)
				log.Printf("Chunk %d allocated (%s total) - Heap: %s / %s (%.1f%%) - Governor: %s @ %.0f%%",
					len(chunks),
					engine.FormatBytes(uint64(len(chunks)*chunkSize)),
					engine.FormatBytes(stats.HeapAlloc),
					engine.FormatBytes(stats.Limit),
					stats.UsagePct*100,
					eng.Governor().State(),
					eng.Governor().Scale()*100,
				)
			} else {
				log.Printf("Chunk %d allocated (%s total) - Governor: %s @ %.0f%%",
					len(chunks),
					engine.FormatBytes(uint64(len(chunks)*chunkSize)),
					eng.Governor().State(),
					eng.Governor().Scale()*100,
				)
			}

		case <-sigCh:
			log.Println("\nShutdown signal received...")
			log.Printf("Dropped events: %d", eng.ErrorBus().DroppedCount())
			log.Printf("Final governor state: %s @ %.0f%%", eng.Governor().State(), eng.Governor().Scale()*100)

			// Shutdown engine
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := eng.Shutdown(shutdownCtx); err != nil {
				log.Printf("Shutdown error: %v", err)
			}

			log.Println("Shutdown complete")
			return
		}
	}
}

func printGovernorState(eng *engine.Engine) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	lastState := eng.Governor().State()
	lastScale := eng.Governor().Scale()

	for range ticker.C {
		currentState := eng.Governor().State()
		currentScale := eng.Governor().Scale()

		// Only print if state or scale changed significantly
		scaleChange := currentScale - lastScale
		if currentState != lastState || scaleChange > 0.01 || scaleChange < -0.01 {
			fmt.Printf("\n┌─ GOVERNOR STATUS ─────────────────────────────┐\n")
			fmt.Printf("│ State: %-10s  Scale: %.2f (%.0f%%)        │\n",
				currentState, currentScale, currentScale*100)
			if currentState != lastState {
				fmt.Printf("│ State changed: %s → %s                     │\n",
					lastState, currentState)
			}
			fmt.Printf("└───────────────────────────────────────────────┘\n\n")

			lastState = currentState
			lastScale = currentScale
		}
	}
}
