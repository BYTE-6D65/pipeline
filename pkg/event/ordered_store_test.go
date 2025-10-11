package event

import (
	"fmt"
	"testing"
	"time"
)

func TestOrderedEventStore_AppendInOrder(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Append events in order
	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
		}
		store.Append(evt)
	}

	if store.Len() != 10 {
		t.Errorf("Expected 10 events, got %d", store.Len())
	}

	// Verify order
	events := store.GetAll()
	for i := 1; i < len(events); i++ {
		if events[i].Timestamp.Before(events[i-1].Timestamp) {
			t.Errorf("Events not in order at index %d", i)
		}
	}
}

func TestOrderedEventStore_AppendOutOfOrder(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Append events out of order
	timestamps := []int{0, 5, 2, 8, 1, 9, 3, 7, 4, 6}
	for _, ts := range timestamps {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", ts),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(ts) * time.Millisecond),
		}
		store.Append(evt)
	}

	// Verify they're stored in chronological order
	events := store.GetAll()
	for i := 0; i < len(events); i++ {
		expectedID := fmt.Sprintf("evt-%d", i)
		if events[i].ID != expectedID {
			t.Errorf("Expected ID %s at index %d, got %s", expectedID, i, events[i].ID)
		}
	}
}

func TestOrderedEventStore_GetRange(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add 10 events at 1ms intervals
	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
		}
		store.Append(evt)
	}

	// Get events from 2ms to 6ms (should get events 2, 3, 4, 5)
	start := base.Add(2 * time.Millisecond)
	end := base.Add(6 * time.Millisecond)
	events := store.GetRange(start, end)

	if len(events) != 4 {
		t.Errorf("Expected 4 events, got %d", len(events))
	}

	expectedIDs := []string{"evt-2", "evt-3", "evt-4", "evt-5"}
	for i, evt := range events {
		if evt.ID != expectedIDs[i] {
			t.Errorf("Expected ID %s at index %d, got %s", expectedIDs[i], i, evt.ID)
		}
	}
}

func TestOrderedEventStore_GetLast(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
		}
		store.Append(evt)
	}

	// Get last 3 events
	events := store.GetLast(3)

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	expectedIDs := []string{"evt-7", "evt-8", "evt-9"}
	for i, evt := range events {
		if evt.ID != expectedIDs[i] {
			t.Errorf("Expected ID %s at index %d, got %s", expectedIDs[i], i, evt.ID)
		}
	}
}

func TestOrderedEventStore_Trim(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now().Add(-100 * time.Millisecond) // Start 100ms ago

	// Add 10 events spread over 100ms
	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 10 * time.Millisecond),
		}
		store.Append(evt)
	}

	// Trim events older than 50ms (should keep last 5 events approximately)
	removed := store.Trim(50 * time.Millisecond)

	t.Logf("Trimmed %d old events", removed)

	remainingLen := store.Len()
	if remainingLen == 0 {
		t.Error("Expected some events to remain after trim")
	}

	t.Logf("Remaining events: %d", remainingLen)

	// Verify all remaining events are recent enough
	events := store.GetAll()
	cutoff := time.Now().Add(-50 * time.Millisecond)
	for i, evt := range events {
		if evt.Timestamp.Before(cutoff) {
			t.Errorf("Event %d should have been trimmed (timestamp: %v, cutoff: %v)",
				i, evt.Timestamp, cutoff)
		}
	}
}

func TestOrderedEventStore_DetectChords_SingleChord(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	// Simulate a 3-key chord pressed within 50ms
	store.Append(Event{
		ID:        "key-h",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base,
	})
	store.Append(Event{
		ID:        "key-e",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base.Add(20 * time.Millisecond),
	})
	store.Append(Event{
		ID:        "key-l",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base.Add(45 * time.Millisecond),
	})

	// Add a later key that's outside the window
	store.Append(Event{
		ID:        "key-o",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base.Add(200 * time.Millisecond),
	})

	// Detect chords with 100ms window, minimum 3 keys
	chords := store.DetectChords(100*time.Millisecond, 3)

	if len(chords) != 1 {
		t.Errorf("Expected 1 chord, got %d", len(chords))
	}

	if len(chords[0].Events) != 3 {
		t.Errorf("Expected chord with 3 events, got %d", len(chords[0].Events))
	}

	// Verify the chord contains the right keys
	expectedIDs := []string{"key-h", "key-e", "key-l"}
	for i, evt := range chords[0].Events {
		if evt.ID != expectedIDs[i] {
			t.Errorf("Expected chord key %s at index %d, got %s", expectedIDs[i], i, evt.ID)
		}
	}
}

