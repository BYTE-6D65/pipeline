package testdata

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/engine"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// TestScenario represents a test data scenario
type TestScenario string

const (
	ScenarioNormal      TestScenario = "normal"
	ScenarioMassive     TestScenario = "massive"
	ScenarioAdversarial TestScenario = "adversarial"
)

// PerformanceMetrics contains comprehensive performance results
type PerformanceMetrics struct {
	Scenario   TestScenario
	EventCount int
	Duration   time.Duration

	// Latency metrics
	LatencyMin    time.Duration
	LatencyMax    time.Duration
	LatencyMean   time.Duration
	LatencyMedian time.Duration
	LatencyP90    time.Duration
	LatencyP95    time.Duration
	LatencyP99    time.Duration
	LatencyStdDev time.Duration
	Jitter        time.Duration

	// Throughput
	EventsPerSec float64
	MBPerSec     float64

	// Memory
	AllocatedMB float64

	// GC metrics
	GCCount    uint32
	GCPauseAvg time.Duration
	GCPauseMax time.Duration
}

// ProgressCallback is called periodically during test execution
type ProgressCallback func(eventCount, totalEvents int, currentRate float64, elapsed time.Duration)

// RunTestScenario executes a test scenario and returns metrics
func RunTestScenario(ctx context.Context, scenario TestScenario, eventCount int) (*PerformanceMetrics, error) {
	return RunTestScenarioWithProgress(ctx, scenario, eventCount, nil)
}

// RunTestScenarioWithProgress executes a test scenario with progress callbacks
func RunTestScenarioWithProgress(ctx context.Context, scenario TestScenario, eventCount int, progressCb ProgressCallback) (*PerformanceMetrics, error) {
	eng := engine.New()
	defer eng.Shutdown(ctx)

	// Force GC before test
	runtime.GC()

	var gcStatsBefore runtime.MemStats
	runtime.ReadMemStats(&gcStatsBefore)

	latencies := make([]time.Duration, 0, eventCount)
	startTime := time.Now()
	lastProgressUpdate := startTime

	// Determine throttle delay based on scenario to make it visible
	var throttleDelay time.Duration
	switch scenario {
	case ScenarioNormal:
		throttleDelay = 2 * time.Millisecond // ~500 events/sec, takes ~2 seconds for 1000
	case ScenarioMassive:
		throttleDelay = 20 * time.Millisecond // ~50 events/sec, takes ~2 seconds for 100
	case ScenarioAdversarial:
		throttleDelay = 4 * time.Millisecond // ~250 events/sec, takes ~2 seconds for 500
	}

	// Generate and publish test events based on scenario
	for i := 0; i < eventCount; i++ {
		evt, payloadSize := generateEvent(scenario, i)

		pubStart := time.Now()
		if err := eng.InternalBus().Publish(ctx, evt); err != nil {
			return nil, fmt.Errorf("publish failed: %w", err)
		}
		latencies = append(latencies, time.Since(pubStart))

		// Add throttle delay to make progress visible
		if throttleDelay > 0 {
			time.Sleep(throttleDelay)
		}

		// Send progress update every 50ms
		if progressCb != nil && time.Since(lastProgressUpdate) >= 50*time.Millisecond {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			progressCb(i+1, eventCount, rate, elapsed)
			lastProgressUpdate = time.Now()
		}

		_ = payloadSize
	}

	// Final progress update
	if progressCb != nil {
		elapsed := time.Since(startTime)
		rate := float64(eventCount) / elapsed.Seconds()
		progressCb(eventCount, eventCount, rate, elapsed)
	}

	testDuration := time.Since(startTime)

	var gcStatsAfter runtime.MemStats
	runtime.ReadMemStats(&gcStatsAfter)

	return calculateMetrics(scenario, latencies, testDuration, &gcStatsBefore, &gcStatsAfter), nil
}

func generateEvent(scenario TestScenario, index int) (event.Event, int) {
	var payload interface{}
	var payloadSize int

	switch scenario {
	case ScenarioNormal:
		payload = map[string]interface{}{
			"index": index,
			"key":   "A",
			"code":  30,
		}
		payloadSize = 100

	case ScenarioMassive:
		// 1MB payload
		data := make([]byte, 1024*1024)
		rand.Read(data)
		payload = map[string]interface{}{
			"index": index,
			"data":  data,
		}
		payloadSize = 1024 * 1024

	case ScenarioAdversarial:
		payload = generateDeeplyNestedJSON(5, 3)
		payloadSize = 50 * 1024
	}

	data, _ := json.Marshal(payload)

	return event.Event{
		ID:        fmt.Sprintf("test-%d", index),
		Type:      "test.event",
		Source:    "testdata-generator",
		Timestamp: time.Now(),
		Data:      data,
	}, payloadSize
}

func generateDeeplyNestedJSON(depth int, width int) interface{} {
	if depth == 0 {
		return "🔥💀⚡️\x00\xFF\n\r\t\"\\混乱🎭"
	}
	result := make(map[string]interface{})
	for i := 0; i < width; i++ {
		key := fmt.Sprintf("level_%d_key_%d_🎯", depth, i)
		result[key] = generateDeeplyNestedJSON(depth-1, width)
	}
	return result
}

