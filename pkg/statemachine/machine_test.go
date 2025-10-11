package statemachine

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestNewMachine(t *testing.T) {
	m := NewMachine("idle")
	if m == nil {
		t.Fatal("NewMachine returned nil")
	}

	if m.Current() != "idle" {
		t.Errorf("Expected initial state 'idle', got %s", m.Current())
	}
}

func TestMachine_AddState(t *testing.T) {
	m := NewMachine("idle")

	config := StateConfig{
		Name: "running",
	}
	m.AddState(config)

	states := m.States()
	found := false
	for _, s := range states {
		if s == "running" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected state 'running' to be registered")
	}
}

func TestMachine_AddTransition(t *testing.T) {
	m := NewMachine("idle")

	trans := Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	}

	if err := m.AddTransition(trans); err != nil {
		t.Fatalf("AddTransition failed: %v", err)
	}

	if !m.Can("start") {
		t.Error("Expected event 'start' to be available")
	}
}

func TestMachine_AddTransition_Duplicate(t *testing.T) {
	m := NewMachine("idle")

	trans := Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	}

	m.AddTransition(trans)

	// Try to add duplicate
	err := m.AddTransition(trans)
	if err == nil {
		t.Error("Expected error when adding duplicate transition")
	}
}

func TestMachine_Trigger_Success(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	if err := m.Trigger(ctx, "start"); err != nil {
		t.Fatalf("Trigger failed: %v", err)
	}

	if m.Current() != "running" {
		t.Errorf("Expected state 'running', got %s", m.Current())
	}
}

func TestMachine_Trigger_NoTransition(t *testing.T) {
	m := NewMachine("idle")

	ctx := context.Background()
	err := m.Trigger(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent transition")
	}
}

func TestMachine_Trigger_InvalidEvent(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	err := m.Trigger(ctx, "stop") // Wrong event
	if err == nil {
		t.Error("Expected error for invalid event")
	}
}

func TestMachine_OnEnterOnExit(t *testing.T) {
	m := NewMachine("idle")

	var exitCalled bool
	var enterCalled bool

	m.AddState(StateConfig{
		Name: "idle",
		OnExit: func(ctx context.Context, state State) error {
			exitCalled = true
			return nil
		},
	})

	m.AddState(StateConfig{
		Name: "running",
		OnEnter: func(ctx context.Context, state State) error {
			enterCalled = true
			return nil
		},
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	m.Trigger(ctx, "start")

	if !exitCalled {
		t.Error("Expected OnExit to be called")
	}

	if !enterCalled {
		t.Error("Expected OnEnter to be called")
	}
}

func TestMachine_ActionFunc(t *testing.T) {
	m := NewMachine("idle")

	var actionCalled bool

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
		Action: func(ctx context.Context, from State, to State, event Event) error {
			actionCalled = true
			if from != "idle" || to != "running" || event != "start" {
				t.Errorf("Unexpected values: from=%s, to=%s, event=%s", from, to, event)
			}
			return nil
		},
	})

	ctx := context.Background()
	m.Trigger(ctx, "start")

	if !actionCalled {
		t.Error("Expected Action to be called")
	}
}

func TestMachine_GuardFunc_Allow(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
		Guard: func(ctx context.Context, from State, to State, event Event) bool {
			return true // Allow transition
		},
	})

	ctx := context.Background()
	if err := m.Trigger(ctx, "start"); err != nil {
		t.Fatalf("Trigger failed: %v", err)
	}

	if m.Current() != "running" {
		t.Error("Expected transition to succeed with guard allowing it")
	}
}

func TestMachine_GuardFunc_Reject(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
		Guard: func(ctx context.Context, from State, to State, event Event) bool {
			return false // Reject transition
		},
	})

	ctx := context.Background()
	err := m.Trigger(ctx, "start")
	if err == nil {
		t.Error("Expected guard to reject transition")
	}

	if m.Current() != "idle" {
		t.Error("Expected state to remain 'idle' when guard rejects")
	}
}

