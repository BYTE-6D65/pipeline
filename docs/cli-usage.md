# CmdWheel Pipeline - Interactive TUI

A beautiful, menu-driven TUI for testing and interacting with the CmdWheel event pipeline.

## Features

### 🎮 Interactive Menu System
- Launch with just `./bin/pipeline` - no arguments needed!
- Navigate with keyboard: `↑/↓` or `j/k`
- Select with `Enter` or `Space`
- Quit anytime with `q` or `Ctrl+C`

### 🧪 Performance Testing Suite

The TUI includes comprehensive performance tests with live metrics:

#### 1. Normal Load Test
- **1,000 events** with typical payload sizes
- Tests standard operating conditions
- Shows baseline performance metrics

#### 2. Massive Payload Test
- **100 events** @ 1MB each
- Stress tests with extremely large payloads
- Measures throughput in MB/s

#### 3. Adversarial Test
- **500 events** with pathological data
- Deeply nested JSON (10 levels)
- Unicode/emoji torture tests
- Memory fragmentation patterns

### 📊 Comprehensive Metrics

All tests display:

**Latency Statistics:**
- Min, Max, Mean, Median
- P90, P95, P99 percentiles
- Standard deviation
- Jitter (inter-event variance)

**Throughput:**
- Events per second
- MB/s (for large payloads)

**Memory:**
- Total allocated MB

**Garbage Collection:**
- Collection count
- Average pause time
- Max pause time

## Usage

### Interactive Mode (Default)
```bash
./bin/pipeline
```

This launches the TUI with a menu:
```
🎮 CmdWheel Pipeline - Interactive Menu

  ▶ 🧪 Run Performance Tests
    🎧 Listen to Events (adapter test)
    ⚡ Emit Events (emitter test)
    🔄 Full Pipeline Test
    📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit
```

### Performance Tests

Select "Run Performance Tests" to access test scenarios:

```
🧪 Performance Test Scenarios

  ▶ 📈 Normal Load Test (1,000 events)
    💪 Massive Payload Test (100 events @ 1MB each)
    🔥 Adversarial Test (500 events)
    ⬅️  Back to Main Menu
```

Each test shows:
1. **Running state** with animated spinner
2. **Complete results** with full metrics breakdown
3. Option to run another test or quit

### Command Line Mode

You can still use traditional CLI commands:

```bash
# Listen to hardware events
./bin/pipeline listen --device=/dev/input/event0 --duration=5s

# Emit synthetic events (Linux only)
./bin/pipeline emit --key=30 --count=10

# Full pipeline test (Linux only)
./bin/pipeline pipeline --device=/dev/input/event0 --duration=10s

# Monitor event bus
./bin/pipeline monitor --interval=1s

# Show version
./bin/pipeline version
```

## Example Test Results

```
✅ Test Complete

┌─────────────────────────────────────────────────────┐
│                                                     │
│  📊 Performance Metrics - normal scenario           │
│                                                     │
│  Events:     1000                                   │
│  Duration:   13ms                                   │
│                                                     │
│  Latency:                                           │
│    Min:      2.958µs                                │
│    Max:      148.375µs                              │
│    Mean:     13.195µs                               │
│    Median:   11.666µs                               │
│    P90:      17.583µs                               │
│    P95:      21.041µs                               │
│    P99:      45.125µs                               │
│    StdDev:   10.129µs                               │
│    Jitter:   9.824µs                                │
│                                                     │
│  Throughput:                                        │
│    Events/s: 75,757.58                              │
│                                                     │
│  Memory:                                            │
│    Allocated: 0.23 MB                               │
│                                                     │
│  GC:                                                │
│    Collections: 0                                   │
│    Avg Pause:   0s                                  │
│    Max Pause:   0s                                  │
│                                                     │
└─────────────────────────────────────────────────────┘

Press Enter to run another test • q to quit
```

## Platform Notes

- **macOS**: Adapters and emitters are stubs (architecture testing only)
- **Linux**: Full functionality with hardware access
  - Requires permissions for `/dev/input/*` (adapters)
  - Requires permissions for `/dev/uinput` (emitters)

## Navigation Tips

- Use **vim-style keys** (`j/k`) or **arrow keys** (`↑/↓`)
- **Enter** or **Space** to select
- **ESC** or **Enter** to go back from results
- **q** or **Ctrl+C** to quit from anywhere

## Architecture

The TUI is built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Styling
- Custom test data generators with adversarial patterns
- Real-time performance metrics collection
