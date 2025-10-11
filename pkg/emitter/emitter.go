package emitter

import (
	"context"
	"errors"

	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// Common errors returned by emitters
var (
	ErrPermissionDenied = errors.New("emitter: permission denied - check access rights")
	ErrDeviceNotFound   = errors.New("emitter: device/target not found")
	ErrNotInitialized   = errors.New("emitter: not initialized")
	ErrInvalidPayload   = errors.New("emitter: invalid event payload")
	ErrUnsupportedEvent = errors.New("emitter: unsupported event type")
)

// Emitter represents an event sink that translates pipeline events
// into external actions or outputs.
//
// Implementations can emit events to any destination: hardware devices,
// network endpoints, file systems, APIs, displays, etc.
//
// Emitters are managed by the engine's EmitterManager which handles
// subscribing to events and routing them to the emitter.
type Emitter interface {
	// ID returns a unique identifier for this emitter instance.
	// Format is implementation-defined (e.g., "keyboard:virtual0", "webhook:slack")
	ID() string

	// Type returns the emitter type category (e.g., "keyboard", "webhook", "logger")
	Type() string

	// Emit processes a single event and emits it to the destination.
	// Returns ErrNotInitialized if the emitter hasn't been set up.
	// Returns ErrInvalidPayload if the event data cannot be parsed.
	// Returns ErrUnsupportedEvent if the event type is not supported.
	Emit(ctx context.Context, evt event.Event) error

	// Close gracefully shuts down the emitter and releases resources.
	// Safe to call multiple times (idempotent).
	Close() error
}
