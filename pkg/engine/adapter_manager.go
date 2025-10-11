package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/BYTE-6D65/pipeline/pkg/adapter"
)

// AdapterManager manages the lifecycle of HID adapters attached to the engine.
// It handles starting/stopping adapters and ensures they publish to the external bus.
type AdapterManager struct {
	engine *Engine
	mu     sync.RWMutex

	adapters map[string]adapter.Adapter
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewAdapterManager creates a new adapter manager for the given engine.
func NewAdapterManager(engine *Engine) *AdapterManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &AdapterManager{
		engine:   engine,
		adapters: make(map[string]adapter.Adapter),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Register registers an adapter with the manager.
// The adapter is not started until Start() is called.
func (m *AdapterManager) Register(adapter adapter.Adapter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := adapter.ID()
	if _, exists := m.adapters[id]; exists {
		return fmt.Errorf("adapter %s already registered", id)
	}

	m.adapters[id] = adapter
	return nil
}

// Unregister removes an adapter from the manager.
// If the adapter is running, it will be stopped first.
func (m *AdapterManager) Unregister(adapterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	adapter, exists := m.adapters[adapterID]
	if !exists {
		return fmt.Errorf("adapter %s not found", adapterID)
	}

	// Stop the adapter if it's running
	if err := adapter.Stop(); err != nil {
		return fmt.Errorf("failed to stop adapter %s: %w", adapterID, err)
	}

	delete(m.adapters, adapterID)
	return nil
}

// Start starts all registered adapters.
// Each adapter is connected to the engine's external bus and clock.
func (m *AdapterManager) Start() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var startErrors []error

	for id, adapter := range m.adapters {
		// Start the adapter with the engine's external bus and clock
		if err := adapter.Start(m.ctx, m.engine.ExternalBus(), m.engine.Clock()); err != nil {
			startErrors = append(startErrors, fmt.Errorf("adapter %s: %w", id, err))
		}
	}

	if len(startErrors) > 0 {
		// Best effort: stop any adapters that did start
		m.stopAll()
		return fmt.Errorf("failed to start adapters: %v", startErrors)
	}

	return nil
}

// Stop stops all running adapters.
func (m *AdapterManager) Stop() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.stopAll()
}

// stopAll is an internal helper that stops all adapters.
// Must be called with at least read lock held.
func (m *AdapterManager) stopAll() error {
	var stopErrors []error

	for id, adapter := range m.adapters {
		if err := adapter.Stop(); err != nil {
			stopErrors = append(stopErrors, fmt.Errorf("adapter %s: %w", id, err))
		}
	}

	if len(stopErrors) > 0 {
		return fmt.Errorf("errors stopping adapters: %v", stopErrors)
	}

	return nil
}

// Shutdown gracefully shuts down the adapter manager.
// It stops all adapters and cancels the context.
func (m *AdapterManager) Shutdown() error {
	// Cancel the context (signals all adapters to stop)
	m.cancel()

	// Stop all adapters
	if err := m.Stop(); err != nil {
		return err
	}

	// Wait for any background work to complete
	m.wg.Wait()

	return nil
}

// List returns a list of all registered adapter IDs.
func (m *AdapterManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.adapters))
	for id := range m.adapters {
		ids = append(ids, id)
	}
	return ids
}

// Get retrieves an adapter by ID.
func (m *AdapterManager) Get(adapterID string) (adapter.Adapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	adapter, exists := m.adapters[adapterID]
	return adapter, exists
}
