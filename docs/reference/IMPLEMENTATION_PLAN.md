# Implementation Plan: Error Signaling & Fault Tolerance

## Based on Feedback Analysis

This plan incorporates production-systems wisdom from code review. Key corrections:
- Error bus must be **bounded + lossy** (not unbuffered)
- Use **runtime/metrics + GOMEMLIMIT** (not just MemStats)
- Add **PSI** (Pressure Stall Information) for pre-OOM detection
- Use **RED** (Random Early Detection) for graceful degradation
- Use **AIMD** (Additive Increase Multiplicative Decrease) for scaling
- Add **flight recorder** for crash forensics

---

## Phase 0: Critical Fixes (Before Implementation)

### Fix #1: Error Bus Architecture

**Problem**: Original design said "unbuffered bus that never blocks" - this is contradictory. Unbuffered channels block by definition.

**Correct Design**: Bounded, lossy, non-blocking fan-out

```go
// Correct implementation
type ErrorBus struct {
    subs atomic.Pointer[[]*ErrorSubscription]
    droppedCounter atomic.Uint64  // Sample dropped events
}

type ErrorSubscription struct {
    ch chan ErrorEvent  // Small bounded buffer (e.g., 32)
}

func (b *ErrorBus) Publish(e ErrorEvent) {
    subs := b.subs.Load()
    if subs == nil {
        return
    }

    for i := range *subs {
        select {
        case (*subs)[i].ch <- e:
            // Success
        default:
            // Drop diagnostic to protect data path
            // Don't emit errors about dropping errors (recursion!)
            b.droppedCounter.Add(1)
        }
    }
}
```

**Key Principle**: Never let error reporting block the critical path.

### Fix #2: ErrorEvent Structure

Add `Signal` field separate from `Severity` for control intents:

```go
type ErrorEvent struct {
    Severity    ErrorSeverity   // Log level: debug/info/warn/error/crit
    Signal      ControlSignal   // Control intent: throttle/shed/breaker_open/etc
    Code        string          // Terse: MEM_PRESSURE, BUF_SAT, etc
    Message     string          // Human readable
    Component   string          // Which component
    Timestamp   time.Time
    Context     map[string]any
    Recoverable bool
}

type ControlSignal int

const (
    SignalNone ControlSignal = iota
    SignalThrottle    // Slow down ingestion
    SignalShed        // Drop low-priority work
    SignalBreakerOpen // Circuit breaker opened
    SignalDegraded    // Entered degraded mode
    SignalRecovered   // Recovered from degraded mode
)
```

This allows routing by `Signal` without string-matching messages.

### Fix #3: Stable Error Codes

Use terse, refactor-stable codes:

```go
const (
    // Memory & Resources
    CodeMemPressure   = "MEM_PRESSURE"
    CodeMemRelief     = "MEM_RELIEF"

    // Buffering & Flow
    CodeBufSat        = "BUF_SAT"
    CodePublishBlock  = "PUBLISH_BLOCK"
    CodeDropSlow      = "DROP_SLOW"

    // Components
    CodeAdapterFail   = "ADAPTER_FAIL"
    CodeEmitterFail   = "EMITTER_FAIL"
    CodeBreakerOpen   = "BREAKER_OPEN"
    CodeBreakerHalf   = "BREAKER_HALF"
    CodeBreakerClose  = "BREAKER_CLOSE"
)
```

---

## Phase 1: Memory Sensing (Foundation)

### 1.1: Robust Container Limit Detection

