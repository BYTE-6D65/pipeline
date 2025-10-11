package event

import (
	"encoding/json"
	"testing"
	"time"
)

type testPayload struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

func TestNewEvent(t *testing.T) {
	codec := JSONCodec{}
	payload := testPayload{Message: "hello", Count: 42}

	evt, err := NewEvent("test.event", "test-source", payload, codec)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	if evt.ID == "" {
		t.Error("Event ID should not be empty")
	}

	if evt.Type != "test.event" {
		t.Errorf("Expected type 'test.event', got '%s'", evt.Type)
	}

	if evt.Source != "test-source" {
		t.Errorf("Expected source 'test-source', got '%s'", evt.Source)
	}

	if evt.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	if time.Since(evt.Timestamp) > time.Second {
		t.Error("Timestamp should be recent")
	}

	if len(evt.Data) == 0 {
		t.Error("Event data should not be empty")
	}

	if evt.Metadata == nil {
		t.Error("Metadata should be initialized")
	}
}

func TestEvent_DecodePayload(t *testing.T) {
	codec := JSONCodec{}
	original := testPayload{Message: "test message", Count: 123}

	evt, err := NewEvent("test.event", "test-source", original, codec)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	var decoded testPayload
	if err := evt.DecodePayload(&decoded, codec); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Message != original.Message {
		t.Errorf("Expected message '%s', got '%s'", original.Message, decoded.Message)
	}

	if decoded.Count != original.Count {
		t.Errorf("Expected count %d, got %d", original.Count, decoded.Count)
	}
}

func TestEvent_DecodePayload_EmptyData(t *testing.T) {
	evt := &Event{
		ID:     "test-id",
		Type:   "test.event",
		Source: "test-source",
		Data:   nil,
	}

	var decoded testPayload
	codec := JSONCodec{}
	if err := evt.DecodePayload(&decoded, codec); err != nil {
		t.Errorf("DecodePayload with nil data should not error: %v", err)
	}
}

func TestEvent_WithMetadata(t *testing.T) {
	evt := &Event{
		ID:       "test-id",
		Type:     "test.event",
		Source:   "test-source",
		Metadata: nil,
	}

	evt.WithMetadata("key1", "value1").WithMetadata("key2", "value2")

	if evt.Metadata == nil {
		t.Fatal("Metadata should be initialized")
	}

	if evt.Metadata["key1"] != "value1" {
		t.Errorf("Expected metadata key1='value1', got '%s'", evt.Metadata["key1"])
	}

	if evt.Metadata["key2"] != "value2" {
		t.Errorf("Expected metadata key2='value2', got '%s'", evt.Metadata["key2"])
	}
}

func TestEvent_WithCorrelationID(t *testing.T) {
	evt := &Event{
		ID:     "test-id",
		Type:   "test.event",
		Source: "test-source",
	}

	correlationID := "correlation-123"
	evt.WithCorrelationID(correlationID)

	if evt.CorrelationID != correlationID {
		t.Errorf("Expected correlation ID '%s', got '%s'", correlationID, evt.CorrelationID)
	}
}

func TestEvent_WithCausationID(t *testing.T) {
	evt := &Event{
		ID:     "test-id",
		Type:   "test.event",
		Source: "test-source",
	}

	causationID := "cause-456"
	evt.WithCausationID(causationID)

	if evt.CausationID != causationID {
		t.Errorf("Expected causation ID '%s', got '%s'", causationID, evt.CausationID)
	}
}

func TestEvent_ChainedBuilders(t *testing.T) {
	codec := JSONCodec{}
	payload := testPayload{Message: "chained", Count: 7}

	evt, err := NewEvent("test.event", "test-source", payload, codec)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	evt.WithMetadata("env", "test").
		WithMetadata("version", "1.0").
		WithCorrelationID("corr-123").
		WithCausationID("cause-456")

	if evt.Metadata["env"] != "test" {
		t.Error("Metadata 'env' not set correctly")
	}

	if evt.Metadata["version"] != "1.0" {
		t.Error("Metadata 'version' not set correctly")
	}

	if evt.CorrelationID != "corr-123" {
		t.Error("CorrelationID not set correctly")
	}

	if evt.CausationID != "cause-456" {
		t.Error("CausationID not set correctly")
	}
}

func TestJSONCodec_Marshal(t *testing.T) {
	codec := JSONCodec{}
	payload := testPayload{Message: "marshal test", Count: 99}

	data, err := codec.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Marshaled data should not be empty")
	}

	// Verify it's valid JSON
	var decoded testPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Errorf("Marshaled data is not valid JSON: %v", err)
	}

	if decoded.Message != payload.Message || decoded.Count != payload.Count {
		t.Error("Marshaled data does not match original payload")
	}
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	codec := JSONCodec{}
	jsonData := []byte(`{"message":"unmarshal test","count":77}`)

	var payload testPayload
	if err := codec.Unmarshal(jsonData, &payload); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if payload.Message != "unmarshal test" {
		t.Errorf("Expected message 'unmarshal test', got '%s'", payload.Message)
	}

	if payload.Count != 77 {
		t.Errorf("Expected count 77, got %d", payload.Count)
	}
}

func TestJSONCodec_MarshalUnmarshal_RoundTrip(t *testing.T) {
	codec := JSONCodec{}
	original := testPayload{Message: "roundtrip", Count: 42}

	data, err := codec.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded testPayload
	if err := codec.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Message != original.Message || decoded.Count != original.Count {
		t.Error("Round-trip encoding/decoding produced different result")
	}
}

func TestEvent_JSONSerialization(t *testing.T) {
	codec := JSONCodec{}
	payload := testPayload{Message: "serialize", Count: 100}

	evt, err := NewEvent("test.event", "test-source", payload, codec)
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	evt.WithMetadata("key", "value").
		WithCorrelationID("corr-id").
		WithCausationID("cause-id")

	// Serialize the entire event
	data, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	// Deserialize
	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if decoded.ID != evt.ID {
		t.Error("ID mismatch after serialization")
	}

	if decoded.Type != evt.Type {
		t.Error("Type mismatch after serialization")
	}

	if decoded.Source != evt.Source {
		t.Error("Source mismatch after serialization")
	}

	if decoded.CorrelationID != evt.CorrelationID {
		t.Error("CorrelationID mismatch after serialization")
	}

	if decoded.CausationID != evt.CausationID {
		t.Error("CausationID mismatch after serialization")
	}

	if decoded.Metadata["key"] != "value" {
		t.Error("Metadata mismatch after serialization")
	}

	// Verify payload can be decoded
	var decodedPayload testPayload
	if err := decoded.DecodePayload(&decodedPayload, codec); err != nil {
		t.Fatalf("Failed to decode payload: %v", err)
	}

	if decodedPayload.Message != payload.Message || decodedPayload.Count != payload.Count {
		t.Error("Payload mismatch after serialization")
	}
}

func TestNewEvent_MarshalError(t *testing.T) {
	codec := JSONCodec{}
	// Create an unmarshalable payload (channel cannot be marshaled to JSON)
	invalidPayload := make(chan int)

	_, err := NewEvent("test.event", "test-source", invalidPayload, codec)
	if err == nil {
		t.Error("Expected error when marshaling invalid payload")
	}
}
