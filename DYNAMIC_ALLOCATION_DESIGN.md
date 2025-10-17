# Dynamic Resource Allocation Design

## Vision

Build a self-adjusting pipeline that dynamically scales resources based on observed constraints:
- **Dynamic buffer sizes** - Grow/shrink channel buffers based on saturation and memory pressure
- **Dynamic adapter/emitter count** - Spawn/destroy workers based on load and capacity
- **Adaptive to container constraints** - Respect memory/CPU limits without crashing

## Prerequisites

**Error Signaling System** (from ERROR_SIGNALING_DESIGN.md) provides the sensor data:
- Memory pressure detection → Know available memory headroom
- Buffer saturation detection → Know when capacity is insufficient
- Back pressure events → Know when publishers are blocked
- Performance metrics → Know throughput and latency

**These signals are the "known variables" for allocation decisions.**

---

## Part 1: Dynamic Buffer Sizing

### Current State
```go
// Fixed at engine creation
bus := event.NewInMemoryBus(
    event.WithBufferSize(8),  // Never changes
)
```

### Proposed: Adaptive Buffers

```go
type AdaptiveBuffer struct {
    current      int       // Current buffer size
    min          int       // Minimum size (e.g., 8)
    max          int       // Maximum size (constrained by memory)
    target       float64   // Target utilization (e.g., 0.7 = 70%)
    growthFactor float64   // How fast to grow (e.g., 1.5x)
    shrinkFactor float64   // How fast to shrink (e.g., 0.75x)

    // Constraints from error signaling
    memoryBudget uint64    // Available memory for buffers
    memoryUsage  float64   // Current memory pressure (0.0-1.0)
}

func (ab *AdaptiveBuffer) Adjust(saturation float64, memoryPressure float64) int {
    // Don't adjust if memory pressure is high
    if memoryPressure > 0.90 {
        // Under pressure: shrink buffer to free memory
        if ab.current > ab.min {
            ab.current = int(float64(ab.current) * ab.shrinkFactor)
            if ab.current < ab.min {
                ab.current = ab.min
            }
            return ab.current
        }
    }

    // Buffer is saturated and we have memory headroom
    if saturation > ab.target && memoryPressure < 0.75 {
        newSize := int(float64(ab.current) * ab.growthFactor)
        if newSize <= ab.max && ab.canAfford(newSize) {
            ab.current = newSize
            return ab.current
        }
    }

    // Buffer is under-utilized - shrink to free memory
    if saturation < 0.3 && ab.current > ab.min {
        ab.current = int(float64(ab.current) * ab.shrinkFactor)
        if ab.current < ab.min {
            ab.current = ab.min
        }
    }

    return ab.current
}

func (ab *AdaptiveBuffer) canAfford(newSize int) bool {
    // Estimate memory cost: buffer_size * avg_event_size
    // Get avg_event_size from metrics
    estimatedCost := uint64(newSize) * avgEventSize
    return estimatedCost < ab.memoryBudget
}
```

### Challenge: Resizing Live Channels

**Problem**: Go channels can't be resized after creation.

**Solutions:**

#### Option A: Buffer Migration (Safe but Complex)
```go
func (b *InMemoryBus) ResizeSubscriptionBuffer(subID string, newSize int) error {
    sub := b.subscriptions[subID]

    // Create new channel with new size
    newCh := make(chan *Event, newSize)

    // Migrate buffered events from old to new
    close(sub.ch)  // Stop new sends
    for evt := range sub.ch {
        newCh <- evt  // Drain old, fill new
    }

    // Atomically swap
    sub.mu.Lock()
    sub.ch = newCh
    sub.bufferSize = newSize
    sub.closed = false
    sub.mu.Unlock()

    return nil
}
```

**Pros**: No event loss
**Cons**: Complex, brief service interruption

#### Option B: Rate-Based Backoff (Simpler)
Instead of resizing channels, adjust **publish rate** based on saturation:

