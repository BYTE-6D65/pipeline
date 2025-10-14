package event

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/telemetry"
)

// Bus defines the interface for an event bus that supports publish/subscribe patterns.
type Bus interface {
	// Publish sends an event to all subscribers
	Publish(ctx context.Context, evt Event) error

	// Subscribe creates a subscription with optional filtering
	Subscribe(ctx context.Context, filter Filter) (Subscription, error)

	// Close shuts down the bus and releases all resources
	Close() error
}

// Filter defines criteria for filtering events in a subscription.
type Filter struct {
	// Types specifies event types to match (supports wildcards like "app.*")
	Types []string

	// Sources specifies event sources to match
	Sources []string

	// Metadata specifies metadata key-value pairs that must match
	Metadata map[string]string
}

// Subscription represents an active subscription to an event bus.
type Subscription interface {
	// Events returns a channel that receives matching events
	Events() <-chan Event

	// Close unsubscribes and releases resources
	Close() error
}

// InMemoryBus is an in-memory implementation of the Bus interface.
// It supports fan-out to multiple subscribers with configurable buffering.
type InMemoryBus struct {
	mu            sync.RWMutex
	subscriptions map[string]*inMemorySubscription
	closed        bool
	bufferSize    int
	dropSlow      bool // If true, drop events for slow subscribers; if false, block
	name          string
	metrics       *telemetry.Metrics
}

// BusOption configures an InMemoryBus.
type BusOption func(*InMemoryBus)

// WithBufferSize sets the buffer size for subscription channels.
func WithBufferSize(size int) BusOption {
	return func(b *InMemoryBus) {
		b.bufferSize = size
	}
}

// WithDropSlow configures whether to drop events for slow subscribers (true)
// or block until they catch up (false).
func WithDropSlow(drop bool) BusOption {
	return func(b *InMemoryBus) {
		b.dropSlow = drop
	}
}

// WithBusName sets the name for this bus (used in metrics labels).
func WithBusName(name string) BusOption {
	return func(b *InMemoryBus) {
		b.name = name
	}
}

// WithMetrics sets the metrics instance for this bus.
func WithMetrics(metrics *telemetry.Metrics) BusOption {
	return func(b *InMemoryBus) {
		b.metrics = metrics
	}
}

// NewInMemoryBus creates a new in-memory event bus with the given options.
func NewInMemoryBus(opts ...BusOption) *InMemoryBus {
	bus := &InMemoryBus{
		subscriptions: make(map[string]*inMemorySubscription),
		bufferSize:    64, // Default buffer size
		dropSlow:      false,
		name:          "default",
		metrics:       telemetry.Default(),
	}

	for _, opt := range opts {
		opt(bus)
	}

	// Update subscriber gauge
	if bus.metrics != nil {
		bus.metrics.SubscribersTotal.WithLabelValues(bus.name).Set(0)
	}

	return bus
}

// Publish sends an event to all matching subscribers.
func (b *InMemoryBus) Publish(ctx context.Context, evt Event) error {
	// Start timing the entire publish operation
	publishTimer := telemetry.NewTimer()
	defer func() {
		if b.metrics != nil {
			b.metrics.PublishDuration.WithLabelValues(b.name, evt.Type).Observe(publishTimer.Elapsed().Seconds())
			b.metrics.EventsPublished.WithLabelValues(b.name, evt.Type).Inc()
		}
	}()

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("bus is closed")
	}

	// Collect matching subscriptions
	var matching []*inMemorySubscription
	for _, sub := range b.subscriptions {
		// Time the filter matching
		filterTimer := telemetry.NewTimer()
		matches := sub.matches(evt)
		if b.metrics != nil {
			b.metrics.FilterDuration.WithLabelValues(b.name, sub.id).Observe(filterTimer.Elapsed().Seconds())
		}

		if matches {
			matching = append(matching, sub)
		}
	}

	// Send to all matching subscriptions
	for _, sub := range matching {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			sub.send(evt, b.dropSlow, b.name, b.metrics)
		}
	}

	return nil
}