func TestMachine_OnTransitionHook(t *testing.T) {
	m := NewMachine("idle")

	var hookCalled bool
	var hookFrom, hookTo State
	var hookEvent Event

	m.OnTransition(func(ctx context.Context, from State, to State, event Event) {
		hookCalled = true
		hookFrom = from
		hookTo = to
		hookEvent = event
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	m.Trigger(ctx, "start")

	if !hookCalled {
		t.Error("Expected transition hook to be called")
	}

	if hookFrom != "idle" || hookTo != "running" || hookEvent != "start" {
		t.Errorf("Hook received wrong values: from=%s, to=%s, event=%s", hookFrom, hookTo, hookEvent)
	}
}

func TestMachine_Can(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	if !m.Can("start") {
		t.Error("Expected Can('start') to return true")
	}

	if m.Can("stop") {
		t.Error("Expected Can('stop') to return false")
	}
}

func TestMachine_AvailableEvents(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{From: "idle", To: "running", Event: "start"})
	m.AddTransition(Transition{From: "idle", To: "paused", Event: "pause"})

	events := m.AvailableEvents()
	if len(events) != 2 {
		t.Errorf("Expected 2 available events, got %d", len(events))
	}

	eventMap := make(map[Event]bool)
	for _, e := range events {
		eventMap[e] = true
	}

	if !eventMap["start"] || !eventMap["pause"] {
		t.Error("Expected both 'start' and 'pause' to be available")
	}
}

func TestMachine_ExecutionOrder(t *testing.T) {
	m := NewMachine("idle")

	var order []string

	m.AddState(StateConfig{
		Name: "idle",
		OnExit: func(ctx context.Context, state State) error {
			order = append(order, "exit-idle")
			return nil
		},
	})

	m.AddState(StateConfig{
		Name: "running",
		OnEnter: func(ctx context.Context, state State) error {
			order = append(order, "enter-running")
			return nil
		},
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
		Action: func(ctx context.Context, from State, to State, event Event) error {
			order = append(order, "action")
			return nil
		},
	})

	ctx := context.Background()
	m.Trigger(ctx, "start")

	if len(order) != 3 {
		t.Fatalf("Expected 3 steps, got %d: %v", len(order), order)
	}

	if order[0] != "exit-idle" {
		t.Error("Expected OnExit to be called first")
	}

	if order[1] != "action" {
		t.Error("Expected Action to be called second")
	}

	if order[2] != "enter-running" {
		t.Error("Expected OnEnter to be called third")
	}
}

func TestMachine_OnExitError(t *testing.T) {
	m := NewMachine("idle")

	m.AddState(StateConfig{
		Name: "idle",
		OnExit: func(ctx context.Context, state State) error {
			return errors.New("exit error")
		},
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	err := m.Trigger(ctx, "start")
	if err == nil {
		t.Error("Expected error from OnExit")
	}

	// State should not have changed
	if m.Current() != "idle" {
		t.Error("Expected state to remain 'idle' after OnExit error")
	}
}

func TestMachine_ActionError(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
		Action: func(ctx context.Context, from State, to State, event Event) error {
			return errors.New("action error")
		},
	})

	ctx := context.Background()
	err := m.Trigger(ctx, "start")
	if err == nil {
		t.Error("Expected error from Action")
	}

	// State should not have changed
	if m.Current() != "idle" {
		t.Error("Expected state to remain 'idle' after Action error")
	}
}

func TestMachine_OnEnterError(t *testing.T) {
	m := NewMachine("idle")

	m.AddState(StateConfig{
		Name: "running",
		OnEnter: func(ctx context.Context, state State) error {
			return errors.New("enter error")
		},
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	err := m.Trigger(ctx, "start")
	if err == nil {
		t.Error("Expected error from OnEnter")
	}

	// Note: State HAS changed even though OnEnter failed
	// This is by design - the transition already occurred
	if m.Current() != "running" {
		t.Error("Expected state to be 'running' even after OnEnter error")
	}
}

func TestMachine_ConcurrentAccess(t *testing.T) {
	m := NewMachine("idle")

	m.AddTransition(Transition{From: "idle", To: "running", Event: "start"})
	m.AddTransition(Transition{From: "running", To: "idle", Event: "stop"})

	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	ctx := context.Background()

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = m.Current()
				_ = m.Can("start")
				_ = m.AvailableEvents()
			}
		}()
	}

	// Concurrent state changes
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				current := m.Current()
				if current == "idle" {
					m.Trigger(ctx, "start")
				} else if current == "running" {
					m.Trigger(ctx, "stop")
				}
			}
		}()
	}

	wg.Wait()
}

func TestMachine_MultipleTransitionHooks(t *testing.T) {
	m := NewMachine("idle")

	var hook1Called, hook2Called bool

	m.OnTransition(func(ctx context.Context, from State, to State, event Event) {
		hook1Called = true
	})

	m.OnTransition(func(ctx context.Context, from State, to State, event Event) {
		hook2Called = true
	})

	m.AddTransition(Transition{
		From:  "idle",
		To:    "running",
		Event: "start",
	})

	ctx := context.Background()
	m.Trigger(ctx, "start")

	if !hook1Called || !hook2Called {
		t.Error("Expected both transition hooks to be called")
	}
}