```go
type AdaptivePublisher struct {
    baseInterval time.Duration  // Normal publish interval
    backoffRate  float64        // Current backoff multiplier (1.0 = normal)

    // Adjusted by error signals
    bufferSaturation float64    // From metrics
    memoryPressure   float64    // From memory monitor
}

func (ap *AdaptivePublisher) Publish(ctx context.Context, evt *Event) error {
    // Apply adaptive delay before publishing
    if ap.backoffRate > 1.0 {
        time.Sleep(ap.baseInterval * time.Duration(ap.backoffRate))
    }

    return ap.bus.Publish(ctx, evt)
}

func (ap *AdaptivePublisher) AdjustRate() {
    // Increase backoff if saturated or memory pressure
    if ap.bufferSaturation > 0.8 || ap.memoryPressure > 0.85 {
        ap.backoffRate = min(ap.backoffRate * 1.2, 10.0)  // Max 10x slowdown
    } else if ap.bufferSaturation < 0.5 && ap.memoryPressure < 0.70 {
        // Reduce backoff if capacity available
        ap.backoffRate = max(ap.backoffRate * 0.9, 1.0)   // Min 1.0 (no backoff)
    }
}
```

**Pros**: Simple, no channel migration
**Cons**: Controls rate, not buffer capacity

#### Option C: Hybrid (Recommended)
- Use **rate-based backoff** for immediate response to pressure
- Use **buffer migration** during low-traffic windows
- Track "optimal buffer size" metric and resize opportunistically

---

## Part 2: Dynamic Adapter/Emitter Scaling

### Current State
```go
// Fixed at startup
for port in [:8080, :8081, :8082] {
    adapter := http.NewServerAdapter(port)
    adapterMgr.Register(adapter)
}
// Never changes
```

### Proposed: Elastic Adapter Pool

```go
type AdapterPool struct {
    template     AdapterFactory      // How to create new adapters
    instances    []Adapter           // Currently running adapters
    min          int                 // Minimum instances (e.g., 1)
    max          int                 // Maximum instances (e.g., 10)
    targetUtil   float64             // Target utilization (e.g., 0.7)

    // Signals from error system
    queueDepth   int                 // Buffered events waiting
    processingTime time.Duration      // Avg time per event
    errorRate    float64             // Recent error percentage
    memoryPressure float64           // From memory monitor
}

func (ap *AdapterPool) Scale(ctx context.Context) {
    utilization := ap.calculateUtilization()

    // Scale up if:
    // - High utilization
    // - Low error rate (not failing, just overloaded)
    // - Memory headroom available
    if utilization > ap.targetUtil &&
       ap.errorRate < 0.05 &&
       ap.memoryPressure < 0.75 &&
       len(ap.instances) < ap.max {

        ap.scaleUp(ctx)
    }

    // Scale down if:
    // - Low utilization
    // - Above minimum instances
    // - No recent scale events (avoid thrashing)
    if utilization < 0.3 &&
       len(ap.instances) > ap.min &&
       time.Since(ap.lastScaleEvent) > 5*time.Minute {

        ap.scaleDown(ctx)
    }

    // Force scale down on memory pressure (defensive)
    if ap.memoryPressure > 0.90 && len(ap.instances) > ap.min {
        ap.scaleDown(ctx)
    }
}

func (ap *AdapterPool) scaleUp(ctx context.Context) error {
    adapter := ap.template.Create()  // Create new instance

    if err := adapter.Start(ctx, ap.bus, ap.clock); err != nil {
        return err
    }

    ap.instances = append(ap.instances, adapter)

    // Emit event for observability
    ap.errorBus.Publish(ErrorEvent{
        Severity:  InfoSeverity,
        Code:      "ADAPTER_SCALED_UP",
        Message:   fmt.Sprintf("Added adapter, now %d instances", len(ap.instances)),
        Component: "pool:adapter",
        Context: map[string]any{
            "count":       len(ap.instances),
            "utilization": ap.calculateUtilization(),
        },
    })

    return nil
}

func (ap *AdapterPool) scaleDown(ctx context.Context) error {
    if len(ap.instances) <= ap.min {
        return nil
    }

    // Remove last adapter (LIFO)
    adapter := ap.instances[len(ap.instances)-1]
    ap.instances = ap.instances[:len(ap.instances)-1]

    // Gracefully stop adapter
    if err := adapter.Stop(); err != nil {
        return err
    }

    ap.errorBus.Publish(ErrorEvent{
        Severity:  InfoSeverity,
        Code:      "ADAPTER_SCALED_DOWN",
        Message:   fmt.Sprintf("Removed adapter, now %d instances", len(ap.instances)),
        Component: "pool:adapter",
    })

    return nil
}

func (ap *AdapterPool) calculateUtilization() float64 {
    // Multiple ways to measure utilization:

    // 1. Queue depth approach
    if ap.queueDepth == 0 {
        return 0.0
    }
    targetQueueDepth := float64(len(ap.instances) * 10)  // 10 events per adapter
    return float64(ap.queueDepth) / targetQueueDepth

    // 2. Processing time approach
    // If processing time approaches inter-arrival time, we're saturated

    // 3. Buffer saturation approach (from error signals)
    // If buffers are full, utilization is 1.0
}
```