```go
package engine

import (
    "os"
    "runtime"
    "strconv"
    "strings"
)

// DetectMemoryLimit returns the effective memory limit in bytes.
// Considers: GOMEMLIMIT, cgroups v1/v2, explicit config.
// Returns (limit, source, ok)
func DetectMemoryLimit() (uint64, string, bool) {
    // Priority 1: GOMEMLIMIT (Go 1.19+)
    if limit := runtime.MemoryLimit(); limit != math.MaxInt64 {
        return uint64(limit), "GOMEMLIMIT", true
    }

    // Priority 2: cgroup v2
    if limit, ok := cgroupV2MemLimit(); ok {
        return limit, "cgroup-v2", true
    }

    // Priority 3: cgroup v1
    if limit, ok := cgroupV1MemLimit(); ok {
        return limit, "cgroup-v1", true
    }

    // Priority 4: Environment variable (Apple containers, manual)
    if envLimit := os.Getenv("MEMORY_LIMIT_BYTES"); envLimit != "" {
        if limit, err := strconv.ParseUint(envLimit, 10, 64); err == nil && limit > 0 {
            return limit, "env:MEMORY_LIMIT_BYTES", true
        }
    }

    // No limit detected
    return 0, "none", false
}

func cgroupV2MemLimit() (uint64, bool) {
    b, err := os.ReadFile("/sys/fs/cgroup/memory.max")
    if err != nil {
        return 0, false
    }

    s := strings.TrimSpace(string(b))

    // Handle "max" (unlimited)
    if s == "max" {
        return 0, false
    }

    v, err := strconv.ParseUint(s, 10, 64)
    if err != nil || v == 0 || v > 1<<60 {
        // Absurd values like 2^63-1 are sentinels for "unlimited"
        return 0, false
    }

    return v, true
}

func cgroupV1MemLimit() (uint64, bool) {
    b, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
    if err != nil {
        return 0, false
    }

    v, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
    if err != nil || v == 0 || v > 1<<60 {
        return 0, false
    }

    return v, true
}

func cgroupV2CPUQuota() (float64, bool) {
    // cpu.max format: "<quota> <period>" or "max <period>"
    b, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
    if err != nil {
        return 0, false
    }

    fields := strings.Fields(string(b))
    if len(fields) != 2 || fields[0] == "max" {
        return 0, false
    }

    quota, err1 := strconv.ParseFloat(fields[0], 64)
    period, err2 := strconv.ParseFloat(fields[1], 64)

    if err1 != nil || err2 != nil || quota <= 0 || period <= 0 {
        return 0, false
    }

    return quota / period, true  // CPUs as float (e.g., 1.5 CPUs)
}
```

### 1.2: PSI (Pressure Stall Information) Monitoring

**Key Insight**: PSI gives us early warning **before** OOM kill.

```go
package engine

import (
    "bufio"
    "os"
    "strconv"
    "strings"
    "time"
)

// PSIMemory represents memory pressure stall information
type PSIMemory struct {
    Avg10  float64  // 10-second average
    Avg60  float64  // 60-second average
    Avg300 float64  // 300-second average
    Total  uint64   // Total stall time (microseconds)
}

// ReadPSIMemory reads /proc/pressure/memory
// Returns pre-OOM warning if avg10 > threshold for sustained period
func ReadPSIMemory() (PSIMemory, error) {
    f, err := os.Open("/proc/pressure/memory")
    if err != nil {
        return PSIMemory{}, err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := scanner.Text()
        // Format: "some avg10=0.00 avg60=0.00 avg300=0.00 total=0"
        if !strings.HasPrefix(line, "some ") {
            continue
        }

        fields := strings.Fields(line[5:]) // Skip "some "
        psi := PSIMemory{}

        for _, field := range fields {
            parts := strings.Split(field, "=")
            if len(parts) != 2 {
                continue
            }

            key, val := parts[0], parts[1]
            switch key {
            case "avg10":
                psi.Avg10, _ = strconv.ParseFloat(val, 64)
            case "avg60":
                psi.Avg60, _ = strconv.ParseFloat(val, 64)
            case "avg300":
                psi.Avg300, _ = strconv.ParseFloat(val, 64)
            case "total":
                psi.Total, _ = strconv.ParseUint(val, 10, 64)
            }
        }

        return psi, nil
    }

    return PSIMemory{}, scanner.Err()
}

// PSIMonitor polls PSI and emits pre-OOM warnings
type PSIMonitor struct {
    threshold     float64       // avg10 threshold (e.g., 0.2 = 20%)
    sustainWindow time.Duration // How long above threshold before alert
    interval      time.Duration // Polling interval (e.g., 1s)
    errorBus      *ErrorBus

    // State
    aboveThresholdSince time.Time
    lastAlert           time.Time
}

func (pm *PSIMonitor) Start(ctx context.Context) {
    ticker := time.NewTicker(pm.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            pm.checkPSI()
        }
    }
}

func (pm *PSIMonitor) checkPSI() {
    psi, err := ReadPSIMemory()
    if err != nil {
        // PSI not available (not Linux or old kernel)
        return
    }

    now := time.Now()

    // avg10 > threshold
    if psi.Avg10 > pm.threshold {
        if pm.aboveThresholdSince.IsZero() {
            pm.aboveThresholdSince = now
        }

        // Sustained for sustainWindow?
        if now.Sub(pm.aboveThresholdSince) >= pm.sustainWindow {
            // Don't spam - alert once per minute
            if now.Sub(pm.lastAlert) >= time.Minute {
                pm.errorBus.Publish(ErrorEvent{
                    Severity:  CriticalSeverity,
                    Signal:    SignalShed,
                    Code:      "PSI_PRE_OOM",
                    Message:   "Memory pressure sustained - pre-OOM warning",
                    Component: "monitor:psi",
                    Context: map[string]any{
                        "avg10":  psi.Avg10,
                        "avg60":  psi.Avg60,
                        "avg300": psi.Avg300,
                    },
                    Recoverable: false,
                })
                pm.lastAlert = now
            }
        }
    } else {
        // Below threshold - reset
        pm.aboveThresholdSince = time.Time{}
    }
}
```