// Subscribe creates a new subscription with the given filter.
func (b *InMemoryBus) Subscribe(ctx context.Context, filter Filter) (Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("bus is closed")
	}

	sub := &inMemorySubscription{
		id:         fmt.Sprintf("sub-%d", len(b.subscriptions)),
		bus:        b,
		filter:     filter,
		ch:         make(chan Event, b.bufferSize),
		closed:     false,
		bufferSize: b.bufferSize,
	}

	b.subscriptions[sub.id] = sub

	// Update metrics
	if b.metrics != nil {
		b.metrics.SubscribersTotal.WithLabelValues(b.name).Set(float64(len(b.subscriptions)))
		b.metrics.BufferSize.WithLabelValues(b.name, sub.id).Set(float64(b.bufferSize))
		b.metrics.BufferUsage.WithLabelValues(b.name, sub.id).Set(0)
	}

	return sub, nil
}

// Close shuts down the bus and all subscriptions.
func (b *InMemoryBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all subscriptions
	for _, sub := range b.subscriptions {
		sub.closeChannel()
	}

	b.subscriptions = nil
	return nil
}

// inMemorySubscription represents a single subscription.
type inMemorySubscription struct {
	id         string
	bus        *InMemoryBus
	filter     Filter
	ch         chan Event
	mu         sync.Mutex
	closed     bool
	bufferSize int
}

// Events returns the channel that receives events.
func (s *inMemorySubscription) Events() <-chan Event {
	return s.ch
}

// Close unsubscribes and closes the event channel.
func (s *inMemorySubscription) Close() error {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()

	delete(s.bus.subscriptions, s.id)
	s.closeChannel()

	// Update metrics
	if s.bus.metrics != nil {
		s.bus.metrics.SubscribersTotal.WithLabelValues(s.bus.name).Set(float64(len(s.bus.subscriptions)))
	}

	return nil
}

// closeChannel closes the event channel (internal use only, assumes lock is held).
func (s *inMemorySubscription) closeChannel() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		close(s.ch)
	}
}

// send attempts to send an event to the subscription channel.
func (s *inMemorySubscription) send(evt Event, dropSlow bool, busName string, metrics *telemetry.Metrics) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	// Start timing the send operation (includes blocking time)
	sendTimer := time.Now()

	if dropSlow {
		// Non-blocking send, drop event if channel is full
		select {
		case s.ch <- evt:
			// Successful send
			if metrics != nil {
				elapsed := time.Since(sendTimer).Seconds()
				metrics.SendDuration.WithLabelValues(busName, s.id, "success").Observe(elapsed)
				metrics.BufferUsage.WithLabelValues(busName, s.id).Set(float64(len(s.ch)))
			}
		default:
			// Event dropped due to slow subscriber
			if metrics != nil {
				elapsed := time.Since(sendTimer).Seconds()
				metrics.SendDuration.WithLabelValues(busName, s.id, "dropped").Observe(elapsed)
				metrics.EventsDropped.WithLabelValues(busName, evt.Type, s.id).Inc()
			}
		}
	} else {
		// Blocking send, wait for space in channel
		// Check if we'll block
		if len(s.ch) >= s.bufferSize {
			if metrics != nil {
				metrics.SendBlocked.WithLabelValues(busName, s.id).Inc()
			}
		}

		s.ch <- evt

		// Record send duration (includes any blocking time!)
		if metrics != nil {
			elapsed := time.Since(sendTimer).Seconds()
			metrics.SendDuration.WithLabelValues(busName, s.id, "success").Observe(elapsed)
			metrics.BufferUsage.WithLabelValues(busName, s.id).Set(float64(len(s.ch)))
		}
	}
}

// matches checks if an event matches the subscription filter.
func (s *inMemorySubscription) matches(evt Event) bool {
	// If no filters specified, match all events
	if len(s.filter.Types) == 0 && len(s.filter.Sources) == 0 && len(s.filter.Metadata) == 0 {
		return true
	}

	// Check type filters
	if len(s.filter.Types) > 0 {
		if !matchesAny(evt.Type, s.filter.Types) {
			return false
		}
	}

	// Check source filters
	if len(s.filter.Sources) > 0 {
		if !matchesAny(evt.Source, s.filter.Sources) {
			return false
		}
	}

	// Check metadata filters
	for key, value := range s.filter.Metadata {
		if evt.Metadata[key] != value {
			return false
		}
	}

	return true
}

// matchesAny checks if a string matches any pattern in the list.
// Supports wildcard patterns using filepath.Match syntax.
func matchesAny(str string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, str)
		if err == nil && matched {
			return true
		}
	}
	return false
}
