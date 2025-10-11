package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
)

const version = "0.1.0"

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	// If no arguments or "demo", launch interactive TUI
	if len(os.Args) < 2 || os.Args[1] == "demo" {
		if err := startTUI(); err != nil {
			log.Fatalf("TUI error: %v", err)
		}
		return
	}

	cmd := os.Args[1]

	switch cmd {
	case "version":
		fmt.Printf("pipeline v%s\n", version)
		fmt.Printf("Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return
	case "help", "-h", "--help":
		usage()
		return
	default:
		log.Fatalf("ERROR: unknown command %q (try 'pipeline help')", cmd)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Pipeline - Event-Driven Processing Library Demo

Usage:
  pipeline [demo]
      Launch interactive demo with performance tests

  pipeline version
      Show version and platform information

  pipeline help
      Show this help message

Examples:
  # Launch interactive demo
  pipeline

  # Run specific demo mode
  pipeline demo

  # Show version
  pipeline version

About:
  Pipeline is a high-performance event processing library for Go.
  This demo showcases the event bus, ordered storage, and performance
  characteristics through interactive test scenarios.

  The demo includes:
  - Normal load test (1,000 events)
  - Massive payload test (100 events @ 1MB each)
  - Adversarial test (500 events with pathological data)

  All tests include live progress tracking and comprehensive metrics.
`)
}
