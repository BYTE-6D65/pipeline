Current Timing Analysis

  Data Path (Hot Path - Critical)

  Adapter → ExternalBus → Emitter
  │         │             │
  ~10μs     ~50-100μs    ~10μs     Total: ~70-120μs per event

  Breakdown:
  - Adapter publishes: ~10μs (direct channel send)
  - Bus routing: ~50-100μs (subscription matching, channel send to N subscribers)
  - Emitter processes: ~10μs + network I/O

  This is GOOD - sub-millisecond data path latency.

  Control Path (Cold Path - Not Critical)

  Memory Poll → Control Loop → InternalBus → Governor → State Change
  │             │              │             │          │
  50ms          ~100μs         ~50μs         ~10μs     30s cooldown

  Current Timings (from config.go):
  - GovernorPollInterval: 50ms - Poll memory every 50ms
  - ControlLoopInterval: 3s - (unused? control loop runs continuously)
  - ControlCooldown: 30s - Minimum 30s between scale changes

  Problems I See:

  1. Governor polls every 50ms but can only act every 30s - wasting CPU
  2. Control loop writes to ErrorBus + InternalBus - double event overhead
  3. Event marshaling on control path - adds ~10-50μs (event-driven refactor)
  4. No batching - each control decision = separate event

  What I Would Change

  1. Slow Down Control Loop Polling (Major Win)

  Current:
  GovernorPollInterval: 50ms  // Polls 20 times/second
  ControlCooldown: 30s        // Can only act every 30s

  Problem: Polling 600 times between each action (20/sec × 30s = 600 polls). That's 599 wasted polls!

  Recommendation:
  GovernorPollInterval: 1s    // Poll once per second (20x less CPU)
  ControlCooldown: 30s        // Keep 30s cooldown

  Rationale:
  - Memory pressure changes slowly (seconds, not milliseconds)
  - 1s poll interval gives you 30 samples before action
  - Even 5s poll interval would be fine (6 samples before action)
  - Your thresholds have 15% hysteresis (70% → 55%) - plenty of buffer

  Savings: 95% reduction in control path CPU usage

  2. Add Exponential Backoff When Stable (Medium Win)

  type AdaptivePollInterval struct {
      current      time.Duration
      min          time.Duration  // 1s
      max          time.Duration  // 30s
      stable       bool           // No state changes recently
  }

  func (a *AdaptivePollInterval) Adjust(stateChanged bool) time.Duration {
      if stateChanged {
          // State changed - poll faster
          a.current = a.min
          a.stable = false
      } else if !a.stable {
          // Still stable - slow down
          a.current = min(a.current * 2, a.max)
          if a.current >= a.max {
              a.stable = true
          }
      }
      return a.current
  }

  Behavior:
  [Normal operation]
  Poll: 1s, 2s, 4s, 8s, 16s, 30s, 30s... (exponential backoff)

  [Memory pressure detected]
  Poll: 1s, 1s, 1s... (fast until stable again)

  Savings: 90%+ reduction in steady-state polling

  3. Batch Control Events (Small Win)

  With event-driven refactor, you could batch multiple commands:

  Current (after refactor):
  // 3 separate events
  internalBus.Publish(GovernorScaleCommand{Scale: 0.5})
  internalBus.Publish(BufferShrinkCommand{})
  internalBus.Publish(ForceGCCommand{})

  Optimized:
  // Single batch event
  internalBus.Publish(ControlBatch{
      Commands: []ControlCommand{
          {Type: "governor.scale", Scale: 0.5},
          {Type: "buffer.shrink"},
          {Type: "gc.force"},
      },
  })

  Savings: ~100μs → ~50μs (event marshaling overhead)

  4. Remove Duplicate Observability Events (Small Win)

  Current:
  - Control loop emits to ErrorBus: "Issuing scale command"
  - Governor emits to ErrorBus: "Scale changed"

  Problem: Two events for one action (double overhead)

  Recommendation: Only governor emits (records actual state change, not intent)

  Savings: 50% reduction in control observability overhead

  5. Use Ring Buffer for ErrorBus (Medium Win)

  Current:
  ErrorBusBufferSize: 32  // Channel buffer

  Problem: Channel operations have mutex overhead

  Alternative: Lock-free ring buffer
  type LockFreeErrorBus struct {
      buffer [256]*ErrorEvent  // Power of 2 for fast modulo
      head   atomic.Uint64
      tail   atomic.Uint64
  }

  func (b *LockFreeErrorBus) Publish(evt *ErrorEvent) bool {
      h := b.head.Load()
      if h - b.tail.Load() >= 256 {
          return false  // Full, drop
      }
      b.buffer[h & 255] = evt
      b.head.Add(1)
      return true
  }

  Savings: ~100ns per ErrorBus publish (vs ~1μs for channel)

  6. Wider Hysteresis Thresholds (Major Stability Win)

  Current:
  Enter degraded: 70%
  Exit degraded: 55%
  Gap: 15%

  Problem: If memory oscillates 68% ↔ 72%, you could thrash

  Recommendation: Widen the gap
  Enter degraded: 75%  // Higher threshold
  Exit degraded: 50%   // Lower threshold
  Gap: 25%             // Wider hysteresis

  Benefit: More stable, fewer control actions, easier to reason about

  Recommended Configuration

  func OptimizedConfig() Config {
      return Config{
          // Governor (SLOWER POLLING)
          MemoryEnterThreshold: 0.75,  // Higher threshold (more headroom)
          MemoryExitThreshold:  0.50,  // Lower threshold (wider hysteresis)
          MemoryCriticalPct:    0.90,
          GovernorPollInterval: 1 * time.Second,  // 20x slower (was 50ms)

          // Control Loop (LONGER COOLDOWN)
          ControlCooldown:     60 * time.Second,  // 2x longer (was 30s)
          MaxActionsPerLoop:   1,

          // Memory budget (MORE AGGRESSIVE)
          BufferMemoryBudgetPct: 0.40,  // 40% instead of 50% (more headroom)

          // PSI (SLOWER POLLING)
          PSIEnabled:       true,
          PSIThreshold:     0.2,
          PSISustainWindow: 5 * time.Second,     // Longer sustain (was 2s)
          PSIPollInterval:  2 * time.Second,     // Slower (was 1s)

          // Flight Recorder (SLOWER SNAPSHOT)
          FlightRecorderSize:     100,
          FlightRecorderInterval: 5 * time.Second,  // Slower (was 1s)

          // Error Bus (LARGER BUFFER)
          ErrorBusBufferSize: 256,  // Bigger (was 32)
          ErrorBusSampling:   true, // Enable sampling for high-frequency errors

          // AIMD (SLOWER RECOVERY)
          AIMDIncrStep:   0.05,  // Same
          AIMDDecrFactor: 0.5,   // Same
          AIMDMaxPerTick: 0.1,   // Same

          // Rest unchanged...
      }
  }

  Time Budget Analysis

  Current System (After My Changes)

  | Layer                    | Hot Path | Cold Path | Frequency    | Critical? |
  |--------------------------|----------|-----------|--------------|-----------|
  | Data routing             | 100μs    | -         | 1000-10000/s | ✅ YES     |
  | Memory polling           | -        | 1ms       | 1/s          | ❌ No      |
  | Control decision         | -        | 100μs     | 1/60s        | ❌ No      |
  | Control command (event)  | -        | 50μs      | 1/60s        | ❌ No      |
  | State application        | -        | 10μs      | 1/60s        | ❌ No      |
  | Observability (ErrorBus) | -        | 100μs     | ~10/s        | ❌ No      |

  Total Control Path: ~1.3ms every 60 seconds = 0.002% of CPU time

  Total Data Path: 100μs × 10,000 events/s = 1 second of CPU per second (1 core)

  Optimization Impact

  | Optimization           | Savings                      | Justification                      |
  |------------------------|------------------------------|------------------------------------|
  | 50ms → 1s polling      | 95% CPU on control path      | Memory changes slowly              |
  | Adaptive polling       | 90% further reduction        | Exponential backoff when stable    |
  | 30s → 60s cooldown     | 50% fewer actions            | More time for effects to propagate |
  | Batch control events   | 50μs/action                  | Reduces event overhead             |
  | Ring buffer ErrorBus   | 90% latency on observability | Lock-free is faster                |
  | Wider hysteresis (25%) | Fewer state transitions      | More stable system                 |

  Critical Insight: Latency vs Throughput

  You're absolutely right about buffering with thresholds. Here's the key insight:

  Control Loop Latency Budget:
  Time to detect pressure: 1s (poll interval)
  Time to issue command:   100μs (event publish)
  Time to apply command:   10μs (governor state change)
  Time for effect:         ???  (This is the real delay!)

  The Real Delay: Effect Propagation

  When you scale to 0.5:
  1. Governor changes scale: instant (~10μs)
  2. Adapters see new scale: next event (0-100μs)
  3. Adapters slow down: immediate
  4. Memory stops growing: 0-1s (events in flight drain)
  5. Memory starts shrinking: GC cycle (1-10s)
  6. Memory drops below threshold: 5-30s (depends on load)

  Total effect propagation: 5-30 seconds

  So your 30-60s cooldown is PERFECT - it's longer than the effect propagation time!

  Final Recommendations

  Immediate (Zero Risk)

  1. ✅ Change GovernorPollInterval: 50ms → 1s (20x less CPU)
  2. ✅ Change ControlCooldown: 30s → 60s (more time for effects)
  3. ✅ Change MemoryEnterThreshold: 0.70 → 0.75 (wider hysteresis)
  4. ✅ Change MemoryExitThreshold: 0.55 → 0.50 (wider hysteresis)

  Later (After Event-Driven Refactor)

  5. Add adaptive polling (exponential backoff)
  6. Batch control commands
  7. Remove duplicate observability events
  8. Consider lock-free ring buffer for ErrorBus

  Don't Bother With

  - Sub-millisecond control loop (not needed)
  - Complex prediction algorithms (AIMD is good enough)
  - Async control application (current design is fine)