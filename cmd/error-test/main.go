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
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("=== Pipeline Error Signaling Test ===")

	// Load config from environment
	cfg, err := engine.LoadFromEnv()
	if err != nil {
		// Use defaults if env vars not set
		cfg = engine.DefaultConfig()
		log.Printf("Using default config (no env vars found)")
	} else {
		log.Printf("Loaded config from environment")
	}

	// Set GOMEMLIMIT if specified
	memLimitEnv := os.Getenv("MEMORY_LIMIT_BYTES")
	if memLimitEnv != "" {
		log.Printf("MEMORY_LIMIT_BYTES: %s", memLimitEnv)
	}

	// Create engine with error signaling
	log.Println("Creating engine with error signaling enabled...")
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
	log.Println("Memory limit detected:")
	if limit, src, ok := engine.DetectMemoryLimit(); ok {
		log.Printf("  Limit: %s (source: %s)", engine.FormatBytes(limit), src)
	} else {
		log.Printf("  No memory limit detected")
	}

	// Start error event printer
	go func() {
		for evt := range sub.Events() {
			fmt.Printf("\n[ERROR EVENT] %s\n", time.Now().Format("15:04:05.000"))
			fmt.Printf("  Severity:    %s\n", evt.Severity)
			fmt.Printf("  Code:        %s\n", evt.Code)
			fmt.Printf("  Component:   %s\n", evt.Component)
			fmt.Printf("  Message:     %s\n", evt.Message)
			if evt.Signal != event.SignalNone {
				fmt.Printf("  Signal:      %s\n", evt.Signal)
			}
			if len(evt.Context) > 0 {
				fmt.Printf("  Context:\n")
				for k, v := range evt.Context {
					fmt.Printf("    %s: %v\n", k, v)
				}
			}
			fmt.Println()
		}
	}()

	// Allocate memory in chunks to trigger warnings
	log.Println("\nStarting memory stress test...")
	log.Println("Allocating memory in 10MB chunks every 2 seconds...")

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
				log.Printf("Allocated %d chunks (%s total) - Heap: %s / %s (%.1f%%)",
					len(chunks),
					engine.FormatBytes(uint64(len(chunks)*chunkSize)),
					engine.FormatBytes(stats.HeapAlloc),
					engine.FormatBytes(stats.Limit),
					stats.UsagePct*100,
				)
			} else {
				log.Printf("Allocated %d chunks (%s total)",
					len(chunks),
					engine.FormatBytes(uint64(len(chunks)*chunkSize)),
				)
			}

		case <-sigCh:
			log.Println("\nShutdown signal received...")
			log.Printf("Dropped events: %d", eng.ErrorBus().DroppedCount())

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
