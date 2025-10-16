# Error Signaling and Fault Tolerance Design

## Problem Statement

**Current State:**
- Pipeline has error handling (returns errors) but **no error observability**
- Errors are silently swallowed by callers - "the pipeline has never reported back and said XYZ"
- OOM crashes happen without warning - we should detect memory pressure **before** the kernel kills us
- No centralized error signaling mechanism
- No fault tolerance - system crashes instead of degrading gracefully

**User's Insight:**
> "In theory, we should know if [OOM] is about to happen, shouldn't we? And if we do, shouldn't we be able to do some error signaling?"

## Goals

1. **Error Observability** - Make all pipeline errors visible via events
2. **Predictive Fault Detection** - Detect memory pressure, buffer saturation, etc. before failure
3. **Fault Tolerance** - Graceful degradation instead of crashes
4. **Actionable Signals** - Emit error events that can trigger defensive actions

## Design

### 1. Error Event System

Create a special error event type that flows through the pipeline:

```go
// ErrorEvent represents a pipeline internal error or warning
type ErrorEvent struct {
	Severity    ErrorSeverity  // Debug, Info, Warning, Error, Critical
	Code        string         // Structured error code (e.g., "BUS_PUBLISH_BLOCKED")
	Message     string         // Human-readable message
	Component   string         // Which component reported this (e.g., "bus:internal", "adapter:http-server")
	Timestamp   time.Time      // When the error occurred
	Context     map[string]any // Additional context (memory usage, buffer levels, etc.)
	StackTrace  string         // Optional stack trace for errors
	Recoverable bool           // Can the system continue operating?
}

type ErrorSeverity int

const (
	DebugSeverity    ErrorSeverity = iota // Verbose debugging
	InfoSeverity                           // Informational (e.g., "back pressure activated")
	WarningSeverity                        // Warning but not critical (e.g., "buffer 80% full")
	ErrorSeverity                          // Error but recoverable (e.g., "event dropped")
	CriticalSeverity                       // Critical, may cause crash (e.g., "memory >90%")
)
```

**Error Event Types:**
- `pipeline.error.memory_pressure` - Memory usage approaching limits
- `pipeline.error.buffer_saturated` - Channel buffer near capacity
- `pipeline.error.publish_blocked` - Publish is blocking due to slow subscriber
- `pipeline.error.event_dropped` - Event was dropped (dropSlow=true)
- `pipeline.error.adapter_failed` - Adapter encountered an error
- `pipeline.error.emitter_failed` - Emitter encountered an error

### 2. Memory Pressure Detection

Add runtime memory monitoring that emits warnings before OOM:

```go
// MemoryMonitor watches system memory and emits warning events
type MemoryMonitor struct {
	interval     time.Duration
	threshold    float64  // % of allocated memory before warning (e.g., 0.85)
	maxMemory    uint64   // Container memory limit (detected or configured)
	errorBus     Bus      // Where to send error events
	metrics      *telemetry.Metrics
}

func (m *MemoryMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkMemory()
		}
	}
}

func (m *MemoryMonitor) checkMemory() {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	usage := float64(stats.Alloc) / float64(m.maxMemory)

	if usage > m.threshold {
		severity := WarningSeverity
		if usage > 0.95 {
			severity = CriticalSeverity
		} else if usage > 0.90 {
			severity = ErrorSeverity
		}

		m.errorBus.Publish(context.Background(), &ErrorEvent{
			Severity:  severity,
			Code:      "MEMORY_PRESSURE",
			Message:   fmt.Sprintf("Memory usage at %.1f%%", usage*100),
			Component: "monitor:memory",
			Context: map[string]any{
				"alloc_bytes":    stats.Alloc,
				"sys_bytes":      stats.Sys,
				"gc_count":       stats.NumGC,
				"max_bytes":      m.maxMemory,
				"usage_percent":  usage * 100,
			},
			Recoverable: usage < 0.95,
		})
	}
}
```

**How to detect container memory limit:**
- Linux: Read `/sys/fs/cgroup/memory/memory.limit_in_bytes` (cgroup v1) or `/sys/fs/cgroup/memory.max` (cgroup v2)
- macOS/Apple containers: May need to be configured explicitly
- Fallback: Use `MEMORY_LIMIT` environment variable

### 3. Buffer Saturation Detection

Enhance existing buffer metrics with warning events:

