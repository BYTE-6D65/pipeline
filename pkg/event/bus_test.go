package event

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewInMemoryBus(t *testing.T) {
	bus := NewInMemoryBus()
	if bus == nil {
		t.Fatal("NewInMemoryBus returned nil")
	}

	if bus.subscriptions == nil {
		t.Error("Subscriptions map should be initialized")
	}

	if bus.bufferSize != 64 {
		t.Errorf("Expected default buffer size 64, got %d", bus.bufferSize)
	}

	if bus.dropSlow {
		t.Error("Expected default dropSlow to be false")
	}
}

func TestNewInMemoryBus_WithOptions(t *testing.T) {
	bus := NewInMemoryBus(
		WithBufferSize(128),
		WithDropSlow(true),
	)

	if bus.bufferSize != 128 {
		t.Errorf("Expected buffer size 128, got %d", bus.bufferSize)
	}

	if !bus.dropSlow {
		t.Error("Expected dropSlow to be true")
	}
}

func TestBus_Subscribe(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	filter := Filter{Types: []string{"test.*"}}

	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if sub == nil {
		t.Fatal("Subscribe returned nil subscription")
	}

	if sub.Events() == nil {
		t.Error("Subscription events channel is nil")
	}
}

func TestBus_Subscribe_AfterClose(t *testing.T) {
	bus := NewInMemoryBus()
	bus.Close()

	ctx := context.Background()
	filter := Filter{}

	_, err := bus.Subscribe(ctx, filter)
	if err == nil {
		t.Error("Expected error when subscribing to closed bus")
	}
}