### Challenges: Port Allocation

**Problem**: HTTP adapters need unique ports. Can't just spawn more on same port.

**Solutions:**

#### For HTTP Server Adapters
```go
// Option A: Port range allocation
type HTTPAdapterFactory struct {
    basePort int
    nextPort int
}

func (f *HTTPAdapterFactory) Create() Adapter {
    port := f.nextPort
    f.nextPort++
    return http.NewServerAdapter(fmt.Sprintf(":%d", port))
}

// Option B: Single port with connection pooling
// Don't scale adapters - scale worker goroutines per adapter
type HTTPServerAdapter struct {
    server   *http.Server
    workerPool *WorkerPool  // Scale this instead
}
```

#### For HTTP Client Emitters
```go
// Easier: clients don't need unique ports
// Just spawn more emitter instances
emitters := make([]*http.ClientEmitter, desiredCount)
for i := 0; i < desiredCount; i++ {
    emitters[i] = http.NewClientEmitter()
}
```

**Recommendation**:
- HTTP servers: Scale **workers per adapter**, not adapter count
- HTTP clients: Scale emitter count
- Future protocols (TCP listeners): Scale adapter count

---

## Part 3: Control Loop Architecture

### Feedback Control System

```
                    ┌─────────────────────────────────┐
                    │      Error Signaling System     │
                    │  - Memory Monitor               │
                    │  - Buffer Saturation Detector   │
                    │  - Back Pressure Detector       │
                    └─────────────┬───────────────────┘
                                  │ Error Events
                                  ▼
                    ┌─────────────────────────────────┐
                    │    Resource Controller          │
                    │  - Collects signals             │
                    │  - Makes scaling decisions      │
                    │  - Enforces constraints         │
                    └─────────────┬───────────────────┘
                                  │ Actions
                    ┌─────────────┴───────────────────┐
                    │                                 │
                    ▼                                 ▼
        ┌──────────────────────┐      ┌──────────────────────┐
        │  Buffer Adjuster     │      │  Adapter Scaler      │
        │  - Resize buffers    │      │  - Add/remove        │
        │  - Adjust rates      │      │    adapters          │
        └──────────────────────┘      └──────────────────────┘
```

### Implementation

