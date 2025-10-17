# Pipeline

**An event-driven processing pipeline with real-time event bus and pluggable adapters.**

Pipeline is a Go library providing a flexible event processing infrastructure. Built with clean architecture principles, it offers a publish/subscribe event bus, ordered event storage, adapter/emitter management, graceful degradation under memory pressure, and a beautiful interactive TUI for testing and demonstration.

## ✨ Features

- 🎯 **Event-Driven Architecture** - Clean publish/subscribe model with graceful degradation
- 🔄 **Real-time Processing** - Ordered event storage with time-based queries and chord detection
- 🧩 **Pluggable Components** - Interface-based adapters and emitters for any event source/sink
- 📦 **Generic Registry** - Thread-safe key-value store with type-safe wrappers
- 🎮 **Interactive TUI** - Beautiful Bubble Tea interface for testing and demonstration
- 🧪 **Well-Tested** - Unit tests for core algorithms, behavior tests for system validation

## 🚀 Quick Start

### Installation

```bash
go get github.com/BYTE-6D65/pipeline
```

### Basic Usage

```go
package main

import (
    "context"
    "github.com/BYTE-6D65/pipeline/pkg/engine"
    "github.com/BYTE-6D65/pipeline/pkg/event"
)

func main() {
    // Create engine
    eng := engine.New()
    defer eng.Shutdown(context.Background())

    // Subscribe to events
    eng.ExternalBus().Subscribe(event.FilterAll(), func(evt event.Event) {
        // Process events here
        println("Received:", evt.Type)
    })

    // Publish events
    evt := event.NewEvent("user.action", "my-source", map[string]any{
        "action": "click",
        "target": "button",
    })

    eng.InternalBus().Publish(context.Background(), evt)
}
```

### Interactive Demo

The pipeline includes an interactive TUI for testing and demonstration:

```bash
# Clone the repository
git clone https://github.com/BYTE-6D65/pipeline.git
cd pipeline

# Build the demo CLI
go build -o bin/pipeline ./cmd/pipeline

# Run interactive menu
./bin/pipeline
```

Navigate with arrow keys or `j/k`, select with Enter:

```
🎮 Pipeline Demo - Interactive Menu

  ▶ 🧪 Run Performance Tests
    📊 Monitor Event Bus
    ❌ Exit
```

## 📦 Architecture

### Core Components

```
pkg/
├── event/          # Event system (bus, filters, codecs, ordered storage)
├── engine/         # Coordination layer (adapter/emitter managers)
├── registry/       # Generic key-value store with type-safe wrappers
├── clock/          # Time abstraction for testing
├── statemachine/   # Generic state machine
└── testdata/       # Test data generators and scenarios
```

### Event Flow

```
Event Source → Adapter → Internal Bus → Engine → External Bus → Emitter → Event Sink
                   ↓                                    ↓
            Raw Events                          Processed Events
```

**Example Flow:**
1. Adapter captures events from external source
2. Adapter publishes to `InternalBus`
3. `Engine` routes to `ExternalBus`
4. Subscribers receive processed events
5. Emitters send to external sinks

## 📚 Core Packages

### `pkg/event` - Event System

```go
// Create events
evt := event.NewEvent("event.type", "source-id", payload)

// Event bus
bus := event.NewBus()
bus.Subscribe(event.FilterAll(), handler)
bus.Publish(ctx, evt)

// Ordered storage
store := event.NewOrderedEventStore()
store.Append(evt)
events := store.GetRange(startTime, endTime)

// Chord detection
chords := store.DetectChords(20*time.Millisecond, 2)
```

### `pkg/engine` - Coordination Layer

```go
// Create engine with internal and external buses
// import "github.com/BYTE-6D65/pipeline/pkg/telemetry"
eng := engine.New(
    engine.WithMetrics(telemetry.InitMetrics(nil)), // optional: share metrics registry
)

// Adapter manager
adapterMgr := engine.NewAdapterManager(eng)
adapterMgr.Register(myAdapter)
adapterMgr.Start()

// Emitter manager
emitterMgr := engine.NewEmitterManager(eng)
emitterMgr.Register(myEmitter)
emitterMgr.Start()

// Lifecycle metrics are available via:
//   pipeline_engine_operations_total
//   pipeline_engine_operation_duration_seconds
// Publish/subscribe latency is exported by the event bus via:
//   pipeline_event_send_duration_seconds
//   pipeline_event_send_blocked_total
```