**Default Config**:
- Threshold: `0.2` (20% pressure)
- Sustain window: `2s`
- Poll interval: `1s`

### 1.3: Runtime/Metrics Fast Polling

```go
package engine

import (
    "runtime"
    "runtime/metrics"
)

// MemoryStats provides fast memory statistics
type MemoryStats struct {
    HeapAlloc   uint64  // Currently allocated heap
    HeapSys     uint64  // Total heap from OS
    GCCount     uint32  // Number of GC runs
    Limit       uint64  // Effective memory limit
    UsagePct    float64 // Usage as percentage of limit
}

// ReadMemoryStatsFast uses runtime/metrics for fast polling
func ReadMemoryStatsFast(limit uint64) MemoryStats {
    samples := []metrics.Sample{
        {Name: "/memory/classes/heap/objects:bytes"},
        {Name: "/gc/cycles/total:gc-cycles"},
    }
    metrics.Read(samples)

    heapAlloc := samples[0].Value.Uint64()
    gcCount := uint32(samples[1].Value.Uint64())

    var usagePct float64
    if limit > 0 {
        usagePct = float64(heapAlloc) / float64(limit)
    }

    return MemoryStats{
        HeapAlloc: heapAlloc,
        GCCount:   gcCount,
        Limit:     limit,
        UsagePct:  usagePct,
    }
}
```

---

## Phase 2: Flight Recorder (Crash Forensics)

### 2.1: Ring Buffer Snapshot

```go
package engine

import (
    "runtime"
    "runtime/pprof"
    "sync"
    "time"
)

// FlightRecorder maintains last N snapshots for crash analysis
type FlightRecorder struct {
    snapshots []Snapshot
    index     int
    size      int
    mu        sync.Mutex  // Single-writer lock
}

type Snapshot struct {
    Timestamp      time.Time
    HeapBytes      uint64
    HeapSys        uint64
    NumGoroutine   int
    GCCount        uint32
    MemLimit       uint64
    QueueDepths    map[string]int     // bus:sub -> depth
    Latencies      map[string]float64 // p50/p99 by operation
    GovernorScale  float64            // Current scale factor
}

func NewFlightRecorder(size int) *FlightRecorder {
    return &FlightRecorder{
        snapshots: make([]Snapshot, size),
        size:      size,
    }
}

func (fr *FlightRecorder) Record(snap Snapshot) {
    fr.mu.Lock()
    defer fr.mu.Unlock()

    fr.snapshots[fr.index] = snap
    fr.index = (fr.index + 1) % fr.size
}

func (fr *FlightRecorder) Dump(w io.Writer) {
    fr.mu.Lock()
    snapshots := make([]Snapshot, fr.size)
    copy(snapshots, fr.snapshots)
    fr.mu.Unlock()

    fmt.Fprintf(w, "=== Flight Recorder ===\n")
    fmt.Fprintf(w, "Last %d snapshots:\n\n", fr.size)

    // Dump in chronological order
    start := fr.index
    for i := 0; i < fr.size; i++ {
        idx := (start + i) % fr.size
        snap := snapshots[idx]
        if snap.Timestamp.IsZero() {
            continue
        }

        fmt.Fprintf(w, "[%s] heap=%d/%d (%.1f%%) goroutines=%d gc=%d scale=%.2f\n",
            snap.Timestamp.Format(time.RFC3339Nano),
            snap.HeapBytes,
            snap.MemLimit,
            float64(snap.HeapBytes)/float64(snap.MemLimit)*100,
            snap.NumGoroutine,
            snap.GCCount,
            snap.GovernorScale,
        )

        for queue, depth := range snap.QueueDepths {
            fmt.Fprintf(w, "  %s: %d\n", queue, depth)
        }
    }

    // Dump pprof
    fmt.Fprintf(w, "\n=== Heap Profile ===\n")
    pprof.Lookup("heap").WriteTo(w, 0)

    fmt.Fprintf(w, "\n=== Goroutine Profile ===\n")
    pprof.Lookup("goroutine").WriteTo(w, 0)
}
```

