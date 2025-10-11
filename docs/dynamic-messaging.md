# Dynamic Messaging System

The TUI now includes a **dynamic messaging system** that provides context-aware feedback to users. Messages appear when you interact with features, explaining what they do, why they might not be available, and how to use them.

## Message Types

### 🔵 Info Messages (Blue Border)
- General information about features
- How-to instructions
- Platform-agnostic info

**Example:**
```
┌─────────────────────────────────────────────────────────┐
│ ℹ️  Real-time event bus monitoring.                     │
│    Shows event counts and throughput rates.             │
│    Works on all platforms (tests internal bus)          │
│    Use CLI mode: ./bin/pipeline monitor --interval=1s   │
└─────────────────────────────────────────────────────────┘
```

### 🟡 Warning Messages (Yellow Border)
- Platform limitations
- Features that work but have caveats
- Important notes before proceeding

**Example on macOS:**
```
┌─────────────────────────────────────────────────────────┐
│ ⚠️  Platform Note: You're on darwin                     │
│    Adapter is a stub (architecture testing only)        │
│    For full functionality, run on Linux                 │
│    Use CLI mode: ./bin/pipeline listen                  │
└─────────────────────────────────────────────────────────┘
```

**Example on Linux:**
```
┌─────────────────────────────────────────────────────────┐
│ ⚠️  End-to-End Pipeline Test                            │
│    Requires: /dev/input/* and /dev/uinput access        │
│    ⚠️  Warning: This will echo your keyboard input!     │
│    Use CLI mode: ./bin/pipeline pipeline ...            │
└─────────────────────────────────────────────────────────┘
```

### 🔴 Error Messages (Red Border)
- Features completely unavailable on current platform
- Missing dependencies or permissions
- Clear explanation of why it won't work

**Example on macOS:**
```
┌─────────────────────────────────────────────────────────┐
│ ❌ Not Available: This feature only works on Linux!     │
│    Current platform: darwin                             │
│    Reason: Requires /dev/uinput (Linux kernel feature)  │
│    Emitter is a stub on this platform                   │
└─────────────────────────────────────────────────────────┘
```

### 🟢 Success Messages (Green Border)
- Confirmation when tests complete
- Quick summary of what happened
- Positive feedback

**Example:**
```
┌─────────────────────────────────────────────────────────┐
│ ✅ Test completed successfully!                         │
│    Processed 1000 events in 13ms                        │
└─────────────────────────────────────────────────────────┘
```

## Platform-Aware Messaging

The system **detects your platform** (`runtime.GOOS`) and shows different messages:

### On Linux
- **Listen to Events**: Info message with setup instructions
- **Emit Events**: Info message with /dev/uinput setup
- **Full Pipeline**: Warning about keyboard echo
- **Monitor**: Info message (works everywhere)

### On macOS/Windows
- **Listen to Events**: Warning that adapter is a stub
- **Emit Events**: Error - feature not available
- **Full Pipeline**: Error - feature not available
- **Monitor**: Info message (works everywhere)

## Message Behavior

### Automatic Clearing
Messages clear automatically when you:
- Navigate up/down (arrow keys or j/k)
- Select a different menu item
- Return from results view
- Change menu contexts

### Persistence
Messages stay visible:
- While you're on the same menu item
- Until you navigate away
- During the entire view (for results)

## Two-Way Communication

The messaging system creates a **conversational feel**:

1. **User Action**: You try to select "Emit Events"
2. **System Response**: Red error box appears explaining why it's not available on macOS
3. **User Feedback**: You see exactly what's missing and where to use it
4. **Guidance**: CLI command provided for alternative access

This feels like the system is **talking back** to you, explaining its limitations and guiding you to solutions.

## Examples by Feature

### 🧪 Performance Tests
- ✅ **Works everywhere** - these are internal bus tests
- No warnings needed
- Shows success message on completion

### 🎧 Listen to Events
- **Linux**: 🔵 Info about /dev/input/* permissions
- **Other**: 🟡 Warning that it's a stub, suggests Linux

### ⚡ Emit Events
- **Linux**: 🔵 Info about /dev/uinput setup
- **Other**: 🔴 Error - completely unavailable, explains why

### 🔄 Full Pipeline
- **Linux**: 🟡 Warning about keyboard echo risk
- **Other**: 🔴 Error - needs both adapter and emitter

### 📊 Monitor Event Bus
- **All platforms**: 🔵 Info about what it does
- No platform restrictions

## Visual Flow

```
Main Menu
    │
    ├─ Select item
    │       │
    │       └─> Message appears below menu
    │
    ├─ Navigate away
    │       │
    │       └─> Message clears automatically
    │
    └─ Select "Performance Tests"
            │
            ├─> Enter test submenu (no message)
            │
            ├─> Run test
            │       │
            │       └─> Spinner animation
            │
            └─> Results view
                    │
                    └─> Success message + metrics
```

## Implementation Details

**Message Structure:**
```go
type userMessage struct {
    msgType messageType  // info, warning, error, success
    text    string       // Multi-line message content
}
```

**Styling:**
- Each type has its own `lipgloss.Style`
- Rounded borders with matching color
- Padding and margins for readability
- Icons for quick visual identification

**Platform Detection:**
```go
if runtime.GOOS == "linux" {
    // Show Linux-specific message
} else {
    // Show limitation/error message
}
```

## User Experience Benefits

1. **No Silent Failures**: Always know why something doesn't work
2. **Guided Usage**: CLI commands provided as fallback
3. **Platform Awareness**: Clear about what works where
4. **Educational**: Learn about system requirements
5. **Conversational**: Feels like 2-way communication
6. **Non-Intrusive**: Messages clear when navigating

This creates a **helpful, communicative** interface that aligns perfectly with the CmdWheel project's focus on user interaction and menu-driven design!
