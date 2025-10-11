package statemachine

import (
	"context"
	"fmt"
	"sync"
)

// State represents a state in the state machine.
type State string

// Event represents an event that can trigger a state transition.
type Event string

// GuardFunc is a function that determines if a transition should be allowed.
// Returns true if the transition should proceed, false otherwise.
type GuardFunc func(ctx context.Context, from State, to State, event Event) bool

// ActionFunc is a function executed during a transition.
type ActionFunc func(ctx context.Context, from State, to State, event Event) error

// HookFunc is called when a state is entered or exited.
type HookFunc func(ctx context.Context, state State) error

// StateConfig defines the configuration for a state.
type StateConfig struct {
	// Name is the unique identifier for this state
	Name State

	// OnEnter is called when entering this state
	OnEnter HookFunc

	// OnExit is called when exiting this state
	OnExit HookFunc
}

// Transition defines a state transition.
type Transition struct {
	// From is the source state
	From State

	// To is the destination state
	To State

	// Event is the event that triggers this transition
	Event Event

	// Guard determines if the transition should be allowed
	Guard GuardFunc

	// Action is executed during the transition (after OnExit, before OnEnter)
	Action ActionFunc
}

// TransitionHook is called whenever a transition occurs.
type TransitionHook func(ctx context.Context, from State, to State, event Event)

// Machine is a finite state machine implementation.
type Machine struct {
	mu          sync.RWMutex
	current     State
	states      map[State]StateConfig
	transitions map[State]map[Event]Transition
	hooks       []TransitionHook
}

// NewMachine creates a new state machine with the given initial state.
func NewMachine(initialState State) *Machine {
	return &Machine{
		current:     initialState,
		states:      make(map[State]StateConfig),
		transitions: make(map[State]map[Event]Transition),
		hooks:       make([]TransitionHook, 0),
	}
}

// AddState registers a state configuration.
func (m *Machine) AddState(config StateConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[config.Name] = config
}

// AddTransition registers a state transition.
func (m *Machine) AddTransition(trans Transition) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize transitions map for source state if needed
	if m.transitions[trans.From] == nil {
		m.transitions[trans.From] = make(map[Event]Transition)
	}

	// Check for duplicate transition
	if _, exists := m.transitions[trans.From][trans.Event]; exists {
		return fmt.Errorf("transition from %s on event %s already exists", trans.From, trans.Event)
	}

	m.transitions[trans.From][trans.Event] = trans
	return nil
}

// Trigger attempts to trigger an event and transition to a new state.
func (m *Machine) Trigger(ctx context.Context, event Event) error {
	m.mu.Lock()
	currentState := m.current
	m.mu.Unlock()

	// Find the transition
	m.mu.RLock()
	stateTransitions, ok := m.transitions[currentState]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("no transitions from state %s", currentState)
	}

	trans, ok := stateTransitions[event]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("no transition from %s on event %s", currentState, event)
	}
	m.mu.RUnlock()

	// Check guard
	if trans.Guard != nil {
		if !trans.Guard(ctx, trans.From, trans.To, event) {
			return fmt.Errorf("guard rejected transition from %s to %s on event %s", trans.From, trans.To, event)
		}
	}

	// Execute transition
	return m.executeTransition(ctx, trans)
}

// executeTransition performs the actual state transition.
func (m *Machine) executeTransition(ctx context.Context, trans Transition) error {
	// Call OnExit hook for current state
	m.mu.RLock()
	fromConfig, hasFromConfig := m.states[trans.From]
	toConfig, hasToConfig := m.states[trans.To]
	m.mu.RUnlock()

	if hasFromConfig && fromConfig.OnExit != nil {
		if err := fromConfig.OnExit(ctx, trans.From); err != nil {
			return fmt.Errorf("OnExit failed for state %s: %w", trans.From, err)
		}
	}

	// Execute transition action
	if trans.Action != nil {
		if err := trans.Action(ctx, trans.From, trans.To, trans.Event); err != nil {
			return fmt.Errorf("action failed for transition %s -> %s: %w", trans.From, trans.To, err)
		}
	}

	// Update current state
	m.mu.Lock()
	m.current = trans.To
	m.mu.Unlock()

	// Call OnEnter hook for new state
	if hasToConfig && toConfig.OnEnter != nil {
		if err := toConfig.OnEnter(ctx, trans.To); err != nil {
			// State was already changed, but OnEnter failed
			return fmt.Errorf("OnEnter failed for state %s: %w", trans.To, err)
		}
	}

	// Notify transition hooks
	m.mu.RLock()
	hooks := m.hooks
	m.mu.RUnlock()

	for _, hook := range hooks {
		hook(ctx, trans.From, trans.To, trans.Event)
	}

	return nil
}

// Current returns the current state.
func (m *Machine) Current() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Can checks if an event can be triggered from the current state.
func (m *Machine) Can(event Event) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateTransitions, ok := m.transitions[m.current]
	if !ok {
		return false
	}

	_, ok = stateTransitions[event]
	return ok
}

// OnTransition registers a hook that is called on every transition.
func (m *Machine) OnTransition(hook TransitionHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook)
}

// States returns all registered states.
func (m *Machine) States() []State {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]State, 0, len(m.states))
	for state := range m.states {
		states = append(states, state)
	}
	return states
}

// AvailableEvents returns all events that can be triggered from the current state.
func (m *Machine) AvailableEvents() []Event {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateTransitions, ok := m.transitions[m.current]
	if !ok {
		return []Event{}
	}

	events := make([]Event, 0, len(stateTransitions))
	for event := range stateTransitions {
		events = append(events, event)
	}
	return events
}
