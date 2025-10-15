package event

import (
	"time"

	"github.com/go-json-experiment/json"
	"github.com/google/uuid"
)

// Event is a payload-agnostic message envelope that can carry any type of data.
// It supports event correlation, causation tracking, and filtering via metadata.
type Event struct {
	// ID is a unique identifier for this event instance
	ID string `json:"id"`

	// Type is a namespaced event type (e.g., "cmdwheel.device.input", "app.user.created")
	Type string `json:"type"`

	// Source identifies the originating component or subsystem
	Source string `json:"source"`

	// Timestamp indicates when the event was created
	Timestamp time.Time `json:"timestamp"`

	// Data contains the serialized payload (use codec to marshal/unmarshal)
	Data []byte `json:"data,omitempty"`

	// Metadata provides additional context for filtering and debugging
	Metadata map[string]string `json:"metadata,omitempty"`

	// CorrelationID links related events in a workflow or transaction
	CorrelationID string `json:"correlation_id,omitempty"`

	// CausationID identifies the event that directly caused this event
	CausationID string `json:"causation_id,omitempty"`
}

// EventCodec defines how to serialize and deserialize event payloads.
type EventCodec interface {
	// Marshal converts a payload struct to bytes
	Marshal(v any) ([]byte, error)

	// Unmarshal deserializes bytes into a payload struct
	Unmarshal(data []byte, v any) error
}

// JSONCodec implements EventCodec using encoding/json.
type JSONCodec struct{}

// Marshal converts a payload to JSON bytes.
func (c JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON bytes into a payload.
func (c JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// NewEvent creates a new event with a generated ID and current timestamp.
func NewEvent(eventType, source string, payload any, codec EventCodec) (*Event, error) {
	data, err := codec.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return &Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
		Metadata:  make(map[string]string),
	}, nil
}

// WithMetadata adds metadata key-value pairs to the event.
func (e *Event) WithMetadata(key, value string) *Event {
	if e.Metadata == nil {
		e.Metadata = make(map[string]string)
	}
	e.Metadata[key] = value
	return e
}

// WithCorrelationID sets the correlation ID for tracking related events.
func (e *Event) WithCorrelationID(id string) *Event {
	e.CorrelationID = id
	return e
}

// WithCausationID sets the causation ID to link this event to its cause.
func (e *Event) WithCausationID(id string) *Event {
	e.CausationID = id
	return e
}

// DecodePayload deserializes the event data into the provided struct.
func (e *Event) DecodePayload(v any, codec EventCodec) error {
	if len(e.Data) == 0 {
		return nil
	}
	return codec.Unmarshal(e.Data, v)
}