### `pkg/registry` - Generic Key-Value Store

```go
// Generic registry
reg := registry.NewInMemoryRegistry()
reg.Set("key", anyValue)
value, ok := reg.Get("key")

// Type-safe registry
typed := registry.NewTypedRegistry[MyType](reg)
typed.Set("key", MyType{})
value, ok := typed.Get("key")
```

### `pkg/clock` - Time Abstraction

```go
// Real clock
clk := clock.NewRealClock()

// Mock clock for testing
mockClk := clock.NewMockClock(startTime)
mockClk.Advance(time.Second)
```

### `pkg/statemachine` - State Machine

```go
// Define states and transitions
sm := statemachine.New("initial")
sm.AddTransition("initial", "trigger", "next")
sm.Trigger("trigger") // State is now "next"
```

## 🧪 Testing

### Run Tests

```bash
# Run unit tests
go test ./pkg/...

# Run behavior tests (validates system-level behavior)
cd cmd/behavior-test
go run .
```

### Testing Strategy

We use a two-tier testing approach:

**Unit Tests**: Validate core algorithms and data structures
- AIMD governor state machine (85-100% coverage)
- RED dropper probability calculations (100% coverage)
- Event bus filtering and delivery (65% coverage)
- Registry and state machine operations (97-100% coverage)

**Behavior Tests**: Validate system-level behavior under real conditions
- Memory pressure detection and response
- Graceful degradation under load
- Recovery after pressure relief
- End-to-end event flows
- Control loop interactions

**Note**: The `pkg/engine` package has low unit test coverage (~17%) by design. It's an integration layer validated primarily through behavior tests that simulate real-world scenarios. Unit test coverage metrics don't reflect the true test quality of the system.

See `docs/private/COVERAGE_REPORT.md` for detailed test analysis.

## 🎯 Use Cases

### Event Processing Pipeline
Build event-driven systems with pluggable sources and sinks.

### Real-time Data Processing
Process event streams with ordered storage and time-based queries.

### Input Device Management
Capture and process input from keyboards, mice, gamepads (see [CmdWhl](https://github.com/BYTE-6D65/CmdWhl) for example).

### State Machine Applications
Build complex state-driven applications with the included state machine.

### Configuration Management
Use the generic registry for thread-safe configuration storage with optional database backends.

## 🔧 Extending Pipeline

### Custom Adapters

Implement the `Adapter` interface to capture events from any source:

```go
type MyAdapter struct {
    // Your fields
}

func (a *MyAdapter) ID() string { return "my-adapter" }
func (a *MyAdapter) Type() string { return "custom" }
func (a *MyAdapter) Start(ctx context.Context, publish func(event.Event)) error {
    // Capture events and call publish(evt)
    return nil
}
func (a *MyAdapter) Stop() error { return nil }
```

### Custom Emitters

Implement the `Emitter` interface to send events to any sink:

```go
type MyEmitter struct {
    // Your fields
}

func (e *MyEmitter) ID() string { return "my-emitter" }
func (e *MyEmitter) Type() string { return "custom" }
func (e *MyEmitter) Start(ctx context.Context, eventChan <-chan event.Event) error {
    // Read from eventChan and send events
    return nil
}
func (e *MyEmitter) Stop() error { return nil }
```

### Database-Backed Registry

Implement the `Registry` interface for persistent storage:

```go
type PostgresRegistry struct {
    db *sql.DB
}

func (r *PostgresRegistry) Set(key string, value any) {
    // Serialize and store in database
}

func (r *PostgresRegistry) Get(key string) (any, bool) {
    // Retrieve and deserialize from database
}
// ... implement other methods
```

## 📄 License

MIT License - See LICENSE file for details

## 🙏 Acknowledgments

- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the TUI
- Event architecture inspired by CQRS/Event Sourcing patterns
- Graceful degradation inspired by TCP congestion control (AIMD)

---

**Status:** Production-ready, actively maintained

**Author:** BYTE-6D65

## 💭 Development Philosophy

This library is built using LLM-assisted development. Extensive time has been dedicated to architecture planning and logical flow. Documentation is the source of truth and the concrete reference for code generation.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