```go
type ResourceController struct {
    engine         *Engine
    errorBus       Bus

    // Current state (from error signals)
    memoryPressure float64
    bufferSaturation map[string]float64  // per subscription
    backPressure   bool

    // Configured limits (container constraints)
    memoryLimit    uint64
    cpuLimit       int

    // Control knobs
    bufferAdjuster *AdaptiveBuffer
    adapterScaler  *AdapterPool

    // Hysteresis to avoid thrashing
    lastAction     time.Time
    cooldown       time.Duration
}

func (rc *ResourceController) Start(ctx context.Context) {
    // Subscribe to error events
    sub, _ := rc.errorBus.Subscribe(ctx, Filter{
        Types: []string{"pipeline.error.*"},
    })

    ticker := time.NewTicker(5 * time.Second)  // Control loop interval
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return

        case evt := <-sub.Events():
            rc.updateState(evt)  // Update controller state from error events

        case <-ticker.C:
            rc.adjust()  // Make control decisions
        }
    }
}

func (rc *ResourceController) updateState(evt *Event) {
    var errEvt ErrorEvent
    evt.DecodePayload(&errEvt, JSONCodec{})

    switch errEvt.Code {
    case "MEMORY_PRESSURE":
        rc.memoryPressure = errEvt.Context["usage_percent"].(float64) / 100

    case "BUFFER_SATURATED":
        subID := errEvt.Component  // e.g., "bus:external:sub:0"
        rc.bufferSaturation[subID] = errEvt.Context["usage_percent"].(float64) / 100

    case "BACK_PRESSURE_ACTIVE":
        rc.backPressure = true
    }
}

func (rc *ResourceController) adjust() {
    // Respect cooldown to avoid thrashing
    if time.Since(rc.lastAction) < rc.cooldown {
        return
    }

    // Priority 1: Handle memory pressure (defensive)
    if rc.memoryPressure > 0.90 {
        rc.handleMemoryPressure()
        rc.lastAction = time.Now()
        return
    }

    // Priority 2: Handle buffer saturation (capacity)
    if rc.isBufferSaturated() {
        rc.handleBufferSaturation()
        rc.lastAction = time.Now()
        return
    }

    // Priority 3: Optimize (scale down if under-utilized)
    if rc.memoryPressure < 0.50 && !rc.isBufferSaturated() {
        rc.optimize()
        rc.lastAction = time.Now()
    }
}

func (rc *ResourceController) handleMemoryPressure() {
    // Defensive actions to free memory

    // 1. Shrink buffers
    for _, sub := range rc.engine.InternalBus().subscriptions {
        rc.bufferAdjuster.Adjust(0, rc.memoryPressure)
    }

    // 2. Scale down adapters
    rc.adapterScaler.scaleDown(context.Background())

    // 3. Enable aggressive dropping
    rc.engine.InternalBus().dropSlow = true
    rc.engine.ExternalBus().dropSlow = true

    // 4. Force GC
    runtime.GC()
}

func (rc *ResourceController) handleBufferSaturation() {
    // Try to increase capacity if memory allows

    avgSaturation := rc.avgBufferSaturation()

    if rc.memoryPressure < 0.75 {
        // We have memory headroom - grow buffers
        rc.bufferAdjuster.Adjust(avgSaturation, rc.memoryPressure)
    } else {
        // No memory headroom - apply rate limiting instead
        rc.applyRateLimiting()
    }
}

func (rc *ResourceController) optimize() {
    // Under-utilized - free up resources

    // Shrink oversized buffers
    for _, sub := range rc.engine.InternalBus().subscriptions {
        saturation := rc.bufferSaturation[sub.id]
        if saturation < 0.3 {
            rc.bufferAdjuster.Adjust(saturation, rc.memoryPressure)
        }
    }

    // Scale down adapters if idle
    rc.adapterScaler.scaleDown(context.Background())
}
```

---

## Constraint Discovery

### Container Memory Limit Detection

```go
func DetectMemoryLimit() (uint64, error) {
    // Try cgroup v2 (modern)
    if limit, err := readCgroupV2MemoryLimit(); err == nil {
        return limit, nil
    }

    // Try cgroup v1 (legacy)
    if limit, err := readCgroupV1MemoryLimit(); err == nil {
        return limit, nil
    }

    // Try environment variable (Apple containers, manual config)
    if envLimit := os.Getenv("MEMORY_LIMIT_BYTES"); envLimit != "" {
        return strconv.ParseUint(envLimit, 10, 64)
    }

    // Fallback: use system memory (may be huge, not container limit)
    return getSystemMemory(), nil
}

func readCgroupV2MemoryLimit() (uint64, error) {
    // Linux cgroup v2
    data, err := os.ReadFile("/sys/fs/cgroup/memory.max")
    if err != nil {
        return 0, err
    }
    return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}

func readCgroupV1MemoryLimit() (uint64, error) {
    // Linux cgroup v1
    data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
    if err != nil {
        return 0, err
    }
    return strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
}
```

