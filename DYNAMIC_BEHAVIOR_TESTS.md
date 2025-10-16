# Dynamic Behavior Test Suite Design

**Purpose**: Comprehensive testing of Phase 2 graceful degradation mechanisms (AIMD + RED).

**Inspired by**: Network adapter test methodology - automated, repeatable, pass/fail criteria.

---

## Test Categories

### 1. Core AIMD Cycle Tests

#### Test 1.1: Normal → Degraded Transition
**Goal**: Verify governor enters degraded mode at 70% memory pressure.

**Setup**:
- GOMEMLIMIT=200MB
- Start in NORMAL state

**Procedure**:
1. Allocate memory in 10MB chunks
2. Monitor for DEGRADED transition at 70% (140MB)
3. Verify scale drops to 50% (×0.5 multiplicative decrease)

**Pass Criteria**:
- ✅ DEGRADED state reached within 500ms of crossing 70%
- ✅ Scale exactly 0.50
- ✅ DEGRADED_MODE event emitted
- ✅ WORKER_SCALE_DOWN event emitted

**Fail Cases**:
- ❌ Transition takes > 500ms
- ❌ Scale not 0.50 (wrong multiplicative factor)
- ❌ No control events emitted

---

#### Test 1.2: Degraded → Recovering Transition
**Goal**: Verify governor exits degraded mode when pressure drops below 55%.

**Setup**:
- Start in DEGRADED state (from Test 1.1)

**Procedure**:
1. Release all allocated memory
2. Force GC
3. Wait for pressure < 55% (110MB)
4. Monitor for RECOVERING transition

**Pass Criteria**:
- ✅ RECOVERING state reached within 2 poll intervals (100ms)
- ✅ Scale remains at last degraded value (no change on transition)
- ✅ State change event emitted

**Fail Cases**:
- ❌ Stays in DEGRADED during cooldown (bug we just fixed!)
- ❌ Scale changes on state transition
- ❌ Takes > 1 second to transition

---

#### Test 1.3: Recovering → Normal Transition
**Goal**: Verify additive increase and return to NORMAL.

**Setup**:
- Start in RECOVERING state @ 50% scale

**Procedure**:
1. Keep memory pressure < 55%
2. Monitor scale increases every 30s cooldown
3. Wait for scale=1.0 and NORMAL state

**Pass Criteria**:
- ✅ Scale increases by exactly +0.05 per cooldown window
- ✅ Scale increases occur every ~30 seconds (±1s tolerance)
- ✅ NORMAL state reached when scale=1.0
- ✅ WORKER_SCALE_UP events emitted for each +0.05 increase

**Fail Cases**:
- ❌ Scale increases too fast (cooldown violation)
- ❌ Scale increases by wrong amount (not +0.05)
- ❌ Scale exceeds 1.0
- ❌ Stays in RECOVERING when scale=1.0

**Expected Duration**: ~5 minutes (10 steps × 30s cooldown)

---

### 2. Cooldown Enforcement Tests

#### Test 2.1: No Panic-Saw Under Sustained Pressure
**Goal**: Verify only ONE scale decrease per cooldown window.

**Setup**:
- GOMEMLIMIT=200MB
- Start in NORMAL state

**Procedure**:
1. Allocate to 70% → trigger DEGRADED @ 50%
2. Continue allocating to 90% (critical threshold)
3. Monitor for additional scale decreases

**Pass Criteria**:
- ✅ First decrease: 100% → 50% at 70% pressure
- ✅ Second decrease: 50% → 25% at 90% pressure (after 30s cooldown)
- ✅ NO decreases between 0-30 seconds after first decrease
- ✅ Maximum 2 decreases in first 60 seconds

**Fail Cases**:
- ❌ Multiple decreases within 30s window (panic-saw bug!)
- ❌ Scale drops below 20% (MinScale violation)

---

#### Test 2.2: No Panic-Saw During Recovery
**Goal**: Verify additive increases respect cooldown.

**Setup**:
- Start in RECOVERING state @ 20% scale

**Procedure**:
1. Keep memory pressure < 55%
2. Monitor scale increases for 2 minutes
3. Count increases and measure intervals

**Pass Criteria**:
- ✅ Exactly 4 increases in 2 minutes (every 30s)
- ✅ Each increase separated by 28-32s
- ✅ Each increase exactly +0.05

**Fail Cases**:
- ❌ More than 4 increases (cooldown too short)
- ❌ Irregular intervals (cooldown not enforced)

---

### 3. Hysteresis and Oscillation Tests

#### Test 3.1: Hysteresis Gap Prevents Oscillation
**Goal**: Verify 15% gap (70% enter, 55% exit) prevents rapid state changes.

**Setup**:
- GOMEMLIMIT=200MB
- Start in NORMAL state