```go
// In bus.go, add saturation detection
func (s *inMemorySubscription) send(evt *Event, dropSlow bool, busName string, metrics *telemetry.Metrics) {
	// ... existing code ...

	bufferUsage := float64(len(s.ch)) / float64(s.bufferSize)

	// Emit warning if buffer is getting full
	if bufferUsage > 0.8 && !dropSlow {
		// Emit buffer saturation warning
		s.bus.emitError(ErrorEvent{
			Severity:  WarningSeverity,
			Code:      "BUFFER_SATURATED",
			Message:   fmt.Sprintf("Subscription buffer %.1f%% full", bufferUsage*100),
			Component: fmt.Sprintf("bus:%s:sub:%s", busName, s.id),
			Context: map[string]any{
				"buffer_size":    s.bufferSize,
				"buffer_used":    len(s.ch),
				"usage_percent":  bufferUsage * 100,
			},
			Recoverable: true,
		})
	}
}
```

### 4. Error Bus Architecture

Create a **dedicated error bus** to avoid recursive errors:

```
┌─────────────────────────────────────────────────────────┐
│                       Engine                            │
│                                                         │
│  ┌────────────┐      ┌────────────┐      ┌──────────┐ │
│  │ Internal   │      │ External   │      │  Error   │ │
│  │    Bus     │─────▶│    Bus     │      │   Bus    │ │
│  └────────────┘      └────────────┘      └──────────┘ │
│        │                    │                    ▲     │
│        │                    │                    │     │
│        ▼                    ▼                    │     │
│  ┌──────────┐        ┌──────────┐        ┌──────────┐│
│  │ Adapters │        │ Emitters │        │Monitors  ││
│  │          │        │          │        │Memory    ││
│  │          │        │          │        │Health    ││
│  └──────────┘        └──────────┘        └──────────┘│
└─────────────────────────────────────────────────────────┘
                                                  │
                                                  ▼
                                          ┌───────────────┐
                                          │ Error Handler │
                                          │ - Log errors  │
                                          │ - Emit metrics│
                                          │ - Take action │
                                          └───────────────┘
```

**Key Principles:**
- Error bus is **unbuffered** or has **large buffer** to avoid blocking
- Error bus is **dropSlow=true** to never block on error reporting
- Monitors publish to error bus, not internal/external bus

### 5. Fault Tolerance Mechanisms

#### A. Back Pressure Activation (Already Exists!)
Your current buffer size of 8 with blocking sends **is** a fault tolerance mechanism:
- When buffers fill, publishers block (back pressure)
- This naturally slows down the system instead of crashing

**Enhancement**: Emit events when back pressure activates so you can see it:
```go
if len(s.ch) >= s.bufferSize {
	if metrics != nil {
		metrics.SendBlocked.WithLabelValues(busName, s.id).Inc()
		// NEW: Emit error event
		s.bus.emitError(ErrorEvent{
			Severity:    InfoSeverity, // Informational - this is expected
			Code:        "BACK_PRESSURE_ACTIVE",
			Message:     "Back pressure activated, publisher blocked",
			Component:   fmt.Sprintf("bus:%s:sub:%s", busName, s.id),
			Recoverable: true,
		})
	}
}
```

#### B. Circuit Breaker Pattern
For adapters/emitters that repeatedly fail:

```go
type CircuitBreaker struct {
	failures     int
	threshold    int       // Failures before opening
	timeout      time.Duration
	state        State     // Closed, Open, HalfOpen
	lastFailure  time.Time
}

// In adapter manager
func (m *AdapterManager) Start() error {
	for _, adapter := range m.adapters {
		if err := m.circuit.Call(func() error {
			return adapter.Start(...)
		}); err != nil {
			// Circuit breaker opened, emit error event
			m.errorBus.Publish(ErrorEvent{
				Severity:    CriticalSeverity,
				Code:        "ADAPTER_CIRCUIT_OPEN",
				Message:     fmt.Sprintf("Adapter %s circuit breaker opened", adapter.ID()),
				Component:   fmt.Sprintf("adapter:%s", adapter.ID()),
				Recoverable: false,
			})
		}
	}
}
```

#### C. Graceful Degradation on Memory Pressure
When memory pressure is detected, automatically:
1. Reduce buffer sizes
2. Enable dropSlow mode
3. Rate limit adapter ingestion
4. Force GC

