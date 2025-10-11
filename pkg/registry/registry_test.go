package registry

import (
	"sync"
	"testing"
)

func TestNewInMemoryRegistry(t *testing.T) {
	reg := NewInMemoryRegistry()
	if reg == nil {
		t.Fatal("NewInMemoryRegistry returned nil")
	}

	if reg.items == nil {
		t.Error("Registry items map should be initialized")
	}
}

func TestRegistry_Set_Get(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Set("key2", 42)

	val, ok := reg.Get("key1")
	if !ok {
		t.Fatal("Expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}

	val, ok = reg.Get("key2")
	if !ok {
		t.Fatal("Expected key2 to exist")
	}
	if val != 42 {
		t.Errorf("Expected 42, got %v", val)
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	reg := NewInMemoryRegistry()

	_, ok := reg.Get("nonexistent")
	if ok {
		t.Error("Expected Get to return false for nonexistent key")
	}
}

func TestRegistry_Has(t *testing.T) {
	reg := NewInMemoryRegistry()

	if reg.Has("key1") {
		t.Error("Expected Has to return false for nonexistent key")
	}

	reg.Set("key1", "value1")

	if !reg.Has("key1") {
		t.Error("Expected Has to return true for existing key")
	}
}

func TestRegistry_Delete(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Delete("key1")

	if reg.Has("key1") {
		t.Error("Expected key1 to be deleted")
	}

	// Deleting nonexistent key should not error
	reg.Delete("nonexistent")
}

func TestRegistry_Clear(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Set("key2", "value2")
	reg.Set("key3", "value3")

	reg.Clear()

	if reg.Has("key1") || reg.Has("key2") || reg.Has("key3") {
		t.Error("Expected all keys to be cleared")
	}

	if len(reg.Keys()) != 0 {
		t.Errorf("Expected 0 keys after clear, got %d", len(reg.Keys()))
	}
}

func TestRegistry_Keys(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Set("key2", "value2")
	reg.Set("key3", "value3")

	keys := reg.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	keyMap := make(map[string]bool)
	for _, key := range keys {
		keyMap[key] = true
	}

	if !keyMap["key1"] || !keyMap["key2"] || !keyMap["key3"] {
		t.Error("Expected all keys to be present")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Set("key2", 42)
	reg.Set("key3", true)

	entries := reg.List()
	if len(entries) != 3 {
		t.Errorf("Expected 3 entries, got %d", len(entries))
	}

	entryMap := make(map[string]any)
	for _, entry := range entries {
		entryMap[entry.Key] = entry.Value
	}

	if entryMap["key1"] != "value1" {
		t.Error("key1 value mismatch")
	}
	if entryMap["key2"] != 42 {
		t.Error("key2 value mismatch")
	}
	if entryMap["key3"] != true {
		t.Error("key3 value mismatch")
	}
}

func TestRegistry_Update(t *testing.T) {
	reg := NewInMemoryRegistry()

	reg.Set("key1", "value1")
	reg.Set("key1", "value2")

	val, ok := reg.Get("key1")
	if !ok {
		t.Fatal("Expected key1 to exist")
	}
	if val != "value2" {
		t.Errorf("Expected 'value2', got %v", val)
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewInMemoryRegistry()
	const numGoroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := string(rune('a' + (id % 26)))
				reg.Set(key, id*opsPerGoroutine+j)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := string(rune('a' + (id % 26)))
				reg.Get(key)
				reg.Has(key)
				reg.Keys()
			}
		}(i)
	}

	wg.Wait()
}

func TestTypedRegistry_String(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	typed.Set("key1", "value1")
	typed.Set("key2", "value2")

	val, ok := typed.Get("key1")
	if !ok {
		t.Fatal("Expected key1 to exist")
	}
	if val != "value1" {
		t.Errorf("Expected 'value1', got %v", val)
	}
}

func TestTypedRegistry_Int(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[int](reg)

	typed.Set("count", 42)

	val, ok := typed.Get("count")
	if !ok {
		t.Fatal("Expected count to exist")
	}
	if val != 42 {
		t.Errorf("Expected 42, got %v", val)
	}
}

func TestTypedRegistry_Struct(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[Person](reg)

	person := Person{Name: "Alice", Age: 30}
	typed.Set("person1", person)

	retrieved, ok := typed.Get("person1")
	if !ok {
		t.Fatal("Expected person1 to exist")
	}
	if retrieved.Name != "Alice" || retrieved.Age != 30 {
		t.Errorf("Person mismatch: got %+v", retrieved)
	}
}

func TestTypedRegistry_TypeMismatch(t *testing.T) {
	reg := NewInMemoryRegistry()

	// Set as string
	reg.Set("key1", "string value")

	// Try to get as int
	typedInt := NewTypedRegistry[int](reg)
	_, ok := typedInt.Get("key1")
	if ok {
		t.Error("Expected Get to return false for type mismatch")
	}
}

func TestTypedRegistry_List(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	typed.Set("key1", "value1")
	typed.Set("key2", "value2")

	// Add a non-string value to the underlying registry
	reg.Set("key3", 42)

	entries := typed.List()
	if len(entries) != 2 {
		t.Errorf("Expected 2 string entries, got %d", len(entries))
	}

	found := make(map[string]string)
	for _, entry := range entries {
		found[entry.Key] = entry.Value
	}

	if found["key1"] != "value1" {
		t.Error("key1 value mismatch")
	}
	if found["key2"] != "value2" {
		t.Error("key2 value mismatch")
	}
}

func TestTypedRegistry_Delete(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	typed.Set("key1", "value1")
	typed.Delete("key1")

	if typed.Has("key1") {
		t.Error("Expected key1 to be deleted")
	}
}

func TestTypedRegistry_Clear(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	typed.Set("key1", "value1")
	typed.Set("key2", "value2")

	typed.Clear()

	if typed.Has("key1") || typed.Has("key2") {
		t.Error("Expected all keys to be cleared")
	}
}

func TestTypedRegistry_Keys(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	typed.Set("key1", "value1")
	typed.Set("key2", "value2")

	keys := typed.Keys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestTypedRegistry_Has(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	if typed.Has("key1") {
		t.Error("Expected Has to return false for nonexistent key")
	}

	typed.Set("key1", "value1")

	if !typed.Has("key1") {
		t.Error("Expected Has to return true for existing key")
	}
}

func TestTypedRegistry_NotFound(t *testing.T) {
	reg := NewInMemoryRegistry()
	typed := NewTypedRegistry[string](reg)

	_, ok := typed.Get("nonexistent")
	if ok {
		t.Error("Expected Get to return false for nonexistent key")
	}
}
