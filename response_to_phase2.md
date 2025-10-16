container exec relay-test sh -c 'GOMEMLIMIT=200MiB timeout 90 /app/aimd-test 2>&1'

---
OUTPUT:

01:12:46.530301 === Pipeline AIMD Governor Test ===
01:12:46.530351 Creating engine with AIMD governor and RED dropper...
01:12:46.530417 Subscribed to error bus (ID: A0)
01:12:46.530426
Initial state:
01:12:46.530428   Memory Limit: 200.0 MB (source: GOMEMLIMIT)
01:12:46.530439   AIMD State: NORMAL
01:12:46.530441   AIMD Scale: 1.00 (100%)
01:12:46.530442   RED Min Threshold: 60%
01:12:46.530443   RED Max Drop Prob: 30%
01:12:46.530447
Starting memory stress test...
01:12:46.530448 Allocating memory in 10MB chunks every 2 seconds...
01:12:46.530448 Watch for governor state changes as memory pressure increases!


[CONTROL EVENT] 01:12:46.530
  Severity:    INFO
  Code:        HEALTH_CHECK
  Message:     Control loop started
  Context:
    poll_interval: 50ms
    cooldown: 8333h20m0s

01:12:48.552644 Chunk 1 allocated (10.0 MB total) - Heap: 10.3 MB / 200.0 MB (5.2%) - Governor: NORMAL @ 100%
01:12:50.560522 Chunk 2 allocated (20.0 MB total) - Heap: 20.3 MB / 200.0 MB (10.1%) - Governor: NORMAL @ 100%
01:12:52.546785 Chunk 3 allocated (30.0 MB total) - Heap: 30.3 MB / 200.0 MB (15.1%) - Governor: NORMAL @ 100%
01:12:54.552605 Chunk 4 allocated (40.0 MB total) - Heap: 40.3 MB / 200.0 MB (20.1%) - Governor: NORMAL @ 100%
01:12:56.554581 Chunk 5 allocated (50.0 MB total) - Heap: 50.3 MB / 200.0 MB (25.1%) - Governor: NORMAL @ 100%
01:12:58.546334 Chunk 6 allocated (60.0 MB total) - Heap: 60.3 MB / 200.0 MB (30.1%) - Governor: NORMAL @ 100%
01:13:00.548083 Chunk 7 allocated (70.0 MB total) - Heap: 70.3 MB / 200.0 MB (35.1%) - Governor: NORMAL @ 100%
01:13:02.561806 Chunk 8 allocated (80.0 MB total) - Heap: 80.3 MB / 200.0 MB (40.1%) - Governor: NORMAL @ 100%
01:13:04.549968 Chunk 9 allocated (90.0 MB total) - Heap: 90.3 MB / 200.0 MB (45.1%) - Governor: NORMAL @ 100%
01:13:06.547019 Chunk 10 allocated (100.0 MB total) - Heap: 100.3 MB / 200.0 MB (50.1%) - Governor: NORMAL @ 100%
01:13:08.548014 Chunk 11 allocated (110.0 MB total) - Heap: 110.3 MB / 200.0 MB (55.1%) - Governor: NORMAL @ 100%
01:13:10.556414 Chunk 12 allocated (120.0 MB total) - Heap: 120.3 MB / 200.0 MB (60.1%) - Governor: NORMAL @ 100%
01:13:12.549725 Chunk 13 allocated (130.0 MB total) - Heap: 130.3 MB / 200.0 MB (65.1%) - Governor: NORMAL @ 100%

[CONTROL EVENT] 01:13:14.532
  Severity:    WARNING
  Code:        DEGRADED_MODE
  Message:     Governor entered degraded mode - reducing scale
  Context:
    state: DEGRADED
    scale: 0.50
    pressure: 70.1%


[CONTROL EVENT] 01:13:14.532
  Severity:    INFO
  Code:        WORKER_SCALE_DOWN
  Message:     Governor scale decreased to 50%
  Context:
    scale: 0.50
    change: -0.50
    pressure: 70.1%

01:13:14.552216 Chunk 14 allocated (140.0 MB total) - Heap: 140.3 MB / 200.0 MB (70.1%) - Governor: DEGRADED @ 50%

┌─ GOVERNOR STATUS ─────────────────────────────┐
│ State: DEGRADED    Scale: 0.50 (50%)          │
│ State changed: NORMAL → DEGRADED              │
└───────────────────────────────────────────────┘

01:13:16.552308 Chunk 15 allocated (150.0 MB total) - Heap: 150.3 MB / 200.0 MB (75.1%) - Governor: DEGRADED @ 50%
01:13:18.554828 Chunk 16 allocated (160.0 MB total) - Heap: 160.3 MB / 200.0 MB (80.1%) - Governor: DEGRADED @ 50%
01:13:20.554771 Chunk 17 allocated (170.0 MB total) - Heap: 170.3 MB / 200.0 MB (85.1%) - Governor: DEGRADED @ 50%
01:13:22.564091 Chunk 18 allocated (180.0 MB total) - Heap: 180.3 MB / 200.0 MB (90.1%) - Governor: DEGRADED @ 50%

[CONTROL EVENT] 01:13:22.588
  Severity:    INFO
  Code:        WORKER_SCALE_DOWN
  Message:     Governor scale decreased to 25%
  Context:
    change: -0.25
    pressure: 90.1%
    scale: 0.25


[CONTROL EVENT] 01:13:22.638
  Severity:    INFO
  Code:        WORKER_SCALE_DOWN
  Message:     Governor scale decreased to 12%
  Context:
    scale: 0.12
    change: -0.12
    pressure: 90.1%