func TestOrderedEventStore_DetectChords_FastTyping(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	// Simulate typing "hello" with varying speeds
	// "he" is a chord (20ms apart)
	// "llo" is separate (100ms+ intervals)
	keys := []struct {
		key    string
		offset time.Duration
	}{
		{"h", 0},
		{"e", 20 * time.Millisecond},  // Chord with 'h'
		{"l", 150 * time.Millisecond}, // Separate
		{"l", 250 * time.Millisecond}, // Separate
		{"o", 350 * time.Millisecond}, // Separate
	}

	for _, k := range keys {
		store.Append(Event{
			ID:        fmt.Sprintf("key-%s", k.key),
			Type:      "keyboard.press",
			Source:    "typing-test",
			Timestamp: base.Add(k.offset),
		})
	}

	// Detect chords with 50ms window, minimum 2 keys
	chords := store.DetectChords(50*time.Millisecond, 2)

	if len(chords) < 1 {
		t.Error("Expected to detect at least one chord (h-e)")
	}

	// The first chord should be "h-e"
	if chords[0].Events[0].ID != "key-h" || chords[0].Events[1].ID != "key-e" {
		t.Error("First chord should be 'h-e'")
	}

	t.Logf("Detected %d chord(s) in typing sequence", len(chords))
	for i, chord := range chords {
		duration := chord.End.Sub(chord.Start)
		t.Logf("  Chord %d: %d keys in %v", i, len(chord.Events), duration)
	}
}

func TestOrderedEventStore_GetIntervals(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	// Add events with known intervals
	intervals := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		15 * time.Millisecond,
		50 * time.Millisecond,
	}

	currentTime := base
	store.Append(Event{
		ID:        "evt-0",
		Type:      "test",
		Source:    "test",
		Timestamp: currentTime,
	})

	for i, interval := range intervals {
		currentTime = currentTime.Add(interval)
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i+1),
			Type:      "test",
			Source:    "test",
			Timestamp: currentTime,
		})
	}

	// Get intervals
	calculated := store.GetIntervals()

	if len(calculated) != len(intervals) {
		t.Fatalf("Expected %d intervals, got %d", len(intervals), len(calculated))
	}

	for i, expected := range intervals {
		// Allow small tolerance for timing precision
		diff := calculated[i] - expected
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Microsecond {
			t.Errorf("Interval %d: expected %v, got %v (diff: %v)",
				i, expected, calculated[i], diff)
		}
	}
}

func TestOrderedEventStore_SubMillisecondPrecision(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	// Add events with sub-millisecond intervals (100 microseconds)
	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 100 * time.Microsecond),
		}
		store.Append(evt)
	}

	intervals := store.GetIntervals()

	// Verify all intervals are ~100 microseconds
	for i, interval := range intervals {
		if interval < 50*time.Microsecond || interval > 150*time.Microsecond {
			t.Errorf("Interval %d out of range: %v", i, interval)
		}
	}

	t.Logf("✓ Successfully stored and retrieved events with sub-millisecond precision")
	t.Logf("  Average interval: %v", intervals[0])
	t.Logf("  This is 10x better than the 20ms requirement")
}

