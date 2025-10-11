package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
	"github.com/BYTE-6D65/pipeline/pkg/registry"
)

// testClock is a simple fake clock for testing
type testClock struct {
	mu  sync.RWMutex
	now clock.MonoTime
}

func newTestClock() *testClock {
	return &testClock{now: 0}
}

func (c *testClock) Now() clock.MonoTime {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.now
}

func (c *testClock) Since(t clock.MonoTime) time.Duration {
	return clock.ToDuration(c.Now() - t)
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now += clock.FromDuration(d)
}

func TestNew(t *testing.T) {
	eng := New()
	if eng == nil {
		t.Fatal("New returned nil")
	}

	if eng.InternalBus() == nil {
		t.Error("InternalBus should not be nil")
	}

	if eng.ExternalBus() == nil {
		t.Error("ExternalBus should not be nil")
	}

	if eng.Clock() == nil {
		t.Error("Clock should not be nil")
	}

	if eng.Registry() == nil {
		t.Error("Registry should not be nil")
	}
}

func TestEngine_DefaultComponents(t *testing.T) {
	eng := New()

	// Test that default clock is SystemClock
	_, ok := eng.Clock().(*clock.SystemClock)
	if !ok {
		t.Error("Expected default clock to be *SystemClock")
	}

	// Test that default registry is InMemoryRegistry
	_, ok = eng.Registry().(*registry.InMemoryRegistry)
	if !ok {
		t.Error("Expected default registry to be InMemoryRegistry")
	}

	// Test that buses work
	ctx := context.Background()

	evt := event.Event{
		ID:     "test",
		Type:   "test.event",
		Source: "test",
	}

	if err := eng.InternalBus().Publish(ctx, evt); err != nil {
		t.Errorf("InternalBus publish failed: %v", err)
	}

	if err := eng.ExternalBus().Publish(ctx, evt); err != nil {
		t.Errorf("ExternalBus publish failed: %v", err)
	}
}

func TestEngine_WithInternalBus(t *testing.T) {
	customBus := event.NewInMemoryBus(event.WithBufferSize(256))
	eng := New(WithInternalBus(customBus))

	if eng.InternalBus() != customBus {
		t.Error("Expected custom internal bus")
	}
}

func TestEngine_WithExternalBus(t *testing.T) {
	customBus := event.NewInMemoryBus(event.WithBufferSize(256))
	eng := New(WithExternalBus(customBus))

	if eng.ExternalBus() != customBus {
		t.Error("Expected custom external bus")
	}
}

func TestEngine_WithClock(t *testing.T) {
	fakeClock := newTestClock()
	eng := New(WithClock(fakeClock))

	if eng.Clock() != fakeClock {
		t.Error("Expected custom clock")
	}
}

func TestEngine_WithRegistry(t *testing.T) {
	customRegistry := registry.NewInMemoryRegistry()
	eng := New(WithRegistry(customRegistry))

	if eng.Registry() != customRegistry {
		t.Error("Expected custom registry")
	}
}

func TestEngine_MultipleOptions(t *testing.T) {
	fakeClock := newTestClock()
	customRegistry := registry.NewInMemoryRegistry()
	customInternalBus := event.NewInMemoryBus()
	customExternalBus := event.NewInMemoryBus()

	eng := New(
		WithClock(fakeClock),
		WithRegistry(customRegistry),
		WithInternalBus(customInternalBus),
		WithExternalBus(customExternalBus),
	)

	if eng.Clock() != fakeClock {
		t.Error("Clock option not applied")
	}

	if eng.Registry() != customRegistry {
		t.Error("Registry option not applied")
	}

	if eng.InternalBus() != customInternalBus {
		t.Error("InternalBus option not applied")
	}

	if eng.ExternalBus() != customExternalBus {
		t.Error("ExternalBus option not applied")
	}
}

func TestEngine_Shutdown(t *testing.T) {
	eng := New()

	ctx := context.Background()
	if err := eng.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// Verify buses are closed by trying to publish
	evt := event.Event{
		ID:     "test",
		Type:   "test.event",
		Source: "test",
	}

	if err := eng.InternalBus().Publish(ctx, evt); err == nil {
		t.Error("Expected error publishing to closed internal bus")
	}

	if err := eng.ExternalBus().Publish(ctx, evt); err == nil {
		t.Error("Expected error publishing to closed external bus")
	}
}