01:13:24.562007 Chunk 19 allocated (190.0 MB total) - Heap: 190.3 MB / 200.0 MB (95.1%) - Governor: DEGRADED @ 10%

┌─ GOVERNOR STATUS ─────────────────────────────┐
│ State: DEGRADED    Scale: 0.10 (10%)          │
└───────────────────────────────────────────────┘

---
THOUGHTS:

What the trace says
	•	Detection works: you entered DEGRADED at ~70% as designed. ✅
	•	But the cooldown is bonkers: cooldown: 8333h20m0s → that’s ~0.95 years. That’s an overflow/units bug. 🐛
	•	Too many scale cuts in one window: at 90.1% you dropped 50% → 25% → 12% within ~100 ms. That’s not AIMD; that’s panic-saw. You need “one change per control tick” and a floor. 🪚
	•	RED wiring is present, but from the logs the drops are not reported; either RED isn’t hitting yet (no queue pressure) or it isn’t tied to actual queue fill. RED should be per-queue, not global heap.
	•	Recovery path not shown: you never hit MemRelieved (low-water≲55%) because the stressor kept allocating. That’s fine for this run, but add a relief phase test to verify ramp-up behavior.

Tight fixes (minimal code moves)

1) Cooldown bug: fix units + sane default

Make cooldown a time.Duration parse or constant—not “seconds as int” multiplied later.

type GovernorCfg struct {
    PollInterval time.Duration // 50 * time.Millisecond
    Cooldown     time.Duration // 30 * time.Second
    HighFrac     float64       // 0.70
    LowFrac      float64       // 0.55
    MinScale     float64       // 0.20
    AdditiveUp   float64       // +0.05 per tick after relief
    MultDown     float64       // ×0.50 on pressure
}

If you load from env, parse with time.ParseDuration("30s"). Log both values at startup to catch nonsense.

2) True AIMD: one action per tick, clamp, and hysteresis

You’re polling every 50 ms. That’s fine for sensing, but only change scale once per N ticks (e.g., 20 ticks = 1 s), and never more than one multiplicative drop in a single degraded episode.

// inside governor loop
if now.Sub(lastAction) >= cfg.Cooldown {
    switch state {
    case Normal:
        if heapFrac >= cfg.HighFrac {
            scale = max(scale*cfg.MultDown, cfg.MinScale) // e.g., ×0.5 floor 0.20
            state = Degraded
            lastAction = now
            publish(AdapterThrottl, scale)
        }
    case Degraded:
        if heapFrac <= cfg.LowFrac {
            state = Recovering
            lastAction = now
        } else if severe(heapFrac) && scale > cfg.MinScale {
            // allow one more drop per cooldown window, not back-to-back within 100ms
            scale = max(scale*cfg.MultDown, cfg.MinScale)
            lastAction = now
            publish(AdapterThrottl, scale)
        }
    case Recovering:
        // additive increase with cap per action
        scale = min(scale+cfg.AdditiveUp, 1.0)
        lastAction = now
        if scale >= 1.0 {
            state = Normal
        }
        publish(AdapterUnth, scale)
    }
}

Define severe(h) as, say, h >= 0.90 for ≥2–3 polls to avoid twitchiness.

3) Per-queue RED (not heap-driven)

RED should look at queue fill, not heap. Wire it into your queue Offer:

func (q *Queue) shouldRedDrop() bool {
    fill := float64(len(q.ch)) / float64(cap(q.ch))
    switch {
    case fill < 0.60: return false
    case fill >= 1.00: return true
    default:
        p := ((fill - 0.60) / 0.40) * 0.30 // up to 30%
        return rand.Float64() < p
    }
}

When RED drops, increment a per-adapter drops_red_total and emit a sampled DROP_SLOW event once per second with the latest counters.

4) Pace adapters from scale with a floor

At 10–12% scale you risk starving yourself. Clamp at MinScale (e.g., 20%) and make pacing proportional, not binary:

// ingress pacing
sleep := time.Duration((1.0-scale) * 300_000) * time.Microsecond // 0..300ms
if sleep > 0 { time.Sleep(sleep) }

5) Prove relief behavior (second phase test)

After you enter DEGRADED and log one throttle event, stop allocating and watch heap fall below LowFrac. Expect:
	•	mem.relief event,
	•	Additive climb: 40% → 45% → 50% … → 100% (one step per cooldown),
	•	State transitions: DEGRADED → RECOVERING → NORMAL.

Add explicit pass/fail checks:
	•	pressure→throttle in ≤500 ms,
	•	relief doesn’t fire until heap ≤ low for ≥2 polls,
	•	scale never drops more than once per cooldown window.

6) Housekeeping (cheap wins)
	•	Log thresholds once: high=140MB low=110MB poll=50ms cooldown=30s.
	•	Include prev_scale in your WORKER_SCALE_DOWN/UP events so diffs are obvious.
	•	Add counters: events_dropped_total{kind}, red_drops_total{adapter}, throttle_seconds_total.

Sensible knobs to start
	•	PollInterval=50ms, Cooldown=30s
	•	HighFrac=0.70, LowFrac=0.55
	•	MultDown=0.5, AdditiveUp=0.05, MinScale=0.20
	•	RED: start at 60% fill, max 30% drop prob at 100% fill.

Why the weirdness happened

Your governor is reading pressure correctly but applying multiple multiplicative decrements with no cooldown, and that year-long cooldown string means your rate limiter wasn’t actually active. Fix those two and the graph will go from “chainsaw” to “smooth ramp.”

