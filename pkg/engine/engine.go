package engine

import (
	"context"
	"fmt"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
	"github.com/BYTE-6D65/pipeline/pkg/registry"
)

// Engine wires together core infrastructure components.
// It provides dependency injection for event buses, clock, and registry.
type Engine struct {
	internalBus event.Bus
	externalBus event.Bus
	clock       clock.Clock
	registry    registry.Registry
}

// EngineOption configures an Engine instance.
type EngineOption func(*Engine)

// WithInternalBus sets the internal event bus (for system events).
func WithInternalBus(bus event.Bus) EngineOption {
	return func(e *Engine) {
		e.internalBus = bus
	}
}

// WithExternalBus sets the external event bus (for domain events).
func WithExternalBus(bus event.Bus) EngineOption {
	return func(e *Engine) {
		e.externalBus = bus
	}
}

// WithClock sets the clock implementation.
func WithClock(clk clock.Clock) EngineOption {
	return func(e *Engine) {
		e.clock = clk
	}
}

// WithRegistry sets the registry implementation.
func WithRegistry(reg registry.Registry) EngineOption {
	return func(e *Engine) {
		e.registry = reg
	}
}

// New creates a new Engine with sensible defaults.
// Default configuration:
// - InternalBus: InMemoryBus with 64 buffer, drop-slow disabled
// - ExternalBus: InMemoryBus with 128 buffer, drop-slow disabled
// - Clock: SystemClock (monotonic)
// - Registry: InMemoryRegistry
func New(opts ...EngineOption) *Engine {
	engine := &Engine{
		internalBus: event.NewInMemoryBus(
			event.WithBufferSize(64),
			event.WithDropSlow(false),
			event.WithBusName("internal"),
		),
		externalBus: event.NewInMemoryBus(
			event.WithBufferSize(128),
			event.WithDropSlow(false),
			event.WithBusName("external"),
		),
		clock:    clock.NewSystemClock(),
		registry: registry.NewInMemoryRegistry(),
	}

	for _, opt := range opts {
		opt(engine)
	}

	return engine
}

// InternalBus returns the internal event bus (for system/coordination events).
func (e *Engine) InternalBus() event.Bus {
	return e.internalBus
}

// ExternalBus returns the external event bus (for domain events).
func (e *Engine) ExternalBus() event.Bus {
	return e.externalBus
}

// Clock returns the clock implementation.
func (e *Engine) Clock() clock.Clock {
	return e.clock
}

// Registry returns the registry implementation.
func (e *Engine) Registry() registry.Registry {
	return e.registry
}

// Shutdown gracefully shuts down the engine and releases resources.
// It closes both event buses and waits for the context to complete.
func (e *Engine) Shutdown(ctx context.Context) error {
	errCh := make(chan error, 2)

	// Close internal bus
	go func() {
		if err := e.internalBus.Close(); err != nil {
			errCh <- fmt.Errorf("internal bus shutdown: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Close external bus
	go func() {
		if err := e.externalBus.Close(); err != nil {
			errCh <- fmt.Errorf("external bus shutdown: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Wait for both to complete or context to cancel
	var errors []error
	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				errors = append(errors, err)
			}
		case <-ctx.Done():
			return fmt.Errorf("shutdown cancelled: %w", ctx.Err())
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}