**Procedure**:
1. Allocate to 70% → DEGRADED
2. Release to 60% (between exit and enter thresholds)
3. Hold at 60% for 60 seconds
4. Verify state remains DEGRADED

**Pass Criteria**:
- ✅ State=DEGRADED for full 60 seconds at 60% pressure
- ✅ No transitions to RECOVERING until < 55%
- ✅ Scale stays constant (no changes)

**Fail Cases**:
- ❌ Oscillates between DEGRADED and RECOVERING
- ❌ Transitions at wrong pressure level

---

#### Test 3.2: Recovery Interruption
**Goal**: Verify recovery stops if pressure returns.

**Setup**:
- Start in RECOVERING @ 60% scale

**Procedure**:
1. Allow one additive increase (60% → 65%)
2. Allocate memory back to 72% pressure
3. Verify transition back to DEGRADED

**Pass Criteria**:
- ✅ Transition RECOVERING → DEGRADED when pressure ≥ 70%
- ✅ Scale decreases: 65% → 32.5% (multiplicative decrease)
- ✅ Recovery progress is lost (correct behavior)

**Fail Cases**:
- ❌ Stays in RECOVERING above enter threshold
- ❌ Scale doesn't decrease

---

### 4. MinScale Floor Tests

#### Test 4.1: MinScale Floor at 20%
**Goal**: Verify scale never drops below 20%.

**Setup**:
- GOMEMLIMIT=200MB
- Start in NORMAL state

**Procedure**:
1. Allocate to 70% → scale=50%
2. Allocate to 90% → scale=25%
3. Force another pressure event (simulate sustained 95%+)
4. Attempt another decrease

**Pass Criteria**:
- ✅ First decrease: 50%
- ✅ Second decrease: 25%
- ✅ Third decrease: 20% (clamped, not 12.5%)
- ✅ Fourth decrease: 20% (stays at floor)

**Fail Cases**:
- ❌ Scale drops below 20%
- ❌ Scale goes to 0% (complete starvation)

---

### 5. Full Cycle Stress Tests

#### Test 5.1: Multiple Degradation/Recovery Cycles
**Goal**: Verify system handles repeated pressure cycles.

**Setup**:
- GOMEMLIMIT=200MB
- Run for 15 minutes

**Procedure**:
1. Cycle 1: Allocate to 75% → DEGRADED → release → RECOVERING → NORMAL
2. Cycle 2: Allocate to 80% → DEGRADED → release → RECOVERING → NORMAL
3. Cycle 3: Allocate to 90% → DEGRADED (multiple decreases) → release → RECOVERING → NORMAL
4. Repeat 3 full cycles

**Pass Criteria**:
- ✅ All 3 cycles complete successfully
- ✅ Each cycle follows correct state machine
- ✅ Governor returns to NORMAL @ 100% after each cycle
- ✅ No state machine corruption

**Metrics Tracked**:
- Time to degrade (NORMAL → DEGRADED)
- Time to recover (DEGRADED → NORMAL)
- Number of scale changes per cycle
- Memory high-water mark

**Fail Cases**:
- ❌ State machine gets stuck
- ❌ Scale doesn't return to 100%
- ❌ Crash or panic

---

#### Test 5.2: Sustained Pressure Stability
**Goal**: Verify system is stable under prolonged degraded state.

**Setup**:
- GOMEMLIMIT=200MB
- Maintain 85% memory pressure

**Procedure**:
1. Allocate to trigger DEGRADED @ 50%
2. Maintain pressure at 85% for 10 minutes
3. Monitor for unexpected scale changes

**Pass Criteria**:
- ✅ State=DEGRADED for full duration
- ✅ Scale stabilizes at 25% (50% → 25% after cooldown)
- ✅ No oscillations or thrashing
- ✅ System remains responsive

**Fail Cases**:
- ❌ Continuous scale decreases (runaway)
- ❌ Oscillation between states
- ❌ OOM kill (should degrade gracefully)

---

### 6. RED Dropper Tests (Future - Phase 3)

#### Test 6.1: RED Probability Ramp
**Goal**: Verify probabilistic dropping at 60-100% buffer fill.

**Setup**:
- Queue with 100 slots
- RED min=0.6, max=1.0, maxProb=0.3

**Procedure**:
1. Fill queue to 60% → drop probability = 0%
2. Fill queue to 80% → drop probability = 15%
3. Fill queue to 100% → drop probability = 30%
4. Run 10,000 trials at each level

**Pass Criteria**:
- ✅ At 60%: 0% drops (within statistical margin)
- ✅ At 80%: 15% drops (±2%)
- ✅ At 100%: 30% drops (±2%)
- ✅ Linear ramp behavior

**Fail Cases**:
- ❌ Drops below 60% threshold
- ❌ Drops > 30% at any level
- ❌ Non-linear ramp

---

