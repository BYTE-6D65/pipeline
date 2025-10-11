package event

import (
	"sort"
	"sync"
	"time"
)

// OrderedEventStore maintains events in chronological order and provides
// efficient querying by time range. This is critical for chord detection
// where precise timing and ordering matter.
type OrderedEventStore struct {
	mu     sync.RWMutex
	events []Event
}

// NewOrderedEventStore creates a new ordered event store.
func NewOrderedEventStore() *OrderedEventStore {
	return &OrderedEventStore{
		events: make([]Event, 0, 1024), // Pre-allocate for performance
	}
}

// Append adds an event to the store, maintaining chronological order.
// Events are expected to arrive mostly in order, so this uses an optimized
// insertion algorithm.
func (s *OrderedEventStore) Append(evt Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Fast path: append if this is the latest event (common case)
	if len(s.events) == 0 || !evt.Timestamp.Before(s.events[len(s.events)-1].Timestamp) {
		s.events = append(s.events, evt)
		return
	}

	// Slow path: find insertion point using binary search
	idx := sort.Search(len(s.events), func(i int) bool {
		return s.events[i].Timestamp.After(evt.Timestamp)
	})

	// Insert at the correct position
	s.events = append(s.events, Event{})
	copy(s.events[idx+1:], s.events[idx:])
	s.events[idx] = evt
}

// GetRange returns all events within the specified time range [start, end).
// Start is inclusive, end is exclusive.
func (s *OrderedEventStore) GetRange(start, end time.Time) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.events) == 0 {
		return nil
	}

	// Find start index
	startIdx := sort.Search(len(s.events), func(i int) bool {
		return !s.events[i].Timestamp.Before(start)
	})

	if startIdx >= len(s.events) {
		return nil
	}

	// Find end index
	endIdx := sort.Search(len(s.events), func(i int) bool {
		return !s.events[i].Timestamp.Before(end)
	})

	// Handle inverted range or empty result
	if endIdx <= startIdx {
		return nil
	}

	// Copy the slice to avoid holding the lock during processing
	result := make([]Event, endIdx-startIdx)
	copy(result, s.events[startIdx:endIdx])
	return result
}

// GetSince returns all events after the specified time.
func (s *OrderedEventStore) GetSince(since time.Time) []Event {
	return s.GetRange(since, time.Now().Add(time.Hour)) // Future cutoff
}

// GetLast returns the N most recent events.
func (s *OrderedEventStore) GetLast(n int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if n <= 0 || len(s.events) == 0 {
		return nil
	}

	if n > len(s.events) {
		n = len(s.events)
	}

	start := len(s.events) - n
	result := make([]Event, n)
	copy(result, s.events[start:])
	return result
}

// GetAll returns all events in chronological order.
func (s *OrderedEventStore) GetAll() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

// Clear removes all events from the store.
func (s *OrderedEventStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = s.events[:0]
}

// Len returns the number of events in the store.
func (s *OrderedEventStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}

// Trim removes events older than the specified duration.
// This prevents unbounded memory growth.
func (s *OrderedEventStore) Trim(keepDuration time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.events) == 0 {
		return 0
	}

	cutoff := time.Now().Add(-keepDuration)

	// Find the first event to keep
	idx := sort.Search(len(s.events), func(i int) bool {
		return !s.events[i].Timestamp.Before(cutoff)
	})

	removed := idx
	if idx > 0 {
		// Shift events to remove old ones
		copy(s.events, s.events[idx:])
		s.events = s.events[:len(s.events)-idx]
	}

	return removed
}

// ChordWindow represents a time window for detecting key chords.
type ChordWindow struct {
	Start  time.Time
	End    time.Time
	Events []Event
}

// DetectChords finds groups of events within the specified time window.
// This is useful for detecting keyboard chords where multiple keys are
// pressed within a short time frame (e.g., 50-100ms).
func (s *OrderedEventStore) DetectChords(windowSize time.Duration, minEvents int) []ChordWindow {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.events) < minEvents {
		return nil
	}

	var chords []ChordWindow

	for i := 0; i < len(s.events); i++ {
		windowStart := s.events[i].Timestamp
		windowEnd := windowStart.Add(windowSize)

		// Find all events within this window
		var windowEvents []Event
		for j := i; j < len(s.events) && s.events[j].Timestamp.Before(windowEnd); j++ {
			windowEvents = append(windowEvents, s.events[j])
		}

		// Check if we found a chord
		if len(windowEvents) >= minEvents {
			chords = append(chords, ChordWindow{
				Start:  windowStart,
				End:    windowEnd,
				Events: windowEvents,
			})
		}
	}

	return chords
}

// GetIntervals calculates the time intervals between consecutive events.
// Returns a slice of durations where intervals[i] is the time between
// events[i] and events[i+1].
func (s *OrderedEventStore) GetIntervals() []time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.events) < 2 {
		return nil
	}

	intervals := make([]time.Duration, len(s.events)-1)
	for i := 1; i < len(s.events); i++ {
		intervals[i-1] = s.events[i].Timestamp.Sub(s.events[i-1].Timestamp)
	}
	return intervals
}
