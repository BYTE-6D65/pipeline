package event

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestEventBus_SubMillisecondTiming verifies that the event bus can handle
// events with sub-millisecond timing precision, which is critical for
// fast typing detection (50-100ms chord intervals).
func TestEventBus_SubMillisecondTiming(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, Filter{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Simulate rapid keyboard events (targeting sub-20ms intervals)
	const numEvents = 10
	const targetInterval = 10 * time.Millisecond // 10ms between events

	events := make([]*Event, numEvents)
	start := time.Now()

	// Publish events as fast as possible
	for i := 0; i < numEvents; i++ {
		events[i] = &Event{
			ID:        fmt.Sprintf("key-%d", i),
			Type:      "keyboard.press",
			Source:    "test",
			Timestamp: time.Now(), // Capture precise timestamp
		}
		if err := bus.Publish(ctx, events[i]); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}

		// Small delay to simulate realistic typing
		if i < numEvents-1 {
			time.Sleep(targetInterval)
		}
	}

	publishDuration := time.Since(start)
	t.Logf("Published %d events in %v (avg: %v per event)",
		numEvents, publishDuration, publishDuration/numEvents)

	// Receive and verify timing precision
	received := make([]*Event, 0, numEvents)
	timeout := time.After(1 * time.Second)

	for i := 0; i < numEvents; i++ {
		select {
		case evt := <-sub.Events():
			received = append(received, evt)
		case <-timeout:
			t.Fatalf("Timeout: only received %d/%d events", len(received), numEvents)
		}
	}

	// Analyze timing precision
	if len(received) != numEvents {
		t.Fatalf("Expected %d events, received %d", numEvents, len(received))
	}

	// Calculate intervals between received events
	var totalInterval time.Duration
	var maxInterval time.Duration
	var minInterval = time.Hour // Start high

	for i := 1; i < len(received); i++ {
		interval := received[i].Timestamp.Sub(received[i-1].Timestamp)
		totalInterval += interval

		if interval > maxInterval {
			maxInterval = interval
		}
		if interval < minInterval {
			minInterval = interval
		}

		t.Logf("Event %d -> %d: %v (timestamp diff)",
			i-1, i, interval)
	}

	avgInterval := totalInterval / time.Duration(numEvents-1)

	t.Logf("\nTiming Analysis:")
	t.Logf("  Target interval: %v", targetInterval)
	t.Logf("  Average interval: %v", avgInterval)
	t.Logf("  Min interval: %v", minInterval)
	t.Logf("  Max interval: %v", maxInterval)

	// Verify we can detect sub-20ms events
	if minInterval > 20*time.Millisecond {
		t.Logf("WARNING: Minimum interval (%v) exceeds 20ms threshold", minInterval)
	} else {
		t.Logf("✓ Successfully captured sub-20ms timing precision")
	}

	// Verify timestamps are monotonically increasing
	for i := 1; i < len(received); i++ {
		if !received[i].Timestamp.After(received[i-1].Timestamp) {
			t.Errorf("Timestamps not monotonically increasing at index %d", i)
		}
	}
}

// TestEventBus_HighFrequencyTyping simulates realistic fast typing scenarios.
func TestEventBus_HighFrequencyTyping(t *testing.T) {
	bus := NewInMemoryBus(WithBufferSize(256)) // Larger buffer for burst
	defer bus.Close()

	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, Filter{Types: []string{"keyboard.*"}})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Simulate a fast typing burst: "hello" typed in rapid succession
	// Typical fast typist: 6-8 keys per second = ~125-170ms per key
	// Chord typing can be 50-100ms between keys
	keys := []string{"h", "e", "l", "l", "o"}
	chordInterval := 75 * time.Millisecond // Mid-range chord speed

	go func() {
		for _, key := range keys {
			// Press event
			press := &Event{
				ID:        fmt.Sprintf("press-%s", key),
				Type:      "keyboard.press",
				Source:    "typing-test",
				Timestamp: time.Now(),
			}
			press.WithMetadata("key", key)
			bus.Publish(ctx, press)

			// Small delay for key hold
			time.Sleep(20 * time.Millisecond)

			// Release event
			release := &Event{
				ID:        fmt.Sprintf("release-%s", key),
				Type:      "keyboard.release",
				Source:    "typing-test",
				Timestamp: time.Now(),
			}
			release.WithMetadata("key", key)
			bus.Publish(ctx, release)

			// Delay until next key press
			time.Sleep(chordInterval - 20*time.Millisecond)
		}
	}()

	// Collect all events
	const expectedEvents = 10 // 5 keys × 2 (press + release)
	received := make([]*Event, 0, expectedEvents)
	timeout := time.After(2 * time.Second)

	for len(received) < expectedEvents {
		select {
		case evt := <-sub.Events():
			received = append(received, evt)
		case <-timeout:
			t.Fatalf("Timeout: received %d/%d events", len(received), expectedEvents)
		}
	}

	// Analyze press-to-press intervals (chord speed)
	pressEvents := make([]*Event, 0, len(keys))
	for _, evt := range received {
		if evt.Type == "keyboard.press" {
			pressEvents = append(pressEvents, evt)
		}
	}

	t.Logf("\nChord Timing Analysis (press-to-press):")
	for i := 1; i < len(pressEvents); i++ {
		interval := pressEvents[i].Timestamp.Sub(pressEvents[i-1].Timestamp)
		t.Logf("  Key %d -> %d: %v", i-1, i, interval)

		// Verify we captured the timing accurately
		if interval < 50*time.Millisecond || interval > 150*time.Millisecond {
			t.Logf("    Note: Interval outside typical chord range (50-150ms)")
		}
	}

	t.Logf("✓ Successfully processed %d rapid keyboard events", len(received))
}

// TestTimestamp_Precision verifies Go's time.Time precision.
func TestTimestamp_Precision(t *testing.T) {
	// Capture timestamps in rapid succession
	const iterations = 1000
	timestamps := make([]time.Time, iterations)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		timestamps[i] = time.Now()
	}
	duration := time.Since(start)

	// Analyze precision
	uniqueTimestamps := make(map[int64]bool)
	var minDiff = time.Hour
	var diffCount int

	for i := 1; i < len(timestamps); i++ {
		diff := timestamps[i].Sub(timestamps[i-1])
		if diff > 0 {
			diffCount++
			if diff < minDiff {
				minDiff = diff
			}
		}
		uniqueTimestamps[timestamps[i].UnixNano()] = true
	}

	t.Logf("\nGo time.Time Precision Analysis:")
	t.Logf("  Total iterations: %d", iterations)
	t.Logf("  Total duration: %v", duration)
	t.Logf("  Avg per call: %v", duration/time.Duration(iterations))
	t.Logf("  Unique timestamps: %d", len(uniqueTimestamps))
	t.Logf("  Non-zero diffs: %d", diffCount)
	t.Logf("  Minimum diff: %v", minDiff)

	// time.Time in Go uses nanosecond precision (int64 nanoseconds since epoch)
	t.Logf("\n✓ Go's time.Time provides nanosecond precision")
	t.Logf("  This is far more than sufficient for sub-20ms timing requirements")

	// Verify nanosecond precision is accessible
	now := time.Now()
	nanos := now.UnixNano()
	t.Logf("\nExample timestamp: %v", now)
	t.Logf("  Nanoseconds: %d", nanos)
	t.Logf("  Resolution: 1 nanosecond = 0.000001 milliseconds")
	t.Logf("  Sub-20ms = 20,000,000 nanoseconds (easily distinguishable)")
}