func TestOrderedEventStore_ChordDetection_20msTarget(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Now()

	// Simulate extremely fast chord: 3 keys within 20ms
	store.Append(Event{
		ID:        "key-1",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base,
	})
	store.Append(Event{
		ID:        "key-2",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base.Add(8 * time.Millisecond),
	})
	store.Append(Event{
		ID:        "key-3",
		Type:      "keyboard.press",
		Source:    "test",
		Timestamp: base.Add(18 * time.Millisecond),
	})

	// Detect with 20ms window
	chords := store.DetectChords(20*time.Millisecond, 3)

	if len(chords) != 1 {
		t.Fatalf("Expected to detect 1 chord, got %d", len(chords))
	}

	if len(chords[0].Events) != 3 {
		t.Errorf("Expected 3 keys in chord, got %d", len(chords[0].Events))
	}

	duration := chords[0].Events[2].Timestamp.Sub(chords[0].Events[0].Timestamp)
	t.Logf("✓ Successfully detected 3-key chord within %v", duration)
	t.Logf("  This meets the sub-20ms requirement")
}

func TestOrderedEventStore_Clear(t *testing.T) {
	store := NewOrderedEventStore()

	for i := 0; i < 5; i++ {
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: time.Now(),
		})
	}

	if store.Len() != 5 {
		t.Errorf("Expected 5 events before clear, got %d", store.Len())
	}

	store.Clear()

	if store.Len() != 0 {
		t.Errorf("Expected 0 events after clear, got %d", store.Len())
	}
}

// Test GetSince (currently 0% coverage)
func TestOrderedEventStore_GetSince(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add 10 events at 10ms intervals
	for i := 0; i < 10; i++ {
		evt := Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 10 * time.Millisecond),
		}
		store.Append(evt)
	}

	// Get events since 50ms (should get events 5-9)
	cutoff := base.Add(50 * time.Millisecond)
	events := store.GetSince(cutoff)

	if len(events) != 5 {
		t.Errorf("Expected 5 events since cutoff, got %d", len(events))
	}

	// Verify all returned events are after cutoff
	for i, evt := range events {
		if evt.Timestamp.Before(cutoff) {
			t.Errorf("Event %d has timestamp before cutoff: %v < %v", i, evt.Timestamp, cutoff)
		}
	}

	// Verify they are the right events
	expectedIDs := []string{"evt-5", "evt-6", "evt-7", "evt-8", "evt-9"}
	for i, evt := range events {
		if evt.ID != expectedIDs[i] {
			t.Errorf("Expected ID %s at index %d, got %s", expectedIDs[i], i, evt.ID)
		}
	}
}

// Test edge cases for GetLast
func TestOrderedEventStore_GetLast_EdgeCases(t *testing.T) {
	store := NewOrderedEventStore()

	// Test with empty store
	events := store.GetLast(5)
	if len(events) != 0 {
		t.Errorf("Expected 0 events from empty store, got %d", len(events))
	}

	// Add 3 events
	base := time.Now()
	for i := 0; i < 3; i++ {
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * time.Millisecond),
		})
	}

	// Request more than available (should return all 3)
	events = store.GetLast(10)
	if len(events) != 3 {
		t.Errorf("Expected 3 events (all available), got %d", len(events))
	}

	// Request 0 events
	events = store.GetLast(0)
	if len(events) != 0 {
		t.Errorf("Expected 0 events when requesting 0, got %d", len(events))
	}
}

// Test edge cases for GetRange
func TestOrderedEventStore_GetRange_EdgeCases(t *testing.T) {
	store := NewOrderedEventStore()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add events
	for i := 0; i < 10; i++ {
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 10 * time.Millisecond),
		})
	}

	// Test range before all events
	events := store.GetRange(
		base.Add(-100*time.Millisecond),
		base.Add(-50*time.Millisecond),
	)
	if len(events) != 0 {
		t.Errorf("Expected 0 events before range, got %d", len(events))
	}

	// Test range after all events
	events = store.GetRange(
		base.Add(200*time.Millisecond),
		base.Add(300*time.Millisecond),
	)
	if len(events) != 0 {
		t.Errorf("Expected 0 events after range, got %d", len(events))
	}

	// Test inverted range (end before start) - should return empty
	events = store.GetRange(
		base.Add(50*time.Millisecond),
		base.Add(20*time.Millisecond),
	)
	if len(events) != 0 {
		t.Errorf("Expected 0 events for inverted range, got %d", len(events))
	}

	// Test exact boundaries (start inclusive, end exclusive)
	events = store.GetRange(
		base.Add(20*time.Millisecond), // evt-2 (inclusive)
		base.Add(40*time.Millisecond), // evt-4 (exclusive)
	)
	if len(events) != 2 { // Should include evt-2, evt-3 only
		t.Errorf("Expected 2 events [start, end), got %d", len(events))
	}

	// Test inclusive end by adding 1 nanosecond
	events = store.GetRange(
		base.Add(20*time.Millisecond), // evt-2
		base.Add(40*time.Millisecond).Add(1*time.Nanosecond), // Just after evt-4
	)
	if len(events) != 3 { // Should include evt-2, evt-3, evt-4
		t.Errorf("Expected 3 events with end+1ns, got %d", len(events))
	}
}

