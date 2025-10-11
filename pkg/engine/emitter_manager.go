package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/BYTE-6D65/pipeline/pkg/emitter"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// EmitterManager manages the lifecycle of emitters attached to the engine.
// It handles subscribing emitters to the external bus and routing events to them.
type EmitterManager struct {
	engine *Engine
	mu     sync.RWMutex

	emitters      map[string]emitter.Emitter
	filters       map[string]event.Filter
	subscriptions map[string]event.Subscription
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// NewEmitterManager creates a new emitter manager for the given engine.
func NewEmitterManager(engine *Engine) *EmitterManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &EmitterManager{
		engine:        engine,
		emitters:      make(map[string]emitter.Emitter),
		filters:       make(map[string]event.Filter),
		subscriptions: make(map[string]event.Subscription),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Register registers an emitter with the manager and its event filter.
// The emitter is not started until Start() is called.
// If filter is nil or empty, all events will be routed to this emitter.
func (m *EmitterManager) Register(id string, emit emitter.Emitter, filter event.Filter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.emitters[id]; exists {
		return fmt.Errorf("emitter %s already registered", id)
	}

	m.emitters[id] = emit
	m.filters[id] = filter
	return nil
}

// Unregister removes an emitter from the manager.
// If the emitter is running, it will be stopped first.
func (m *EmitterManager) Unregister(emitterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	emitter, exists := m.emitters[emitterID]
	if !exists {
		return fmt.Errorf("emitter %s not found", emitterID)
	}

	// Stop the emitter's subscription
	if sub, ok := m.subscriptions[emitterID]; ok {
		sub.Close()
		delete(m.subscriptions, emitterID)
	}

	// Close the emitter
	if err := emitter.Close(); err != nil {
		return fmt.Errorf("failed to close emitter %s: %w", emitterID, err)
	}

	delete(m.emitters, emitterID)
	delete(m.filters, emitterID)
	return nil
}

// Start starts all registered emitters.
// Each emitter subscribes to the engine's external bus and processes matching events.
func (m *EmitterManager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var startErrors []error

	for id, emit := range m.emitters {
		// Subscribe to events matching this emitter's filter
		filter, ok := m.filters[id]
		if !ok {
			// No filter specified, match all events
			filter = event.Filter{}
		}
		sub, err := m.engine.ExternalBus().Subscribe(m.ctx, filter)
		if err != nil {
			startErrors = append(startErrors, fmt.Errorf("emitter %s: failed to subscribe: %w", id, err))
			continue
		}

		m.subscriptions[id] = sub

		// Start a goroutine to process events for this emitter
		m.wg.Add(1)
		go m.processEvents(id, emit, sub)
	}

	if len(startErrors) > 0 {
		// Best effort: stop any subscriptions that did start
		m.stopAll()
		return fmt.Errorf("failed to start emitters: %v", startErrors)
	}

	return nil
}

// processEvents is the event processing loop for an emitter.
// It runs in a goroutine and processes events from the subscription.
func (m *EmitterManager) processEvents(id string, emitter emitter.Emitter, sub event.Subscription) {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			// Context cancelled, exit
			return

		case evt, ok := <-sub.Events():
			if !ok {
				// Subscription closed, exit
				return
			}

			// Emit the event via the emitter
			if err := emitter.Emit(m.ctx, evt); err != nil {
				// Log error but continue processing
				// In production, this would use a proper logger
				// For now, we silently continue
				continue
			}
		}
	}
}

// Stop stops all running emitters.
func (m *EmitterManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.stopAll()
}

// stopAll is an internal helper that stops all emitters.
// Must be called with lock held.
func (m *EmitterManager) stopAll() error {
	// Close all subscriptions (this will cause processEvents goroutines to exit)
	for id, sub := range m.subscriptions {
		sub.Close()
		delete(m.subscriptions, id)
	}

	// Wait for all processing goroutines to finish
	// We need to unlock before waiting, otherwise we'll deadlock
	m.mu.Unlock()
	m.wg.Wait()
	m.mu.Lock()

	// Close all emitters
	var closeErrors []error
	for id, emitter := range m.emitters {
		if err := emitter.Close(); err != nil {
			closeErrors = append(closeErrors, fmt.Errorf("emitter %s: %w", id, err))
		}
	}

	if len(closeErrors) > 0 {
		return fmt.Errorf("errors closing emitters: %v", closeErrors)
	}

	return nil
}

// Shutdown gracefully shuts down the emitter manager.
// It stops all emitters and cancels the context.
func (m *EmitterManager) Shutdown() error {
	// Cancel the context (signals all goroutines to stop)
	m.cancel()

	// Stop all emitters
	if err := m.Stop(); err != nil {
		return err
	}

	return nil
}

// List returns a list of all registered emitter IDs.
func (m *EmitterManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.emitters))
	for id := range m.emitters {
		ids = append(ids, id)
	}
	return ids
}

// Get retrieves an emitter by ID.
func (m *EmitterManager) Get(emitterID string) (emitter.Emitter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	emitter, exists := m.emitters[emitterID]
	return emitter, exists
}