### CPU Limit Detection

```go
func DetectCPULimit() (int, error) {
    // cgroup v2
    if cpus, err := readCgroupV2CPULimit(); err == nil {
        return cpus, nil
    }

    // cgroup v1
    if cpus, err := readCgroupV1CPULimit(); err == nil {
        return cpus, nil
    }

    // Fallback: runtime.NumCPU()
    return runtime.NumCPU(), nil
}
```

---

## Implementation Phases

### Phase 0: Foundation (Current Work)
✅ Error signaling system with memory/buffer/pressure detection

### Phase 1: Constraint Discovery
- Implement container limit detection (memory, CPU)
- Add to Engine configuration
- Test on Linux containers and Apple containers

### Phase 2: Adaptive Rate Control (Simpler)
- Implement rate-based backoff for publishers
- Adjust publish rate based on buffer saturation + memory pressure
- Test under stress (should prevent OOM)

### Phase 3: Dynamic Buffer Sizing (Complex)
- Implement buffer resize logic (migration or rate-based)
- Test buffer growth/shrink under varying load
- Measure memory impact

### Phase 4: Resource Controller (Integration)
- Build control loop that subscribes to error events
- Implement decision logic (priority-based)
- Add hysteresis/cooldown to avoid thrashing

### Phase 5: Adapter Scaling (Advanced)
- Design adapter pool abstraction
- Implement for client emitters (easier)
- Consider worker pool scaling for servers

### Phase 6: Testing & Validation
- Stress tests that trigger all control paths
- Verify no OOM under extreme load
- Verify graceful degradation
- Benchmark overhead of control system

---

## Key Principles

1. **Observe Before Acting** - Error signaling provides the sensor data
2. **Degrade Gracefully** - Reduce capacity rather than crash
3. **Respect Constraints** - Never exceed container limits
4. **Avoid Thrashing** - Use hysteresis and cooldown periods
5. **Priority Ordering** - Memory pressure > Capacity > Optimization
6. **Fail Safe** - If in doubt, shrink/throttle

---

## Expected Behavior

### Scenario: Gradual Load Increase

```
[Load increases, buffers fill]
INFO: Buffer saturation 75% - growing buffer 8 → 12
INFO: Buffer saturation 80% - growing buffer 12 → 18

[Memory pressure starts]
WARNING: Memory 87% - throttling buffer growth

[Load continues]
WARNING: Buffer saturation 85%, memory 87% - applying rate limiting
INFO: Publish rate limited to 0.8x normal

[Load peaks]
CRITICAL: Memory 92% - entering defensive mode
  - Shrinking buffers: 18 → 12
  - Enabling dropSlow
  - Forcing GC

[Load decreases]
INFO: Memory 78%, buffers 40% - exiting defensive mode
INFO: Removing rate limit

[Steady state]
INFO: Buffers under-utilized (30%) - shrinking 12 → 8
```

### vs Current Behavior (No Dynamic Allocation)

```
[Load increases, fixed buffer=8]
[Buffers saturate at 100%]
[Back pressure blocks publishers]
[Memory continues growing...]
[OOM kill at 95%]
SIGKILL
```

---

## Questions for Discussion

1. **Control Loop Interval**: How often to check and adjust? (Proposal: 5s)

2. **Hysteresis Window**: How long to wait between adjustments? (Proposal: 30s)

3. **Buffer Size Limits**: Min/max buffer sizes? (Proposal: min=8, max=1024)

4. **Scaling Strategy**: Buffer resize vs rate limiting vs both?

5. **Adapter Scaling**: Worker pool per adapter vs spawning more adapters?

6. **Memory Budget**: What % of container limit to use for buffers? (Proposal: 50%)

7. **Testing Strategy**: How to validate this under realistic stress?
