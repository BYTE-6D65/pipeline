package event

import "time"

// Control event types (published to InternalBus)
const (
	EventTypeGovernorScale  = "control.governor.scale"
	EventTypeWorkerScale    = "control.worker.scale"
	EventTypeBufferResize   = "control.buffer.resize"
	EventTypeBufferOptimize = "control.buffer.optimize"
	EventTypeBusConfig      = "control.bus.config"
	EventTypeForceGC        = "control.gc.force"
)

// GovernorScaleCommand requests governor to change scale.
//
// The scale value represents the throttle level:
//   - 1.0 = full speed (normal operation)
//   - 0.5 = half speed (degraded mode)
//   - 0.1 = minimum speed (critical throttling)
//
// Example:
//
//	cmd := GovernorScaleCommand{
//	    Scale:     0.5,
//	    Reason:    "Memory pressure at 87%",
//	    Source:    "control-lab",
//	    Timestamp: time.Now(),
//	}
//	evt := NewControlEvent(EventTypeGovernorScale, cmd)
//	internalBus.Publish(ctx, evt)
type GovernorScaleCommand struct {
	Scale     float64        `json:"scale"`     // Target scale (0.0-1.0)
	Reason    string         `json:"reason"`    // Why this decision was made
	Source    string         `json:"source"`    // "control-lab", "manual", "health-check"
	Timestamp time.Time      `json:"timestamp"` // When command was issued
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// WorkerScaleCommand requests worker pool scaling.
//
// Actions:
//   - "scale_up": Increase worker count by one
//   - "scale_down": Decrease worker count by one
//   - "set_count": Set to specific count (requires Count field)
//
// Example:
//
//	cmd := WorkerScaleCommand{
//	    Action:    "set_count",
//	    Count:     8,
//	    Reason:    "Manual scale by operator",
//	    Timestamp: time.Now(),
//	}
type WorkerScaleCommand struct {
	Action    string    `json:"action"`          // "scale_up", "scale_down", "set_count"
	Count     int       `json:"count,omitempty"` // Target worker count (for set_count)
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// BufferResizeCommand requests buffer resize.
//
// Target can be:
//   - "all": All buffers in the system
//   - "external": Only ExternalBus buffers
//   - "internal": Only InternalBus buffers
//   - Subscription ID: Specific subscription buffer
//
// Example:
//
//	cmd := BufferResizeCommand{
//	    Target:    "all",
//	    NewSize:   512,
//	    Reason:    "Memory pressure relief",
//	    Timestamp: time.Now(),
//	}
type BufferResizeCommand struct {
	Target    string    `json:"target"`    // "all", "external", "internal", or subscription ID
	NewSize   int       `json:"new_size"`  // Target buffer size
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// BufferOptimizeCommand requests buffer optimization.
//
// Optimization shrinks underutilized buffers to reclaim memory.
// Buffers below MinUtilization threshold are candidates for shrinking.
//
// Example:
//
//	cmd := BufferOptimizeCommand{
//	    MinUtilization: 0.3,  // Shrink if < 30% utilized
//	    Reason:         "Memory pressure at 85%",
//	    Timestamp:      time.Now(),
//	}
type BufferOptimizeCommand struct {
	MinUtilization float64   `json:"min_utilization"` // Shrink if below this (0.0-1.0)
	Reason         string    `json:"reason"`
	Timestamp      time.Time `json:"timestamp"`
}

// BusConfigCommand changes bus configuration at runtime.
//
// Example:
//
//	cmd := BusConfigCommand{
//	    DropSlow:  boolPtr(true),
//	    Reason:    "Enable backpressure protection",
//	    Timestamp: time.Now(),
//	}
type BusConfigCommand struct {
	DropSlow  *bool     `json:"drop_slow,omitempty"` // Enable/disable dropSlow
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// ForceGCCommand requests immediate garbage collection.
//
// Use with caution - forces a stop-the-world GC cycle.
// Only issue when memory pressure is critical and other measures have failed.
//
// Example:
//
//	cmd := ForceGCCommand{
//	    Reason:    "Memory pressure at 95%",
//	    Timestamp: time.Now(),
//	}
type ForceGCCommand struct {
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// NewControlEvent creates a control event with the given type and payload.
//
// The event is automatically populated with:
//   - Unique ID
//   - Source: "control-lab" (can be overridden via SetSource)
//   - Timestamp: Current time
//
// Example:
//
//	evt := NewControlEvent(EventTypeGovernorScale, GovernorScaleCommand{
//	    Scale:  0.5,
//	    Reason: "Memory pressure",
//	    Source: "control-lab",
//	})
//	evt.SetSource("manual-override")  // Override source if needed
func NewControlEvent(eventType string, payload any) *Event {
	evt, err := NewEvent(eventType, "control-lab", payload, JSONCodec{})
	if err != nil {
		// Should not happen with valid payloads
		panic(err)
	}
	return evt
}

// SetSource overrides the source of a control event.
func (e *Event) SetSource(source string) {
	e.Source = source
}