// Test edge cases for Trim
func TestOrderedEventStore_Trim_EdgeCases(t *testing.T) {
	store := NewOrderedEventStore()

	// Test trimming empty store
	removed := store.Trim(100 * time.Millisecond)
	if removed != 0 {
		t.Errorf("Expected 0 events removed from empty store, got %d", removed)
	}

	// Add events and trim with 0 duration (should remove all)
	base := time.Now().Add(-1 * time.Second)
	for i := 0; i < 5; i++ {
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 100 * time.Millisecond),
		})
	}

	removed = store.Trim(0)
	if removed != 5 {
		t.Errorf("Expected all 5 events removed with 0 duration, got %d", removed)
	}
	if store.Len() != 0 {
		t.Errorf("Expected empty store after trim(0), got %d events", store.Len())
	}
}

// Test edge cases for DetectChords
func TestOrderedEventStore_DetectChords_EdgeCases(t *testing.T) {
	store := NewOrderedEventStore()

	// Test with empty store
	chords := store.DetectChords(50*time.Millisecond, 2)
	if len(chords) != 0 {
		t.Errorf("Expected 0 chords from empty store, got %d", len(chords))
	}

	// Test with single event (can't form chord)
	store.Append(Event{
		ID:        "evt-0",
		Type:      "test",
		Source:    "test",
		Timestamp: time.Now(),
	})
	chords = store.DetectChords(50*time.Millisecond, 2)
	if len(chords) != 0 {
		t.Errorf("Expected 0 chords from single event, got %d", len(chords))
	}

	// Test with minSize = 0 (should still work, just find all groupings)
	store.Clear()
	base := time.Now()
	for i := 0; i < 3; i++ {
		store.Append(Event{
			ID:        fmt.Sprintf("evt-%d", i),
			Type:      "test",
			Source:    "test",
			Timestamp: base.Add(time.Duration(i) * 10 * time.Millisecond),
		})
	}
	chords = store.DetectChords(50*time.Millisecond, 0)
	if len(chords) == 0 {
		t.Error("Expected some chords even with minSize=0")
	}
}

// Test edge cases for GetIntervals
func TestOrderedEventStore_GetIntervals_EdgeCases(t *testing.T) {
	store := NewOrderedEventStore()

	// Test with empty store
	intervals := store.GetIntervals()
	if len(intervals) != 0 {
		t.Errorf("Expected 0 intervals from empty store, got %d", len(intervals))
	}

	// Test with single event (no intervals)
	store.Append(Event{
		ID:        "evt-0",
		Type:      "test",
		Source:    "test",
		Timestamp: time.Now(),
	})
	intervals = store.GetIntervals()
	if len(intervals) != 0 {
		t.Errorf("Expected 0 intervals from single event, got %d", len(intervals))
	}

	// Test with two events (one interval)
	store.Append(Event{
		ID:        "evt-1",
		Type:      "test",
		Source:    "test",
		Timestamp: time.Now().Add(10 * time.Millisecond),
	})
	intervals = store.GetIntervals()
	if len(intervals) != 1 {
		t.Errorf("Expected 1 interval from two events, got %d", len(intervals))
	}
}
