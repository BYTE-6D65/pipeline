package wheel

import (
	"sync"
	"testing"
)

func TestWheel_BasicOperations(t *testing.T) {
	w := NewWheel(WheelConfig{Capacity: 16})

	if !w.IsEmpty() {
		t.Error("New wheel should be empty")
	}

	// Enqueue a bucket
	var b Bucket
	b.SetTimestamp(12345)
	b.SetUID(1)
	b.SetTypeCode(TypeHardwareToNetwork)
	b.SetPayload([]byte("test"))

	if !w.Enqueue(&b) {
		t.Fatal("Enqueue failed")
	}

	if w.IsEmpty() {
		t.Error("Wheel should not be empty after enqueue")
	}

	if w.Len() != 1 {
		t.Errorf("Len = %d, want 1", w.Len())
	}

	// Dequeue (unsafe API - must copy and clear)
	result := w.Dequeue()
	if result == nil {
		t.Fatal("Dequeue returned nil")
	}

	// Copy immediately
	var copy Bucket
	result.CopyTo(&copy)
	result.Clear()

	if copy.Timestamp() != 12345 {
		t.Errorf("Timestamp = %d, want 12345", copy.Timestamp())
	}

	if !w.IsEmpty() {
		t.Error("Wheel should be empty after dequeue")
	}
}

func TestWheel_CapacityEnforcement(t *testing.T) {
	w := NewWheel(WheelConfig{Capacity: 4})

	var b Bucket
	b.SetTimestamp(12345)
	b.SetTypeCode(TypeHardwareToNetwork)

	// Fill to capacity
	for i := 0; i < 4; i++ {
		b.SetUID(uint32(i))
		if !w.Enqueue(&b) {
			t.Fatalf("Enqueue %d failed", i)
		}
	}

	if !w.IsFull() {
		t.Error("Wheel should be full")
	}

	// Try to overflow
	b.SetUID(999)
	if w.Enqueue(&b) {
		t.Error("Should not be able to enqueue when full")
	}

	stats := w.Stats()
	if stats.DropCount != 1 {
		t.Errorf("DropCount = %d, want 1", stats.DropCount)
	}
}

func TestWheel_TryDequeue(t *testing.T) {
	w := NewWheel(WheelConfig{Capacity: 16})

	var src Bucket
	src.SetTimestamp(99999)
	src.SetUID(42)
	src.SetTypeCode(TypeMQTTToNetwork)
	src.SetPayload([]byte("mqtt message"))

	w.Enqueue(&src)

	var dst Bucket
	if !w.TryDequeue(&dst) {
		t.Fatal("TryDequeue failed")
	}

	if dst.Timestamp() != 99999 {
		t.Error("Timestamp not copied")
	}
	if dst.UID() != 42 {
		t.Error("UID not copied")
	}
	if string(dst.Payload()) != "mqtt message" {
		t.Error("Payload not copied")
	}
}

func TestWheel_ConcurrentEnqueueDequeue(t *testing.T) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	const numProducers = 4
	const numConsumers = 4
	const packetsPerProducer = 1000

	var wg sync.WaitGroup

	// Start producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(producerID int) {
			defer wg.Done()
			for i := 0; i < packetsPerProducer; i++ {
				var b Bucket
				b.SetTimestamp(uint64(i))
				b.SetUID(uint32(producerID*1000 + i))
				b.SetTypeCode(TypeHardwareToNetwork)
				b.SetPayload([]byte("test"))

				// Retry until enqueue succeeds
				for !w.Enqueue(&b) {
					// Wheel full, retry
				}
			}
		}(p)
	}

	// Start consumers
	consumed := make([]int, numConsumers)
	for c := 0; c < numConsumers; c++ {
		wg.Add(1)
		go func(consumerID int) {
			defer wg.Done()
			var b Bucket
			count := 0
			expectedTotal := numProducers * packetsPerProducer

			for count < expectedTotal/numConsumers {
				if w.TryDequeue(&b) {
					count++
				}
			}
			consumed[consumerID] = count
		}(c)
	}

	wg.Wait()

	totalConsumed := 0
	for _, c := range consumed {
		totalConsumed += c
	}

	expected := numProducers * packetsPerProducer
	if totalConsumed != expected {
		t.Errorf("Consumed %d packets, want %d", totalConsumed, expected)
	}

	if !w.IsEmpty() {
		t.Errorf("Wheel should be empty, has %d packets", w.Len())
	}
}

func TestWheel_BackoffSignal(t *testing.T) {
	w := NewWheel(WheelConfig{Capacity: 100})

	var b Bucket
	b.SetTimestamp(12345)
	b.SetTypeCode(TypeHardwareToNetwork)

	// Fill to 85% (should trigger backoff)
	for i := 0; i < 85; i++ {
		b.SetUID(uint32(i))
		w.Enqueue(&b)
	}

	if !w.ShouldBackoff() {
		t.Error("Should signal backoff at >80% usage")
	}

	usage := w.Usage()
	if usage < 0.8 {
		t.Errorf("Usage = %.2f, want >0.80", usage)
	}
}

func BenchmarkWheel_Enqueue(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	var bucket Bucket
	bucket.SetTimestamp(12345)
	bucket.SetUID(1)
	bucket.SetTypeCode(TypeHardwareToNetwork)
	bucket.SetPayload([]byte("benchmark"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !w.Enqueue(&bucket) {
			// Dequeue to make space
			w.Dequeue()
			w.Enqueue(&bucket)
		}
	}
}

func BenchmarkWheel_Dequeue(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	// Pre-fill
	var bucket Bucket
	bucket.SetTimestamp(12345)
	bucket.SetTypeCode(TypeHardwareToNetwork)
	for i := 0; i < 1024; i++ {
		bucket.SetUID(uint32(i))
		w.Enqueue(&bucket)
	}

	b.ResetTimer()
	idx := 0
	for i := 0; i < b.N; i++ {
		result := w.Dequeue()
		if result == nil {
			// Refill
			for j := 0; j < 1024; j++ {
				bucket.SetUID(uint32(idx))
				idx++
				w.Enqueue(&bucket)
			}
			result = w.Dequeue()
		}
		if result != nil {
			result.Clear()
		}
	}
}

func BenchmarkWheel_TryDequeue(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	// Pre-fill
	var src Bucket
	src.SetTimestamp(12345)
	src.SetTypeCode(TypeHardwareToNetwork)
	src.SetPayload([]byte("benchmark data"))
	for i := 0; i < 1024; i++ {
		src.SetUID(uint32(i))
		w.Enqueue(&src)
	}

	var dst Bucket
	b.ResetTimer()
	idx := 0
	for i := 0; i < b.N; i++ {
		if !w.TryDequeue(&dst) {
			// Refill
			for j := 0; j < 1024; j++ {
				src.SetUID(uint32(idx))
				idx++
				w.Enqueue(&src)
			}
			w.TryDequeue(&dst)
		}
	}
}
