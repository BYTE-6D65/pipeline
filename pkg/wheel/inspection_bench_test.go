package wheel

import (
	"testing"
)

// BenchmarkWheel_InspectionGate_HighThroughput simulates realistic high-throughput scenario
// where buckets are consumed quickly (90% clean on enqueue)
func BenchmarkWheel_InspectionGate_HighThroughput(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	var src Bucket
	src.SetTimestamp(12345)
	src.SetTypeCode(TypeHardwareToNetwork)
	src.SetPayload([]byte("benchmark"))

	var dst Bucket

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Enqueue
		w.Enqueue(&src)

		// Consume immediately (simulates high throughput)
		// This makes bucket clean for next enqueue
		w.TryDequeue(&dst)
	}
}

// BenchmarkWheel_InspectionGate_LowThroughput simulates low-throughput scenario
// where buckets sit in ring and become dirty (100% dirty on enqueue)
func BenchmarkWheel_InspectionGate_LowThroughput(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	var src Bucket
	src.SetTimestamp(12345)
	src.SetTypeCode(TypeHardwareToNetwork)
	src.SetPayload([]byte("benchmark"))

	// Pre-fill entire ring (all slots dirty)
	for i := 0; i < 1024; i++ {
		src.SetUID(uint32(i))
		w.Enqueue(&src)
	}

	// Consume all to make them dirty
	var dst Bucket
	for i := 0; i < 1024; i++ {
		w.TryDequeue(&dst)
	}

	b.ResetTimer()
	// Now enqueue hits dirty slots every time
	for i := 0; i < b.N; i++ {
		src.SetUID(uint32(i))
		w.Enqueue(&src)
	}
}

// BenchmarkWheel_InspectionGate_MixedLoad simulates 50/50 clean/dirty
func BenchmarkWheel_InspectionGate_MixedLoad(b *testing.B) {
	w := NewWheel(WheelConfig{Capacity: 1024})

	var src Bucket
	src.SetTimestamp(12345)
	src.SetTypeCode(TypeHardwareToNetwork)
	src.SetPayload([]byte("benchmark"))

	// Pre-fill half the ring
	for i := 0; i < 512; i++ {
		src.SetUID(uint32(i))
		w.Enqueue(&src)
	}

	// Consume half to make them dirty
	var dst Bucket
	for i := 0; i < 512; i++ {
		w.TryDequeue(&dst)
	}

	b.ResetTimer()
	// Alternates between clean and dirty slots
	for i := 0; i < b.N; i++ {
		src.SetUID(uint32(i))
		w.Enqueue(&src)
	}
}

// BenchmarkWheel_TryDequeue_NoWipe measures dequeue without clearing
func BenchmarkWheel_TryDequeue_NoWipe(b *testing.B) {
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
		}
	}
}
