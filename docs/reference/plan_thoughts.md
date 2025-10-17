
Error signaling: tighten the plumbing
	1.	Event bus must be non-blocking, bounded, and lossy (by design).
Your note says “unbuffered – never blocks,” but unbuffered channels block by definition. Use a small bounded queue per subscriber and drop when full. Emit a sampled counter for dropped diagnostics so you notice storms without causing them.

type Sub struct{ ch chan Event }
type Bus struct{ subs atomic.Pointer[[]*Sub] }

func (b *Bus) Publish(e Event) {
    subs := b.subs.Load()
    for i := range *subs {
        select {
        case (*subs)[i].ch <- e:
        default:
            // drop diagnostic to protect the data path
            // optionally increment a local atomic counter
        }
    }
}

	2.	Severity taxonomy
Keep it minimal and map-able to log levels/alerts: debug/info/warn/error/crit. Add a Signal field separate from Severity for “control intents” like throttle, shed, breaker_open, etc. That lets you route by kind without string-matching messages.
	3.	Stable, short error codes
Use terse codes that survive refactors: MEM_PRESSURE, BUF_SAT, PUBLISH_BLOCK, DROP_SLOW, ADAPTER_FAIL, EMITTER_FAIL, BREAKER_OPEN. Put the human explanation in Message, details in Context.
	4.	Flight recorder (last-N state)
Wire a tiny ring buffer that snapshots: heap bytes, GOMEMLIMIT, queue depths, p50/p99 latencies, governor scale. On crit or panic, dump ring + pprof heap/goroutine. Keep it single-writer so it’s cheap.

Memory sensing: tighter signals, fewer lies
	5.	Prefer runtime/metrics (+ GOMEMLIMIT) for fast polling.
MemStats is heavier. Resolve your soft ceiling as:

	•	limit = min(GOMEMLIMIT, cgroupLimitIfAny, explicitConfig) where “unlimited” values are ignored.

	6.	cgroup v2 edge cases
memory.max can be "max" (meaning unlimited). Same for CPU. Parse robustly:

func cgroupV2MemLimit() (uint64, bool) {
    b, err := os.ReadFile("/sys/fs/cgroup/memory.max")
    if err != nil { return 0, false }
    s := strings.TrimSpace(string(b))
    if s == "max" { return 0, false }
    v, err := strconv.ParseUint(s, 10, 64)
    if err != nil || v == 0 || v > 1<<60 { return 0, false }
    return v, true
}

func cgroupV2CPUQuota() (float64, bool) {
    // cpu.max: "<quota> <period>" or "max <period>"
    b, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
    if err != nil { return 0, false }
    f := strings.Fields(string(b))
    if len(f) != 2 || f[0] == "max" { return 0, false }
    quota, _ := strconv.ParseFloat(f[0], 64)
    period, _ := strconv.ParseFloat(f[1], 64)
    if quota <= 0 || period <= 0 { return 0, false }
    return quota / period, true // CPUs as float
}

	7.	PSI early warning
If you’re on Linux, poll /proc/pressure/memory and treat some avg10 rising > ~0.2 for >2s as a “pre-oom” alarm. That’s your cue to scale down and shed before the reaper.

Dynamic allocation: keep it smooth, not twitchy
	8.	Hysteresis + cool-downs
You’ve got placeholders—make them explicit: control loop every 2–5s, minimum time between actions 30s. Only one change per loop: either resize buffers or adjust workers, not both.
	9.	Buffer auto-resize guardrails

	•	Never exceed a fixed memory budget (e.g., 40–50% of limit). Track it globally across all pools/queues.
	•	Growth: only when saturation > target for consecutive windows (e.g., 3 ticks).
	•	Shrink: when saturation < 0.3 for N windows and no recent drops; reduce by a factor (e.g., ×0.7), not a huge jump.

	10.	Drop-slow > infinite buffers
Keep queues modest (8–1024 frames depending on frame size). Prefer probabilistic early drop (RED-style) starting around 60% full so you don’t hit a hard cliff.

func redDropProb(fill float64) float64 {
    // start dropping around 0.6, reach 30% at 1.0
    if fill <= 0.6 { return 0 }
    return (fill - 0.6) / 0.4 * 0.3
}

	11.	Autoscaling workers: aim for lag, not throughput
Your util calc is fine; make the decision metric “queue-implied lag”:
lag_ms ≈ queue_len * p50_us / 1000.
Scale up if lag_ms > 2×target for K ticks and memory pressure < 0.75. Scale down if lag_ms < 0.5×target for K ticks. Keep a warm spare to avoid cold start bursts.
	12.	Governor: AIMD pacing
When MEM_PRESSURE enters, set scale to e.g. 40% and climb multiplicatively (×1.05 per tick) back to 100% after MEM_RELIEF, with a cap per tick. This converges gently.

Adapters/emitters: correctness under stress
	13.	Circuit breaker per peer/route
Trip on error-rate window (say >20% over 50 ops) AND p95 RTT > threshold; half-open probe window; publish state changes on the bus. Don’t forget to stop pulling from the toxic edge while open.
	14.	Coalesce > drop
For keyed streams, keep only the newest frame in the queue for that key when pressure rises. You preserve freshness while cutting work.
	15.	JSONv2 discipline
Never string(b); operate on []byte. For logs, sample and truncate. For huge payloads, prefer streaming Decoder.ReadValue into pooled buffers, or split into framed chunks with reassembly.

Crash forensics: always have receipts
	16.	Top-level guard
Wrap goroutine roots with a recover that dumps flight recorder + minimal pprof. Set a once-per-minute rate limit so you don’t storm disk during a flappy incident.
	17.	Failpoints + Chaos
Keep your 50 MB “wrecking ball” as a canary, but add deterministic chaos: drop/dup/reorder/delay/truncate with a seed. Wire failpoints before queue offer, after bufpool get, before publish, and in network writes to inject EAGAIN/ENOBUFS.

Small code nits from the docs
	•	The ErrorSeverity constants show a label ErrorSeverity used twice in your snippet—make sure the fifth enum value isn’t accidentally named the same as the type.
	•	Where you read cgroup files, handle the "max" string and absurd sentinel values (e.g., 2^63−1) gracefully.
	•	Your “unbuffered bus that never blocks” comment is inverted—fix per item #1.

Sensible defaults (so you can just ship)
	•	Governor thresholds: enter at 0.70 of limit, exit at 0.55, poll every 50 ms.
	•	Control loop: every 3 s, cooldown 30 s, one action per loop.
	•	Queue sizes: start 128 frames; min 8, max 1024; RED kick-in at 60% fill.
	•	Frame size: 128 KiB (if you frame), or stream with backpressure.
	•	Worker scaling: target lag 10 ms, min workers 2, max 8.
	•	Buffer budget: ≤50% of memory limit across all dynamic structures.

Quick win checklist
	•	Swap error bus to bounded, lossy fan-out; add sampled events_dropped_total.
	•	Implement runtime/metrics + GOMEMLIMIT + robust cgroup parsing.
	•	Add PSI poller and promote MEM_PRESSURE earlier.
	•	Add flight recorder + panic dump.
	•	Implement RED and AIMD governor scaling.
	•	Add coalescing drop policy for keyed streams.
	•	Wire a chaos link + two or three failpoints.

