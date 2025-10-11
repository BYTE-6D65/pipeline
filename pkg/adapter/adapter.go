package adapter

import (
	"context"
	"errors"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Common errors returned by adapters
var (
	ErrPermissionDenied = errors.New("adapter: permission denied - check access rights")
	ErrDeviceNotFound   = errors.New("adapter: device/source not found")
	ErrAlreadyStarted   = errors.New("adapter: already started")
	ErrNotStarted       = errors.New("adapter: not started")
)

// Adapter represents an event source that translates external events
// into the pipeline's event format and publishes them to the event bus.
//
// Implementations can capture events from any source: hardware devices,
// network streams, file systems, APIs, etc.
//
// Adapters are managed by the engine's AdapterManager which handles
// their lifecycle and connects them to the event bus.
type Adapter interface {
	// ID returns a unique identifier for this adapter instance.
	// Format is implementation-defined (e.g., "keyboard:/dev/input/event0", "api:webhook-1")
	ID() string

	// Type returns the adapter type category (e.g., "keyboard", "api", "file")
	Type() string

	// Start begins capturing events from the source and publishing to the bus.
	// The adapter runs until the context is cancelled or Stop() is called.
	// Returns ErrAlreadyStarted if already running.
	Start(ctx context.Context, bus event.Bus, clk clock.Clock) error

	// Stop gracefully shuts down the adapter and releases resources.
	// Safe to call multiple times (idempotent).
	// Returns ErrNotStarted if not currently running.
	Stop() error
}