### 2.2: Panic Guard with Dump

```go
// WrapGoroutine wraps goroutine roots with panic recovery
func (e *Engine) WrapGoroutine(name string, fn func()) {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                // Rate limit: one dump per minute
                if e.canDump() {
                    e.dumpCrashReport(name, r)
                }
            }
        }()
        fn()
    }()
}

func (e *Engine) dumpCrashReport(goroutine string, recovered any) {
    filename := fmt.Sprintf("crash-%s-%d.txt", goroutine, time.Now().Unix())
    f, err := os.Create(filename)
    if err != nil {
        return
    }
    defer f.Close()

    fmt.Fprintf(f, "=== PANIC in %s ===\n", goroutine)
    fmt.Fprintf(f, "Recovered: %v\n\n", recovered)
    fmt.Fprintf(f, "Stack trace:\n%s\n\n", debug.Stack())

    // Dump flight recorder
    e.flightRecorder.Dump(f)
}

var lastDump atomic.Int64

func (e *Engine) canDump() bool {
    now := time.Now().Unix()
    last := lastDump.Load()

    // Allow one dump per minute
    if now-last >= 60 {
        return lastDump.CompareAndSwap(last, now)
    }
    return false
}
```

---

## Phase 3: RED (Random Early Detection) Dropping

### 3.1: Probabilistic Early Drop

```go
// REDDropper implements Random Early Detection
type REDDropper struct {
    minThreshold float64  // Start dropping (e.g., 0.6)
    maxThreshold float64  // Max drop probability (e.g., 1.0)
    maxDropProb  float64  // Max drop probability (e.g., 0.3)
}

func (rd *REDDropper) ShouldDrop(fill float64) bool {
    prob := rd.DropProbability(fill)
    return rand.Float64() < prob
}

func (rd *REDDropper) DropProbability(fill float64) float64 {
    if fill <= rd.minThreshold {
        return 0.0
    }
    if fill >= rd.maxThreshold {
        return rd.maxDropProb
    }

    // Linear ramp from minThreshold to maxThreshold
    range_ := rd.maxThreshold - rd.minThreshold
    excess := fill - rd.minThreshold
    return (excess / range_) * rd.maxDropProb
}

// Default: start dropping at 60% full, reach 30% drop prob at 100% full
func NewDefaultREDDropper() *REDDropper {
    return &REDDropper{
        minThreshold: 0.6,
        maxThreshold: 1.0,
        maxDropProb:  0.3,
    }
}
```

### 3.2: Integration with Bus

```go
func (s *inMemorySubscription) send(evt *Event, red *REDDropper, busName string, metrics *telemetry.Metrics) {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.closed {
        return
    }

    fill := float64(len(s.ch)) / float64(s.bufferSize)

    // RED: Probabilistically drop before hitting hard limit
    if red != nil && red.ShouldDrop(fill) {
        if metrics != nil {
            metrics.EventsDropped.WithLabelValues(busName, evt.Type, s.id, "red").Inc()
        }
        return
    }

    // Try non-blocking send
    select {
    case s.ch <- evt:
        // Success
    default:
        // Hard limit reached - drop
        if metrics != nil {
            metrics.EventsDropped.WithLabelValues(busName, evt.Type, s.id, "full").Inc()
        }
    }
}
```

---

## Phase 4: AIMD Governor

### 4.1: Additive Increase, Multiplicative Decrease