func TestBus_PublishAndReceive(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	filter := Filter{}

	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	evt := &Event{
		ID:     "test-1",
		Type:   "test.event",
		Source: "test-source",
	}

	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case received := <-sub.Events():
		if received.ID != evt.ID {
			t.Errorf("Expected event ID %s, got %s", evt.ID, received.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestBus_PublishToMultipleSubscribers(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	filter := Filter{}

	// Create 3 subscribers
	subs := make([]Subscription, 3)
	for i := range subs {
		sub, err := bus.Subscribe(ctx, filter)
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i, err)
		}
		defer sub.Close()
		subs[i] = sub
	}

	evt := &Event{
		ID:     "broadcast",
		Type:   "test.event",
		Source: "test",
	}

	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Verify all subscribers received the event
	for i, sub := range subs {
		select {
		case received := <-sub.Events():
			if received.ID != evt.ID {
				t.Errorf("Subscriber %d: expected event ID %s, got %s", i, evt.ID, received.ID)
			}
		case <-time.After(time.Second):
			t.Fatalf("Subscriber %d: timeout waiting for event", i)
		}
	}
}

func TestBus_FilterByType(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()

	// Subscribe to "user.*" events
	filter := Filter{Types: []string{"user.*"}}
	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish matching event
	matchingEvt := &Event{ID: "1", Type: "user.created", Source: "test"}
	if err := bus.Publish(ctx, matchingEvt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Publish non-matching event
	nonMatchingEvt := &Event{ID: "2", Type: "product.created", Source: "test"}
	if err := bus.Publish(ctx, nonMatchingEvt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Should only receive the matching event
	select {
	case received := <-sub.Events():
		if received.ID != "1" {
			t.Errorf("Expected event ID '1', got '%s'", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}

	// Verify no other events received
	select {
	case evt := <-sub.Events():
		t.Errorf("Unexpected event received: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestBus_FilterBySource(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()

	filter := Filter{Sources: []string{"source-a"}}
	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish matching event
	bus.Publish(ctx, &Event{ID: "1", Type: "test", Source: "source-a"})

	// Publish non-matching event
	bus.Publish(ctx, &Event{ID: "2", Type: "test", Source: "source-b"})

	select {
	case received := <-sub.Events():
		if received.ID != "1" {
			t.Errorf("Expected event ID '1', got '%s'", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}

	// Verify no other events received
	select {
	case evt := <-sub.Events():
		t.Errorf("Unexpected event received: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestBus_FilterByMetadata(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()

	filter := Filter{Metadata: map[string]string{"env": "test"}}
	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish matching event
	matchingEvt := &Event{
		ID:       "1",
		Type:     "test",
		Source:   "test",
		Metadata: map[string]string{"env": "test"},
	}
	bus.Publish(ctx, matchingEvt)

	// Publish non-matching event
	nonMatchingEvt := &Event{
		ID:       "2",
		Type:     "test",
		Source:   "test",
		Metadata: map[string]string{"env": "prod"},
	}
	bus.Publish(ctx, nonMatchingEvt)

	select {
	case received := <-sub.Events():
		if received.ID != "1" {
			t.Errorf("Expected event ID '1', got '%s'", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}

	// Verify no other events received
	select {
	case evt := <-sub.Events():
		t.Errorf("Unexpected event received: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestBus_CombinedFilters(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()

	filter := Filter{
		Types:    []string{"user.*"},
		Sources:  []string{"api"},
		Metadata: map[string]string{"env": "test"},
	}
	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish fully matching event
	matchingEvt := &Event{
		ID:       "1",
		Type:     "user.created",
		Source:   "api",
		Metadata: map[string]string{"env": "test"},
	}
	bus.Publish(ctx, matchingEvt)

	// Publish partially matching events (should not match)
	bus.Publish(ctx, &Event{ID: "2", Type: "product.created", Source: "api", Metadata: map[string]string{"env": "test"}})
	bus.Publish(ctx, &Event{ID: "3", Type: "user.created", Source: "worker", Metadata: map[string]string{"env": "test"}})
	bus.Publish(ctx, &Event{ID: "4", Type: "user.created", Source: "api", Metadata: map[string]string{"env": "prod"}})

	select {
	case received := <-sub.Events():
		if received.ID != "1" {
			t.Errorf("Expected event ID '1', got '%s'", received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for matching event")
	}

	// Verify no other events received
	select {
	case evt := <-sub.Events():
		t.Errorf("Unexpected event received: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestBus_Unsubscribe(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	filter := Filter{}

	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Unsubscribe
	if err := sub.Close(); err != nil {
		t.Fatalf("Unsubscribe failed: %v", err)
	}

	// Publish event after unsubscribe
	evt := &Event{ID: "1", Type: "test", Source: "test"}
	if err := bus.Publish(ctx, evt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Drain any buffered events and verify channel is eventually closed
	timeout := time.After(100 * time.Millisecond)
	for {
		select {
		case _, ok := <-sub.Events():
			if !ok {
				// Channel closed, this is expected
				return
			}
			// Drain buffered event
		case <-timeout:
			t.Fatal("Timeout waiting for subscription channel to close")
		}
	}
}

func TestBus_Close(t *testing.T) {
	bus := NewInMemoryBus()

	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, Filter{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Close bus
	if err := bus.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify subscription channel is closed
	_, ok := <-sub.Events()
	if ok {
		t.Error("Subscription channel should be closed")
	}

	// Verify cannot publish after close
	evt := &Event{ID: "1", Type: "test", Source: "test"}
	if err := bus.Publish(ctx, evt); err == nil {
		t.Error("Expected error when publishing to closed bus")
	}
}

func TestBus_PublishWithContextCancel(t *testing.T) {
	bus := NewInMemoryBus(WithDropSlow(false)) // Use blocking mode
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Create subscriber that doesn't read events (to cause blocking)
	sub, err := bus.Subscribe(ctx, Filter{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Fill the buffer
	for i := 0; i < 64; i++ {
		evt := &Event{ID: "fill", Type: "test", Source: "test"}
		if err := bus.Publish(ctx, evt); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// Cancel context
	cancel()

	// Try to publish with cancelled context
	evt := &Event{ID: "cancelled", Type: "test", Source: "test"}
	err = bus.Publish(ctx, evt)
	if err == nil {
		t.Error("Expected error when publishing with cancelled context")
	}
}

func TestBus_ConcurrentPublish(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, Filter{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	const numPublishers = 10
	const eventsPerPublisher = 100

	var wg sync.WaitGroup
	wg.Add(numPublishers)

	// Start concurrent publishers
	for i := 0; i < numPublishers; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				evt := &Event{
					ID:     fmt.Sprintf("pub-%d-evt-%d", id, j),
					Type:   "test",
					Source: "concurrent",
				}
				if err := bus.Publish(ctx, evt); err != nil {
					t.Errorf("Publish failed: %v", err)
				}
			}
		}(i)
	}

	// Collect events
	received := make(map[string]bool)
	done := make(chan struct{})

	go func() {
		for evt := range sub.Events() {
			received[evt.ID] = true
			if len(received) == numPublishers*eventsPerPublisher {
				close(done)
				return
			}
		}
	}()

	wg.Wait()

	select {
	case <-done:
		// Success: received all events
	case <-time.After(5 * time.Second):
		t.Fatalf("Timeout: expected %d events, received %d", numPublishers*eventsPerPublisher, len(received))
	}
}

func TestBus_ConcurrentSubscribe(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	const numSubscribers = 50
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	// Create concurrent subscribers
	for i := 0; i < numSubscribers; i++ {
		go func() {
			defer wg.Done()
			sub, err := bus.Subscribe(ctx, Filter{})
			if err != nil {
				t.Errorf("Subscribe failed: %v", err)
				return
			}
			defer sub.Close()
		}()
	}

	wg.Wait()
}

func TestBus_DropSlow(t *testing.T) {
	bus := NewInMemoryBus(WithBufferSize(2), WithDropSlow(true))
	defer bus.Close()

	ctx := context.Background()
	sub, err := bus.Subscribe(ctx, Filter{})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish more events than buffer can hold
	for i := 0; i < 10; i++ {
		evt := &Event{ID: fmt.Sprintf("evt-%d", i), Type: "test", Source: "test"}
		if err := bus.Publish(ctx, evt); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}

	// Should only receive buffer-size events (some dropped)
	received := 0
	timeout := time.After(100 * time.Millisecond)

loop:
	for {
		select {
		case <-sub.Events():
			received++
		case <-timeout:
			break loop
		}
	}

	if received >= 10 {
		t.Errorf("Expected some events to be dropped, but received all %d", received)
	}
}

func TestBus_EmptyFilter_MatchesAll(t *testing.T) {
	bus := NewInMemoryBus()
	defer bus.Close()

	ctx := context.Background()
	filter := Filter{} // Empty filter should match all events

	sub, err := bus.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	events := []*Event{
		{ID: "1", Type: "user.created", Source: "api"},
		{ID: "2", Type: "product.updated", Source: "worker"},
		{ID: "3", Type: "order.deleted", Source: "scheduler"},
	}

	for _, evt := range events {
		if err := bus.Publish(ctx, evt); err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	// Should receive all events
	for i := 0; i < len(events); i++ {
		select {
		case <-sub.Events():
			// Event received
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("Timeout waiting for event %d", i)
		}
	}
}

func TestMatchesAny_Wildcard(t *testing.T) {
	tests := []struct {
		str      string
		patterns []string
		expected bool
	}{
		{"user.created", []string{"user.*"}, true},
		{"user.updated", []string{"user.*"}, true},
		{"product.created", []string{"user.*"}, false},
		{"user.created", []string{"*.created"}, true},
		{"product.created", []string{"*.created"}, true},
		{"user.deleted", []string{"*.created"}, false},
		{"test", []string{"test"}, true},
		{"test", []string{"other"}, false},
		{"user.created", []string{"user.*", "product.*"}, true},
		{"product.updated", []string{"user.*", "product.*"}, true},
		{"order.created", []string{"user.*", "product.*"}, false},
	}

	for _, tt := range tests {
		result := matchesAny(tt.str, tt.patterns)
		if result != tt.expected {
			t.Errorf("matchesAny(%q, %v) = %v, expected %v", tt.str, tt.patterns, result, tt.expected)
		}
	}
}