#### Test 6.2: RED + AIMD Integration
**Goal**: Verify RED and AIMD work together.

**Setup**:
- GOMEMLIMIT=200MB
- Per-queue RED enabled

**Procedure**:
1. Trigger memory pressure → AIMD degrades to 50%
2. Monitor queue fill levels
3. Verify RED starts dropping when queues > 60% full

**Pass Criteria**:
- ✅ AIMD reduces publish rate (scale=50% = slower input)
- ✅ Queues drain faster due to slower input
- ✅ RED drops kick in if queues still saturate
- ✅ System doesn't OOM (double protection)

**Metrics**:
- Events dropped by RED
- Events shed by AIMD rate reduction
- Queue fill distribution

---

## Test Harness Architecture

### Structure

```
cmd/
  behavior-test/
    main.go              # Test runner CLI
    tests/
      aimd_cycle_test.go      # Test 1.x
      cooldown_test.go        # Test 2.x
      hysteresis_test.go      # Test 3.x
      minscale_test.go        # Test 4.x
      stress_test.go          # Test 5.x
      red_test.go             # Test 6.x
    framework/
      test_case.go       # Test case interface
      assertions.go      # Pass/fail helpers
      metrics.go         # Performance tracking
      report.go          # Test report generator
```

### Test Case Interface

```go
type TestCase interface {
    Name() string
    Category() string
    Setup() error
    Run() error
    Teardown() error
    Validate() TestResult
}

type TestResult struct {
    Passed     bool
    Duration   time.Duration
    Metrics    map[string]interface{}
    Assertions []Assertion
    Errors     []error
}

type Assertion struct {
    Name     string
    Expected interface{}
    Actual   interface{}
    Passed   bool
    Message  string
}
```

### CLI Interface

```bash
# Run all tests
./behavior-test --all

# Run specific category
./behavior-test --category=aimd_cycle

# Run specific test
./behavior-test --test=1.1

# Run with detailed output
./behavior-test --all --verbose

# Generate report
./behavior-test --all --report=json > results.json
./behavior-test --all --report=markdown > RESULTS.md
```

### Output Format

```
=== Pipeline Dynamic Behavior Test Suite ===

Category: Core AIMD Cycle Tests
  [PASS] 1.1: Normal → Degraded Transition (452ms)
  [PASS] 1.2: Degraded → Recovering Transition (87ms)
  [PASS] 1.3: Recovering → Normal Transition (5m 12s)

Category: Cooldown Enforcement Tests
  [PASS] 2.1: No Panic-Saw Under Sustained Pressure (1m 30s)
  [PASS] 2.2: No Panic-Saw During Recovery (2m 15s)

Category: Hysteresis and Oscillation Tests
  [PASS] 3.1: Hysteresis Gap Prevents Oscillation (1m 5s)
  [PASS] 3.2: Recovery Interruption (45s)

Category: MinScale Floor Tests
  [PASS] 4.1: MinScale Floor at 20% (2m 10s)

Category: Full Cycle Stress Tests
  [PASS] 5.1: Multiple Degradation/Recovery Cycles (15m 23s)
  [PASS] 5.2: Sustained Pressure Stability (10m 5s)

=== Summary ===
Total Tests: 9
Passed: 9
Failed: 0
Duration: 38m 14s

All tests PASSED ✅
```

---

## Success Criteria

Phase 2 is production-ready when:

- ✅ All core AIMD cycle tests pass (1.x)
- ✅ All cooldown enforcement tests pass (2.x)
- ✅ All hysteresis tests pass (3.x)
- ✅ MinScale floor test passes (4.x)
- ✅ All stress tests pass (5.x)
- ✅ No OOM kills in stress tests
- ✅ System recovers gracefully in all scenarios
- ✅ No state machine corruption
- ✅ Cooldown prevents panic-saw in all cases

---

## Future Enhancements (Phase 3)

- [ ] Per-queue RED integration tests (6.x)
- [ ] Dynamic buffer resizing tests
- [ ] Multi-adapter stress tests
- [ ] Chaos engineering (random memory spikes, GC stalls)
- [ ] Performance benchmarks (throughput under pressure)
- [ ] Long-running soak tests (24h+ stability)

---

## Implementation Priority

**Week 3 (Current)**:
1. Test 1.1, 1.2, 1.3 (Core AIMD) - ✅ Done via recovery-test
2. Test 2.1 (Panic-saw prevention)
3. Test 4.1 (MinScale floor)

**Week 4**:
4. Test framework scaffolding
5. Test 3.1, 3.2 (Hysteresis)
6. Test 2.2 (Recovery cooldown)
7. Test 5.1 (Multi-cycle stress)

**Week 5**:
8. Test 5.2 (Sustained pressure)
9. Test report generation
10. CI/CD integration

**Week 6+**:
11. RED tests (requires per-queue integration)
12. Advanced chaos tests