```go
func (e *Engine) HandleMemoryPressure(level float64) {
	if level > 0.90 {
		// Critical: Enable aggressive dropping
		e.internalBus.dropSlow = true
		e.externalBus.dropSlow = true
		runtime.GC() // Force immediate GC

		e.errorBus.Publish(ErrorEvent{
			Severity:    CriticalSeverity,
			Code:        "DEGRADED_MODE_ACTIVATED",
			Message:     "Entering degraded mode due to memory pressure",
			Component:   "engine",
			Context:     map[string]any{"memory_usage": level},
			Recoverable: true,
		})
	}
}
```

### 6. Error Handler Interface

Allow users to subscribe to errors and take action:

```go
type ErrorHandler interface {
	HandleError(ctx context.Context, err ErrorEvent)
}

// Example: Log all errors
type LoggingErrorHandler struct {
	logger *log.Logger
}

func (h *LoggingErrorHandler) HandleError(ctx context.Context, err ErrorEvent) {
	h.logger.Printf("[%s] %s: %s - %v", err.Severity, err.Component, err.Message, err.Context)
}

// Example: Shut down on critical errors
type ShutdownErrorHandler struct {
	engine *Engine
}

func (h *ShutdownErrorHandler) HandleError(ctx context.Context, err ErrorEvent) {
	if err.Severity == CriticalSeverity && !err.Recoverable {
		log.Printf("CRITICAL ERROR: %s - Initiating graceful shutdown", err.Message)
		h.engine.Shutdown(context.Background())
	}
}
```

## Implementation Plan

### Phase 1: Error Event System (Foundation)
1. Add `ErrorEvent` type to pkg/event
2. Create dedicated error bus in Engine
3. Add `emitError()` helper to Bus

### Phase 2: Memory Monitoring
1. Create `MemoryMonitor` in pkg/engine
2. Add container memory limit detection
3. Emit memory pressure events at 85%, 90%, 95%
4. Add to Prometheus metrics

### Phase 3: Enhanced Observability
1. Emit buffer saturation warnings
2. Emit back pressure activation events
3. Add error handler interface

### Phase 4: Fault Tolerance
1. Implement circuit breaker for adapters
2. Add graceful degradation on memory pressure
3. Add rate limiting for adapters

### Phase 5: Testing
1. Unit tests for memory monitor
2. Integration tests for error flows
3. Stress tests that trigger memory pressure
4. Verify error events appear in Grafana

## Expected Benefits

### Before (Current State)
```
[Memory usage climbing...]
38 MB payload → 935 MB memory
[Silence...]
[OOM Kill by kernel]
Container: relay-node killed (signal 9)
```

### After (With Error Signaling)
```
[Memory usage climbing...]
INFO: Back pressure activated - buffer saturation detected
WARNING: Memory usage at 87% (815 MB / 1024 MB)
WARNING: Buffer saturation on bus:external:sub-0 (7/8 used)
ERROR: Memory usage at 92% (940 MB / 1024 MB)
CRITICAL: Memory usage at 95% (972 MB / 1024 MB) - entering degraded mode
[System enables dropSlow, forces GC, continues operating]
ERROR: Event dropped due to memory pressure (degraded mode active)
[Memory stabilizes or...]
CRITICAL: Memory usage at 98% - graceful shutdown initiated
[Clean shutdown before OOM]
```

## Configuration

Add to Engine options:

```go
engine.New(
	engine.WithMemoryMonitor(
		engine.MemoryLimitBytes(1024 * 1024 * 1024), // 1 GB
		engine.MemoryWarningThreshold(0.85),
		engine.MemoryCheckInterval(1 * time.Second),
	),
	engine.WithErrorHandler(myErrorHandler),
	engine.WithGracefulDegradation(true),
)
```

## Questions for Discussion

1. **Error Bus vs Logging**: Should errors go through a bus (can be filtered/subscribed) or just to a logger?
   - **Recommendation**: Both - emit events that can be subscribed to AND logged

2. **Memory Limit Detection**: How to reliably detect container limits on Apple containers?
   - **Recommendation**: Support both auto-detection (Linux cgroups) and explicit configuration

3. **Graceful Degradation**: Should it be automatic or require manual intervention?
   - **Recommendation**: Automatic with opt-out - default to degrading gracefully

4. **Error Event Overhead**: Will emitting error events during high load cause more problems?
   - **Recommendation**: Error bus is dropSlow=true and unbuffered - never blocks

5. **Backward Compatibility**: How to add this without breaking existing code?
   - **Recommendation**: Error bus is optional, defaults to no-op handler
