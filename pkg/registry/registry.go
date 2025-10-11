package registry

import "sync"

// Entry represents a key-value pair in the registry.
type Entry struct {
	Key   string
	Value any
}

// Registry provides thread-safe key-value storage with generic value types.
type Registry interface {
	// Set stores a value with the given key
	Set(key string, value any)

	// Get retrieves a value by key, returns (value, true) if found
	Get(key string) (any, bool)

	// List returns all entries in the registry
	List() []Entry

	// Delete removes an entry by key
	Delete(key string)

	// Clear removes all entries
	Clear()

	// Has checks if a key exists
	Has(key string) bool

	// Keys returns all keys in the registry
	Keys() []string
}

// InMemoryRegistry is a thread-safe in-memory implementation of Registry.
type InMemoryRegistry struct {
	mu    sync.RWMutex
	items map[string]any
}

// NewInMemoryRegistry creates a new in-memory registry.
func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		items: make(map[string]any),
	}
}

// Set stores a value with the given key.
func (r *InMemoryRegistry) Set(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[key] = value
}

// Get retrieves a value by key.
func (r *InMemoryRegistry) Get(key string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.items[key]
	return value, ok
}

// List returns all entries in the registry.
func (r *InMemoryRegistry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]Entry, 0, len(r.items))
	for key, value := range r.items {
		entries = append(entries, Entry{Key: key, Value: value})
	}
	return entries
}

// Delete removes an entry by key.
func (r *InMemoryRegistry) Delete(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, key)
}

// Clear removes all entries.
func (r *InMemoryRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = make(map[string]any)
}

// Has checks if a key exists.
func (r *InMemoryRegistry) Has(key string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[key]
	return ok
}

// Keys returns all keys in the registry.
func (r *InMemoryRegistry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0, len(r.items))
	for key := range r.items {
		keys = append(keys, key)
	}
	return keys
}

// TypedRegistry provides type-safe access to a registry with a specific value type.
type TypedRegistry[T any] struct {
	registry Registry
}

// NewTypedRegistry creates a new typed registry wrapper.
func NewTypedRegistry[T any](registry Registry) *TypedRegistry[T] {
	return &TypedRegistry[T]{registry: registry}
}

// Set stores a typed value.
func (t *TypedRegistry[T]) Set(key string, value T) {
	t.registry.Set(key, value)
}

// Get retrieves a typed value.
func (t *TypedRegistry[T]) Get(key string) (T, bool) {
	value, ok := t.registry.Get(key)
	if !ok {
		var zero T
		return zero, false
	}
	typed, ok := value.(T)
	if !ok {
		var zero T
		return zero, false
	}
	return typed, true
}

// List returns all entries with typed values.
func (t *TypedRegistry[T]) List() []TypedEntry[T] {
	entries := t.registry.List()
	result := make([]TypedEntry[T], 0, len(entries))
	for _, entry := range entries {
		if typed, ok := entry.Value.(T); ok {
			result = append(result, TypedEntry[T]{
				Key:   entry.Key,
				Value: typed,
			})
		}
	}
	return result
}

// Delete removes an entry.
func (t *TypedRegistry[T]) Delete(key string) {
	t.registry.Delete(key)
}

// Clear removes all entries.
func (t *TypedRegistry[T]) Clear() {
	t.registry.Clear()
}

// Has checks if a key exists.
func (t *TypedRegistry[T]) Has(key string) bool {
	return t.registry.Has(key)
}

// Keys returns all keys.
func (t *TypedRegistry[T]) Keys() []string {
	return t.registry.Keys()
}

// TypedEntry represents a key-value pair with a typed value.
type TypedEntry[T any] struct {
	Key   string
	Value T
}