```go
// AIMDGovernor implements additive increase, multiplicative decrease pacing
type AIMDGovernor struct {
    scale       float64       // Current scale factor (0.0 - 1.0)
    target      float64       // Target scale (1.0 = full speed)
    incrStep    float64       // Additive increase per tick (e.g., 0.05)
    decrFactor  float64       // Multiplicative decrease (e.g., 0.5)
    maxPerTick  float64       // Max change per tick (e.g., 0.1)

    state       GovernorState // Normal, Degraded, Recovering
}

type GovernorState int

const (
    StateNormal GovernorState = iota
    StateDegraded
    StateRecovering
)

func (g *AIMDGovernor) Update(memPressure float64) {
    switch g.state {
    case StateNormal:
        if memPressure > 0.70 {
            // Enter degraded mode
            g.state = StateDegraded
            g.scale *= g.decrFactor  // Multiplicative decrease (e.g., ×0.5)
            g.scale = max(g.scale, 0.1)
        }

    case StateDegraded:
        if memPressure < 0.55 {
            // Memory relieved - enter recovery
            g.state = StateRecovering
        } else if memPressure > 0.90 {
            // Still high pressure - decrease more
            g.scale *= g.decrFactor
            g.scale = max(g.scale, 0.1)
        }

    case StateRecovering:
        if memPressure < 0.55 {
            // Additive increase
            g.scale += g.incrStep
            g.scale = min(g.scale, g.target)

            if g.scale >= g.target {
                // Fully recovered
                g.state = StateNormal
            }
        } else if memPressure > 0.70 {
            // Pressure returned - back to degraded
            g.state = StateDegraded
        }
    }

    // Cap rate of change
    change := abs(g.scale - g.target)
    if change > g.maxPerTick {
        if g.scale < g.target {
            g.scale = g.target - g.maxPerTick
        } else {
            g.scale = g.target + g.maxPerTick
        }
    }
}

func (g *AIMDGovernor) Scale() float64 {
    return g.scale
}
```

---

## Quick Win Checklist (Priority Order)

### Week 1: Foundation
- [x] Fix error bus to bounded, lossy fan-out
- [x] Add `Signal` field to ErrorEvent
- [x] Use terse, stable error codes
- [ ] Implement DetectMemoryLimit with cgroup v1/v2/GOMEMLIMIT
- [ ] Add ReadMemoryStatsFast with runtime/metrics
- [ ] Add flight recorder with ring buffer

### Week 2: Early Warning
- [ ] Implement PSI poller (Linux only, graceful fallback)
- [ ] Emit MEM_PRESSURE events at 70%, 85%, 90%
- [ ] Add panic guard with crash dump

### Week 3: Graceful Degradation
- [ ] Implement RED dropper
- [ ] Add AIMD governor
- [ ] Wire governor to scale publish rate

### Week 4: Testing & Validation
- [ ] Add chaos link (drop/delay/duplicate)
- [ ] Add 2-3 failpoints for injection
- [ ] Run 50 MB stress test - verify no OOM
- [ ] Verify flight recorder dumps on crash

---

## Sensible Defaults (Ship-Ready Config)

```go
// DefaultConfig provides production-ready defaults
type Config struct {
    // Governor
    MemoryEnterThreshold  float64       // 0.70 (70%)
    MemoryExitThreshold   float64       // 0.55 (55%)
    GovernorPollInterval  time.Duration // 50ms

    // Control Loop
    ControlLoopInterval   time.Duration // 3s
    ControlCooldown       time.Duration // 30s
    MaxActionsPerLoop     int           // 1

    // Queues
    QueueSizeStart        int           // 128
    QueueSizeMin          int           // 8
    QueueSizeMax          int           // 1024
    REDMinFill            float64       // 0.6 (60%)
    REDMaxDropProb        float64       // 0.3 (30%)

    // Workers
    TargetLagMs           int           // 10ms
    MinWorkers            int           // 2
    MaxWorkers            int           // 8

    // Memory
    BufferMemoryBudgetPct float64       // 0.50 (50%)

    // PSI
    PSIThreshold          float64       // 0.2 (20%)
    PSISustainWindow      time.Duration // 2s

    // Flight Recorder
    FlightRecorderSize    int           // 100 snapshots
    FlightRecorderInterval time.Duration // 1s
}
```

---

## Next Steps

1. **Review this plan** - Any changes before implementation?
2. **Start with Week 1** - Foundation + memory sensing
3. **Test incrementally** - Each phase tested before moving to next
4. **Document as we go** - Keep README/docs updated

Ready to start implementing?
