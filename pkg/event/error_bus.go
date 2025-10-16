package event

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrorBus is a bounded, lossy, non-blocking bus for error events.
//
// Key properties:
//   - Never blocks publishers (critical path protection)
//   - Bounded buffers (32 events per subscriber)
//   - Drops events on full buffers (lossy by design)
//   - Lock-free read path (atomic.Pointer for subscriptions)
//
// Use this for diagnostic/observability events, NOT critical data events.
type ErrorBus struct {
	subs           atomic.Pointer[[]*ErrorSubscription]
	droppedCounter atomic.Uint64 // Total events dropped across all subs
	mu             sync.Mutex    // Protects subscription modifications only
	closed         bool
	bufferSize     int // Buffer size per subscription
}

// ErrorSubscription represents a subscription to the error bus.
type ErrorSubscription struct {
	id     string
	ch     chan ErrorEvent
	closed atomic.Bool
}

// NewErrorBus creates a new error bus with the given buffer size per subscription.
// Default buffer size is 32 events per subscriber.
func NewErrorBus(bufferSize int) *ErrorBus {
	if bufferSize <= 0 {
		bufferSize = 32 // Sensible default
	}

	bus := &ErrorBus{
		bufferSize: bufferSize,
	}

	// Initialize with empty subscription list
	emptyList := make([]*ErrorSubscription, 0)
	bus.subs.Store(&emptyList)

	return bus
}

// Publish sends an error event to all subscribers.
// This method NEVER blocks - it drops events if subscriber buffers are full.
// Returns the number of successful deliveries.
func (b *ErrorBus) Publish(evt ErrorEvent) int {
	subs := b.subs.Load()
	if subs == nil || len(*subs) == 0 {
		return 0
	}

	delivered := 0

	for i := range *subs {
		sub := (*subs)[i]

		// Skip closed subscriptions
		if sub.closed.Load() {
			continue
		}

		// Non-blocking send - drop on full buffer
		select {
		case sub.ch <- evt:
			delivered++
		default:
			// Buffer full - drop event to protect critical path
			b.droppedCounter.Add(1)
		}
	}

	return delivered
}

// Subscribe creates a new subscription to error events.
// The returned subscription will receive all error events published after subscription.
func (b *ErrorBus) Subscribe(ctx context.Context) (*ErrorSubscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, ErrBusClosed
	}

	sub := &ErrorSubscription{
		id: generateSubscriptionID(),
		ch: make(chan ErrorEvent, b.bufferSize),
	}

	// Copy-on-write: create new subscription list
	oldSubs := b.subs.Load()
	newSubs := make([]*ErrorSubscription, len(*oldSubs)+1)
	copy(newSubs, *oldSubs)
	newSubs[len(*oldSubs)] = sub

	// Atomic swap
	b.subs.Store(&newSubs)

	return sub, nil
}

// Unsubscribe removes a subscription from the error bus.
func (b *ErrorBus) Unsubscribe(sub *ErrorSubscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	// Mark as closed
	sub.closed.Store(true)
	close(sub.ch)

	// Remove from subscription list
	oldSubs := b.subs.Load()
	newSubs := make([]*ErrorSubscription, 0, len(*oldSubs)-1)

	for _, s := range *oldSubs {
		if s != sub {
			newSubs = append(newSubs, s)
		}
	}

	b.subs.Store(&newSubs)
}

// Close shuts down the error bus and all subscriptions.
func (b *ErrorBus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all subscriptions
	subs := b.subs.Load()
	for _, sub := range *subs {
		if !sub.closed.Load() {
			sub.closed.Store(true)
			close(sub.ch)
		}
	}

	// Clear subscription list
	empty := make([]*ErrorSubscription, 0)
	b.subs.Store(&empty)

	return nil
}

// DroppedCount returns the total number of events dropped due to full buffers.
func (b *ErrorBus) DroppedCount() uint64 {
	return b.droppedCounter.Load()
}

// SubscriberCount returns the current number of active subscribers.
func (b *ErrorBus) SubscriberCount() int {
	subs := b.subs.Load()
	if subs == nil {
		return 0
	}
	return len(*subs)
}

// Events returns the channel for receiving error events.
func (s *ErrorSubscription) Events() <-chan ErrorEvent {
	return s.ch
}

// Close closes the subscription and stops receiving events.
func (s *ErrorSubscription) Close() {
	if s.closed.CompareAndSwap(false, true) {
		close(s.ch)
	}
}

// ID returns the subscription identifier.
func (s *ErrorSubscription) ID() string {
	return s.id
}

// generateSubscriptionID generates a unique subscription ID.
var subIDCounter atomic.Uint64

func generateSubscriptionID() string {
	id := subIDCounter.Add(1)
	return string(rune('A' + (id-1)%26)) + string(rune('0' + (id-1)/26))
}

// ErrorHandler is a function that processes error events.
type ErrorHandler func(ErrorEvent)

// Subscribe with a handler that processes events in a background goroutine.
// The goroutine is automatically stopped when the context is cancelled.
func (b *ErrorBus) SubscribeWithHandler(ctx context.Context, handler ErrorHandler) (*ErrorSubscription, error) {
	sub, err := b.Subscribe(ctx)
	if err != nil {
		return nil, err
	}

	go func() {
		defer sub.Close()

		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-sub.Events():
				if !ok {
					return
				}
				handler(evt)
			}
		}
	}()

	return sub, nil
}

var ErrBusClosed = fmt.Errorf("error bus is closed")
