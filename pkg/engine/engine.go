package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
	"github.com/BYTE-6D65/pipeline/pkg/registry"
	"github.com/BYTE-6D65/pipeline/pkg/telemetry"
)

// Engine wires together core infrastructure components.
// It provides dependency injection for event buses, clock, and registry.
type Engine struct {
	internalBus event.Bus
	externalBus event.Bus
	clock       clock.Clock
	registry    registry.Registry
	metrics     *telemetry.Metrics
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

// WithMetrics sets the telemetry metrics instance.
func WithMetrics(metrics *telemetry.Metrics) EngineOption {
	return func(e *Engine) {
		e.metrics = metrics
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
		clock:    clock.NewSystemClock(),
		registry: registry.NewInMemoryRegistry(),
		metrics:  telemetry.Default(),
	}

	for _, opt := range opts {
		opt(engine)
	}

	if engine.metrics == nil {
		engine.metrics = telemetry.Default()
	}

	if engine.internalBus == nil {
		engine.internalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("internal"),
			event.WithMetrics(engine.metrics),
		)
	}

	if engine.externalBus == nil {
		engine.externalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("external"),
			event.WithMetrics(engine.metrics),
		)
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

// Metrics returns the telemetry metrics instance.
func (e *Engine) Metrics() *telemetry.Metrics {
	return e.metrics
}

// Shutdown gracefully shuts down the engine and releases resources.
// It closes both event buses and waits for the context to complete.
func (e *Engine) Shutdown(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		recordEngineOperation(e.metrics, "engine.shutdown", start, err)
	}()

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
			err = fmt.Errorf("shutdown cancelled: %w", ctx.Err())
			return
		}
	}

	if len(errors) > 0 {
		err = fmt.Errorf("shutdown errors: %v", errors)
		return
	}

	return
}
