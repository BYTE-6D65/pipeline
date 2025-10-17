package event

import (
	"testing"
	"time"
)

func TestGovernorScaleCommand(t *testing.T) {
	cmd := GovernorScaleCommand{
		Scale:     0.5,
		Reason:    "Memory pressure at 87%",
		Source:    "control-lab",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeGovernorScale, cmd)
	if evt.Type != EventTypeGovernorScale {
		t.Errorf("Wrong event type: %s", evt.Type)
	}

	var decoded GovernorScaleCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Scale != 0.5 {
		t.Errorf("Scale = %f, want 0.5", decoded.Scale)
	}

	if decoded.Reason != "Memory pressure at 87%" {
		t.Errorf("Reason = %s, want 'Memory pressure at 87%%'", decoded.Reason)
	}
}

func TestWorkerScaleCommand_ScaleUp(t *testing.T) {
	cmd := WorkerScaleCommand{
		Action:    "scale_up",
		Reason:    "Queue saturation",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeWorkerScale, cmd)

	var decoded WorkerScaleCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Action != "scale_up" {
		t.Errorf("Action = %s, want 'scale_up'", decoded.Action)
	}
}

func TestWorkerScaleCommand_SetCount(t *testing.T) {
	cmd := WorkerScaleCommand{
		Action:    "set_count",
		Count:     8,
		Reason:    "Manual override",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeWorkerScale, cmd)

	var decoded WorkerScaleCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Count != 8 {
		t.Errorf("Count = %d, want 8", decoded.Count)
	}
}

func TestBufferResizeCommand(t *testing.T) {
	cmd := BufferResizeCommand{
		Target:    "all",
		NewSize:   512,
		Reason:    "Memory pressure relief",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeBufferResize, cmd)

	var decoded BufferResizeCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.NewSize != 512 {
		t.Errorf("NewSize = %d, want 512", decoded.NewSize)
	}
}

func TestBufferOptimizeCommand(t *testing.T) {
	cmd := BufferOptimizeCommand{
		MinUtilization: 0.3,
		Reason:         "Memory pressure",
		Timestamp:      time.Now(),
	}

	evt := NewControlEvent(EventTypeBufferOptimize, cmd)

	var decoded BufferOptimizeCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.MinUtilization != 0.3 {
		t.Errorf("MinUtilization = %f, want 0.3", decoded.MinUtilization)
	}
}

func TestBusConfigCommand(t *testing.T) {
	dropSlow := true
	cmd := BusConfigCommand{
		DropSlow:  &dropSlow,
		Reason:    "Enable backpressure protection",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeBusConfig, cmd)

	var decoded BusConfigCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.DropSlow == nil || *decoded.DropSlow != true {
		t.Errorf("DropSlow = %v, want true", decoded.DropSlow)
	}
}

func TestForceGCCommand(t *testing.T) {
	cmd := ForceGCCommand{
		Reason:    "Memory pressure at 95%",
		Timestamp: time.Now(),
	}

	evt := NewControlEvent(EventTypeForceGC, cmd)

	var decoded ForceGCCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Reason != "Memory pressure at 95%" {
		t.Errorf("Reason = %s, want 'Memory pressure at 95%%'", decoded.Reason)
	}
}

func TestControlEvent_DefaultSource(t *testing.T) {
	cmd := GovernorScaleCommand{Scale: 0.5}
	evt := NewControlEvent(EventTypeGovernorScale, cmd)

	if evt.Source != "control-lab" {
		t.Errorf("Default source = %s, want 'control-lab'", evt.Source)
	}
}

func TestControlEvent_OverrideSource(t *testing.T) {
	cmd := GovernorScaleCommand{Scale: 0.5}
	evt := NewControlEvent(EventTypeGovernorScale, cmd)
	evt.SetSource("manual-override")

	if evt.Source != "manual-override" {
		t.Errorf("Source = %s, want 'manual-override'", evt.Source)
	}
}

func TestControlEvent_HasID(t *testing.T) {
	cmd := GovernorScaleCommand{Scale: 0.5}
	evt := NewControlEvent(EventTypeGovernorScale, cmd)

	if evt.ID == "" {
		t.Error("Event ID is empty")
	}
}

func TestControlEvent_HasTimestamp(t *testing.T) {
	before := time.Now()
	cmd := GovernorScaleCommand{Scale: 0.5}
	evt := NewControlEvent(EventTypeGovernorScale, cmd)
	after := time.Now()

	if evt.Timestamp.Before(before) || evt.Timestamp.After(after) {
		t.Errorf("Event timestamp %v not between %v and %v", evt.Timestamp, before, after)
	}
}

func TestGovernorScaleCommand_WithMetadata(t *testing.T) {
	cmd := GovernorScaleCommand{
		Scale:  0.5,
		Reason: "Test",
		Metadata: map[string]any{
			"pressure":  0.87,
			"threshold": 0.70,
		},
	}

	evt := NewControlEvent(EventTypeGovernorScale, cmd)

	var decoded GovernorScaleCommand
	if err := evt.DecodePayload(&decoded, JSONCodec{}); err != nil {
		t.Fatalf("DecodePayload failed: %v", err)
	}

	if decoded.Metadata == nil {
		t.Fatal("Metadata is nil")
	}

	if pressure, ok := decoded.Metadata["pressure"].(float64); !ok || pressure != 0.87 {
		t.Errorf("Metadata pressure = %v, want 0.87", decoded.Metadata["pressure"])
	}
}