func calculateMetrics(scenario TestScenario, latencies []time.Duration, testDuration time.Duration, before, after *runtime.MemStats) *PerformanceMetrics {
	// Sort for percentiles
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	min := sorted[0]
	max := sorted[len(sorted)-1]
	median := sorted[len(sorted)*50/100]
	p90 := sorted[len(sorted)*90/100]
	p95 := sorted[len(sorted)*95/100]
	p99 := sorted[len(sorted)*99/100]

	// Mean
	var total time.Duration
	for _, lat := range sorted {
		total += lat
	}
	mean := total / time.Duration(len(sorted))

	// Standard deviation
	var sumSquaredDiff float64
	for _, lat := range sorted {
		diff := float64(lat - mean)
		sumSquaredDiff += diff * diff
	}
	variance := sumSquaredDiff / float64(len(sorted))
	stdDev := time.Duration(math.Sqrt(variance))

	// Jitter
	var totalJitter time.Duration
	for i := 1; i < len(latencies); i++ {
		diff := latencies[i] - latencies[i-1]
		if diff < 0 {
			diff = -diff
		}
		totalJitter += diff
	}
	avgJitter := totalJitter / time.Duration(len(latencies)-1)

	// GC metrics
	gcCount := after.NumGC - before.NumGC
	var gcPauseTotal, gcPauseMax uint64

	for i := before.NumGC; i < after.NumGC; i++ {
		pause := after.PauseNs[i%256]
		gcPauseTotal += pause
		if pause > gcPauseMax {
			gcPauseMax = pause
		}
	}

	var gcPauseAvg time.Duration
	if gcCount > 0 {
		gcPauseAvg = time.Duration(gcPauseTotal / uint64(gcCount))
	}

	// Throughput
	eventsPerSec := float64(len(latencies)) / testDuration.Seconds()

	// Memory (approximate)
	allocatedMB := float64(after.TotalAlloc-before.TotalAlloc) / (1024 * 1024)

	return &PerformanceMetrics{
		Scenario:      scenario,
		EventCount:    len(latencies),
		Duration:      testDuration,
		LatencyMin:    min,
		LatencyMax:    max,
		LatencyMean:   mean,
		LatencyMedian: median,
		LatencyP90:    p90,
		LatencyP95:    p95,
		LatencyP99:    p99,
		LatencyStdDev: stdDev,
		Jitter:        avgJitter,
		EventsPerSec:  eventsPerSec,
		AllocatedMB:   allocatedMB,
		GCCount:       gcCount,
		GCPauseAvg:    time.Duration(gcPauseAvg),
		GCPauseMax:    time.Duration(gcPauseMax),
	}
}

// FormatMetrics returns a human-readable string of metrics
func FormatMetrics(m *PerformanceMetrics) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 Performance Metrics - %s scenario\n\n", m.Scenario))
	sb.WriteString(fmt.Sprintf("Events:     %d\n", m.EventCount))
	sb.WriteString(fmt.Sprintf("Duration:   %v\n\n", m.Duration.Round(time.Millisecond)))

	sb.WriteString("Latency:\n")
	sb.WriteString(fmt.Sprintf("  Min:      %v\n", m.LatencyMin))
	sb.WriteString(fmt.Sprintf("  Max:      %v\n", m.LatencyMax))
	sb.WriteString(fmt.Sprintf("  Mean:     %v\n", m.LatencyMean))
	sb.WriteString(fmt.Sprintf("  Median:   %v\n", m.LatencyMedian))
	sb.WriteString(fmt.Sprintf("  P90:      %v\n", m.LatencyP90))
	sb.WriteString(fmt.Sprintf("  P95:      %v\n", m.LatencyP95))
	sb.WriteString(fmt.Sprintf("  P99:      %v\n", m.LatencyP99))
	sb.WriteString(fmt.Sprintf("  StdDev:   %v\n", m.LatencyStdDev))
	sb.WriteString(fmt.Sprintf("  Jitter:   %v\n\n", m.Jitter))

	sb.WriteString("Throughput:\n")
	sb.WriteString(fmt.Sprintf("  Events/s: %.2f\n", m.EventsPerSec))
	if m.MBPerSec > 0 {
		sb.WriteString(fmt.Sprintf("  MB/s:     %.2f\n", m.MBPerSec))
	}
	sb.WriteString("\n")

	sb.WriteString("Memory:\n")
	sb.WriteString(fmt.Sprintf("  Allocated: %.2f MB\n\n", m.AllocatedMB))

	sb.WriteString("GC:\n")
	sb.WriteString(fmt.Sprintf("  Collections: %d\n", m.GCCount))
	sb.WriteString(fmt.Sprintf("  Avg Pause:   %v\n", m.GCPauseAvg))
	sb.WriteString(fmt.Sprintf("  Max Pause:   %v\n", m.GCPauseMax))

	return sb.String()
}
