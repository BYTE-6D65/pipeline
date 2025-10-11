# Interactive TUI Demo - Dynamic Messaging

## Scenario 1: Running on macOS (Current Platform)

### Main Menu - Hovering over "Emit Events"
```
🎮 CmdWheel Pipeline - Interactive Menu

    🧪 Run Performance Tests
  ▶ ⚡ Emit Events (emitter test)
    🔄 Full Pipeline Test
    📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit

┌────────────────────────────────────────────────────────────────┐
│ ❌ Not Available: This feature only works on Linux!            │
│    Current platform: darwin                                    │
│    Reason: Requires /dev/uinput (Linux kernel feature)         │
│    Emitter is a stub on this platform                          │
└────────────────────────────────────────────────────────────────┘
```

### Main Menu - Hovering over "Listen to Events"
```
🎮 CmdWheel Pipeline - Interactive Menu

  ▶ 🎧 Listen to Events (adapter test)
    ⚡ Emit Events (emitter test)
    🔄 Full Pipeline Test
    📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit

┌────────────────────────────────────────────────────────────────┐
│ ⚠️  Platform Note: You're on darwin                            │
│    Adapter is a stub (architecture testing only)               │
│    For full functionality, run on Linux                        │
│    Use CLI mode: ./bin/pipeline listen                         │
└────────────────────────────────────────────────────────────────┘
```

### Main Menu - Hovering over "Monitor Event Bus"
```
🎮 CmdWheel Pipeline - Interactive Menu

    🔄 Full Pipeline Test
  ▶ 📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit

┌────────────────────────────────────────────────────────────────┐
│ ℹ️  Real-time event bus monitoring.                            │
│    Shows event counts and throughput rates.                    │
│    Works on all platforms (tests internal bus)                 │
│    Use CLI mode: ./bin/pipeline monitor --interval=1s          │
└────────────────────────────────────────────────────────────────┘
```

## Scenario 2: Running Performance Tests

### Test Menu
```
🧪 Performance Test Scenarios

  ▶ 📈 Normal Load Test (1,000 events)
    💪 Massive Payload Test (100 events @ 1MB each)
    🔥 Adversarial Test (500 events)
    ⬅️  Back to Main Menu

Use ↑/↓ or j/k to navigate • Enter to select • q to quit
```

### Running Test
```
⚡ Running normal test...

  ⠹ Processing events...

Please wait...
```

### Results with Success Message
```
✅ Test Complete

┌────────────────────────────────────────────────────────────────┐
│ ✅ Test completed successfully!                                │
│    Processed 1000 events in 13ms                               │
└────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                                                                 │
│  📊 Performance Metrics - normal scenario                       │
│                                                                 │
│  Events:     1000                                               │
│  Duration:   13ms                                               │
│                                                                 │
│  Latency:                                                       │
│    Min:      2.958µs                                            │
│    Max:      148.375µs                                          │
│    Mean:     13.195µs                                           │
│    Median:   11.666µs                                           │
│    P90:      17.583µs                                           │
│    P95:      21.041µs                                           │
│    P99:      45.125µs                                           │
│    StdDev:   10.129µs                                           │
│    Jitter:   9.824µs                                            │
│                                                                 │
│  Throughput:                                                    │
│    Events/s: 75,757.58                                          │
│                                                                 │
│  Memory:                                                        │
│    Allocated: 0.23 MB                                           │
│                                                                 │
│  GC:                                                            │
│    Collections: 0                                               │
│    Avg Pause:   0s                                              │
│    Max Pause:   0s                                              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

Press Enter to run another test • q to quit
```

## Scenario 3: Running on Linux

### Main Menu - Hovering over "Full Pipeline"
```
🎮 CmdWheel Pipeline - Interactive Menu

    ⚡ Emit Events (emitter test)
  ▶ 🔄 Full Pipeline Test
    📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit

┌────────────────────────────────────────────────────────────────┐
│ ⚠️  End-to-End Pipeline Test                                   │
│    Requires: /dev/input/* and /dev/uinput access               │
│    ⚠️  Warning: This will echo your keyboard input!            │
│    Use CLI mode: ./bin/pipeline pipeline --device=/dev/input/… │
└────────────────────────────────────────────────────────────────┘
```

### Main Menu - Hovering over "Emit Events" (Linux)
```
🎮 CmdWheel Pipeline - Interactive Menu

  ▶ ⚡ Emit Events (emitter test)
    🔄 Full Pipeline Test
    📊 Monitor Event Bus
    ❌ Exit

Use ↑/↓ or j/k to navigate • Enter to select • q to quit

┌────────────────────────────────────────────────────────────────┐
│ ℹ️  This feature tests synthetic event emitters.               │
│    Requires: /dev/uinput permissions                           │
│    Setup: sudo modprobe uinput && sudo chmod 666 /dev/uinput   │
│    Use CLI mode: ./bin/pipeline emit --key=30 --count=10       │
└────────────────────────────────────────────────────────────────┘
```

## Message Color Coding

- 🔵 **Blue** (Info): General information, works on this platform
- 🟡 **Yellow** (Warning): Works but has limitations/requirements
- 🔴 **Red** (Error): Not available on this platform
- 🟢 **Green** (Success): Operation completed successfully

## Interactive Behavior

1. **Navigate with arrow keys** → Messages clear automatically
2. **Press Enter on item** → Message appears below menu
3. **Navigate away** → Message clears
4. **Stay on item** → Message persists
5. **Run test** → See running animation
6. **Test completes** → Green success message + full metrics

## Key Features

✅ **Platform-aware**: Different messages for Linux vs macOS
✅ **Contextual**: Explains exactly why something won't work
✅ **Helpful**: Provides CLI alternatives and setup instructions
✅ **Visual**: Color-coded borders for quick understanding
✅ **Non-intrusive**: Auto-clears when navigating
✅ **Conversational**: Feels like 2-way communication

This creates a **responsive, helpful interface** where the system actively communicates with you!
