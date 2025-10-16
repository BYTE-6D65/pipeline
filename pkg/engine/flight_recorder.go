package engine

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"
)

// FlightRecorder maintains a ring buffer of system snapshots for crash analysis.
// Inspired by aircraft "black box" recorders, this captures the last N states
// before a crash to aid in root cause analysis.
type FlightRecorder struct {
	snapshots []Snapshot
	index     int
	size      int
	mu        sync.Mutex // Single-writer lock
}

// Snapshot represents a point-in-time state of the system.
type Snapshot struct {
	Timestamp time.Time

	// Memory
	HeapBytes uint64
	HeapSys   uint64
	MemLimit  uint64

	// Goroutines & GC
	NumGoroutine int
	GCCount      uint32

	// Queue depths (bus:sub -> depth)
	QueueDepths map[string]int

	// Latencies (operation -> p50/p99 in microseconds)
	Latencies map[string]LatencySnapshot

	// Governor state
	GovernorScale float64
	GovernorState string
}

// LatencySnapshot captures latency percentiles.
type LatencySnapshot struct {
	P50 time.Duration
	P99 time.Duration
}

// NewFlightRecorder creates a flight recorder with the given ring buffer size.
func NewFlightRecorder(size int) *FlightRecorder {
	if size <= 0 {
		size = 100 // Default
	}

	return &FlightRecorder{
		snapshots: make([]Snapshot, size),
		size:      size,
	}
}

// Record adds a snapshot to the ring buffer.
// This is fast and lock-protected for single-writer use.
func (fr *FlightRecorder) Record(snap Snapshot) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	fr.snapshots[fr.index] = snap
	fr.index = (fr.index + 1) % fr.size
}

// Dump writes the flight recorder contents to the given writer.
// Includes:
//   - Last N snapshots in chronological order
//   - Heap profile (pprof)
//   - Goroutine profile (pprof)
func (fr *FlightRecorder) Dump(w io.Writer) error {
	fr.mu.Lock()
	// Copy snapshots to avoid holding lock during slow writes
	snapshots := make([]Snapshot, fr.size)
	copy(snapshots, fr.snapshots)
	currentIndex := fr.index
	fr.mu.Unlock()

	fmt.Fprintf(w, "=== Flight Recorder Dump ===\n")
	fmt.Fprintf(w, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "Last %d snapshots:\n\n", fr.size)

	// Dump snapshots in chronological order (oldest to newest)
	count := 0
	start := currentIndex

	for i := 0; i < fr.size; i++ {
		idx := (start + i) % fr.size
		snap := snapshots[idx]

		// Skip uninitialized slots (before buffer fills)
		if snap.Timestamp.IsZero() {
			continue
		}

		count++
		fmt.Fprintf(w, "[%d] %s\n", count, snap.Timestamp.Format("15:04:05.000"))
		fmt.Fprintf(w, "  Memory: %s / %s (%.1f%%) | Goroutines: %d | GC: %d\n",
			FormatBytes(snap.HeapBytes),
			FormatBytes(snap.MemLimit),
			float64(snap.HeapBytes)/float64(snap.MemLimit)*100,
			snap.NumGoroutine,
			snap.GCCount)

		if snap.GovernorScale > 0 {
			fmt.Fprintf(w, "  Governor: %.2fx scale | State: %s\n",
				snap.GovernorScale, snap.GovernorState)
		}

		if len(snap.QueueDepths) > 0 {
			fmt.Fprintf(w, "  Queues:\n")
			for queue, depth := range snap.QueueDepths {
				fmt.Fprintf(w, "    %s: %d\n", queue, depth)
			}
		}

		if len(snap.Latencies) > 0 {
			fmt.Fprintf(w, "  Latencies:\n")
			for op, lat := range snap.Latencies {
				fmt.Fprintf(w, "    %s: p50=%s p99=%s\n",
					op, lat.P50, lat.P99)
			}
		}

		fmt.Fprintln(w)
	}

	if count == 0 {
		fmt.Fprintf(w, "(No snapshots recorded yet)\n\n")
	}

	// Dump heap profile
	fmt.Fprintf(w, "=== Heap Profile ===\n")
	if heap := pprof.Lookup("heap"); heap != nil {
		if err := heap.WriteTo(w, 0); err != nil {
			fmt.Fprintf(w, "Error writing heap profile: %v\n", err)
		}
	} else {
		fmt.Fprintf(w, "Heap profile not available\n")
	}
	fmt.Fprintln(w)

	// Dump goroutine profile
	fmt.Fprintf(w, "=== Goroutine Profile ===\n")
	if goroutine := pprof.Lookup("goroutine"); goroutine != nil {
		if err := goroutine.WriteTo(w, 2); err != nil { // debug=2 for full stacks
			fmt.Fprintf(w, "Error writing goroutine profile: %v\n", err)
		}
	} else {
		fmt.Fprintf(w, "Goroutine profile not available\n")
	}

	return nil
}

// CaptureSnapshot captures the current system state as a snapshot.
func (fr *FlightRecorder) CaptureSnapshot(memLimit uint64, queueDepths map[string]int) Snapshot {
	stats := ReadMemoryStatsFast(memLimit)

	snap := Snapshot{
		Timestamp:     time.Now(),
		HeapBytes:     stats.HeapAlloc,
		HeapSys:       stats.HeapSys,
		MemLimit:      memLimit,
		NumGoroutine:  runtime.NumGoroutine(),
		GCCount:       stats.GCCount,
		QueueDepths:   make(map[string]int),
		Latencies:     make(map[string]LatencySnapshot),
		GovernorScale: 1.0, // Default
		GovernorState: "unknown",
	}

	// Copy queue depths
	for k, v := range queueDepths {
		snap.QueueDepths[k] = v
	}

	return snap
}

// StartRecording begins periodic snapshot capture in a background goroutine.
func (fr *FlightRecorder) StartRecording(ctx context.Context, interval time.Duration, memLimit uint64) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Capture snapshot
			// Note: Queue depths will be empty here - they should be updated
			// by the engine via UpdateQueueDepths()
			snap := fr.CaptureSnapshot(memLimit, nil)
			fr.Record(snap)
		}
	}
}

// UpdateQueueDepths updates the queue depths in the most recent snapshot.
// Call this from the engine after capturing depths from bus subscriptions.
func (fr *FlightRecorder) UpdateQueueDepths(depths map[string]int) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	// Update most recent snapshot (previous index)
	prevIndex := (fr.index - 1 + fr.size) % fr.size
	if !fr.snapshots[prevIndex].Timestamp.IsZero() {
		if fr.snapshots[prevIndex].QueueDepths == nil {
			fr.snapshots[prevIndex].QueueDepths = make(map[string]int)
		}
		for k, v := range depths {
			fr.snapshots[prevIndex].QueueDepths[k] = v
		}
	}
}

// UpdateGovernor updates the governor state in the most recent snapshot.
func (fr *FlightRecorder) UpdateGovernor(scale float64, state string) {
	fr.mu.Lock()
	defer fr.mu.Unlock()

	prevIndex := (fr.index - 1 + fr.size) % fr.size
	if !fr.snapshots[prevIndex].Timestamp.IsZero() {
		fr.snapshots[prevIndex].GovernorScale = scale
		fr.snapshots[prevIndex].GovernorState = state
	}
}