func TestEngine_ShutdownWithTimeout(t *testing.T) {
	eng := New()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := eng.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestEngine_ShutdownCancelled(t *testing.T) {
	eng := New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := eng.Shutdown(ctx)
	if err == nil {
		t.Error("Expected error when shutting down with cancelled context")
	}
}

func TestEngine_InternalBusIsolation(t *testing.T) {
	eng := New()
	ctx := context.Background()

	// Subscribe to internal bus
	internalSub, err := eng.InternalBus().Subscribe(ctx, event.Filter{})
	if err != nil {
		t.Fatalf("Subscribe to internal bus failed: %v", err)
	}
	defer internalSub.Close()

	// Subscribe to external bus
	externalSub, err := eng.ExternalBus().Subscribe(ctx, event.Filter{})
	if err != nil {
		t.Fatalf("Subscribe to external bus failed: %v", err)
	}
	defer externalSub.Close()

	// Publish to internal bus
	internalEvt := event.Event{
		ID:     "internal",
		Type:   "internal.event",
		Source: "test",
	}
	if err := eng.InternalBus().Publish(ctx, internalEvt); err != nil {
		t.Fatalf("Publish to internal bus failed: %v", err)
	}

	// Publish to external bus
	externalEvt := event.Event{
		ID:     "external",
		Type:   "external.event",
		Source: "test",
	}
	if err := eng.ExternalBus().Publish(ctx, externalEvt); err != nil {
		t.Fatalf("Publish to external bus failed: %v", err)
	}

	// Verify internal sub only receives internal event
	select {
	case evt := <-internalSub.Events():
		if evt.ID != "internal" {
			t.Errorf("Expected internal event, got %s", evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for internal event")
	}

	// Verify external sub only receives external event
	select {
	case evt := <-externalSub.Events():
		if evt.ID != "external" {
			t.Errorf("Expected external event, got %s", evt.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for external event")
	}

	// Verify no cross-contamination
	select {
	case evt := <-internalSub.Events():
		t.Errorf("Internal bus received unexpected event: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}

	select {
	case evt := <-externalSub.Events():
		t.Errorf("External bus received unexpected event: %s", evt.ID)
	case <-time.After(50 * time.Millisecond):
		// Expected: no event
	}
}

func TestEngine_RegistryUsage(t *testing.T) {
	eng := New()

	// Store and retrieve values
	eng.Registry().Set("key1", "value1")
	eng.Registry().Set("key2", 42)

	val, ok := eng.Registry().Get("key1")
	if !ok || val != "value1" {
		t.Error("Registry Get failed")
	}

	val, ok = eng.Registry().Get("key2")
	if !ok || val != 42 {
		t.Error("Registry Get failed")
	}
}

func TestEngine_ClockUsage(t *testing.T) {
	fakeClock := newTestClock()
	eng := New(WithClock(fakeClock))

	start := eng.Clock().Now()
	fakeClock.Advance(1 * time.Hour)
	elapsed := eng.Clock().Since(start)

	if elapsed != 1*time.Hour {
		t.Errorf("Expected 1 hour elapsed, got %v", elapsed)
	}
}

func TestEngine_IntegrationTest(t *testing.T) {
	// Create engine with fake clock for deterministic testing
	fakeClock := newTestClock()
	eng := New(WithClock(fakeClock))
	defer eng.Shutdown(context.Background())

	ctx := context.Background()

	// Register a capability in the registry
	eng.Registry().Set("test-capability", map[string]string{
		"name": "Test Capability",
		"type": "test",
	})

	// Subscribe to external bus
	sub, err := eng.ExternalBus().Subscribe(ctx, event.Filter{
		Types: []string{"test.*"},
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Close()

	// Publish event with timestamp from engine clock
	evt := event.Event{
		ID:        "test-1",
		Type:      "test.event",
		Source:    "integration-test",
		Timestamp: time.Now(), // Note: Event.Timestamp is still time.Time
	}

	if err := eng.ExternalBus().Publish(ctx, evt); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive and verify event
	select {
	case received := <-sub.Events():
		if received.ID != "test-1" {
			t.Errorf("Expected event ID 'test-1', got %s", received.ID)
		}
		if !received.Timestamp.Equal(evt.Timestamp) {
			t.Error("Timestamp mismatch")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for event")
	}

	// Verify registry still has the capability
	val, ok := eng.Registry().Get("test-capability")
	if !ok {
		t.Error("Capability not found in registry")
	}
	capMap, ok := val.(map[string]string)
	if !ok || capMap["name"] != "Test Capability" {
		t.Error("Capability data mismatch")
	}
}
