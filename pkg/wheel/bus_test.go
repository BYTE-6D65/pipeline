package wheel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWheelBus_CreateWheel(t *testing.T) {
	bus := NewWheelBus()

	// Create wheel for hardware→network
	wheel, err := bus.CreateWheel(TypeHardwareToNetwork, 256)
	if err != nil {
		t.Fatalf("CreateWheel failed: %v", err)
	}

	if wheel == nil {
		t.Fatal("Wheel is nil")
	}

	if wheel.Cap() != 256 {
		t.Errorf("Capacity = %d, want 256", wheel.Cap())
	}

	// Verify wheel exists in bus
	wheels := bus.ListWheels()
	if len(wheels) != 1 {
		t.Errorf("ListWheels = %d, want 1", len(wheels))
	}

	if wheels[0] != TypeHardwareToNetwork {
		t.Errorf("Wheel type = 0x%04x, want 0x%04x", wheels[0], TypeHardwareToNetwork)
	}
}

func TestWheelBus_PublishConsume(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 256)

	// Publish bucket
	var b Bucket
	b.SetTimestamp(12345)
	b.SetUID(42)
	b.SetTypeCode(TypeHardwareToNetwork)
	b.SetPayload([]byte("hardware event"))

	if err := bus.Publish(&b); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Consume bucket (unsafe API - must copy immediately)
	result := bus.Consume(TypeHardwareToNetwork)
	if result == nil {
		t.Fatal("Consume returned nil")
	}

	// Copy data immediately
	var copy Bucket
	result.CopyTo(&copy)
	result.Clear()

	if copy.UID() != 42 {
		t.Errorf("UID = %d, want 42", copy.UID())
	}

	if string(copy.Payload()) != "hardware event" {
		t.Errorf("Payload = %q, want %q", copy.Payload(), "hardware event")
	}
}

func TestWheelBus_RouteToCorrectWheel(t *testing.T) {
	bus := NewWheelBus()

	// Create multiple wheels (lanes)
	bus.CreateWheel(TypeHardwareToNetwork, 256)
	bus.CreateWheel(TypeMQTTToNetwork, 256)
	bus.CreateWheel(TypeSSH3ToNetwork, 256)

	// Publish to different lanes
	var b1, b2, b3 Bucket

	b1.SetTimestamp(1)
	b1.SetUID(1)
	b1.SetTypeCode(TypeHardwareToNetwork)
	b1.SetPayload([]byte("hardware"))

	b2.SetTimestamp(2)
	b2.SetUID(2)
	b2.SetTypeCode(TypeMQTTToNetwork)
	b2.SetPayload([]byte("mqtt"))

	b3.SetTimestamp(3)
	b3.SetUID(3)
	b3.SetTypeCode(TypeSSH3ToNetwork)
	b3.SetPayload([]byte("ssh3"))

	bus.Publish(&b1)
	bus.Publish(&b2)
	bus.Publish(&b3)

	// Verify routing (using safe TryConsume API)
	var hw, mqtt, ssh Bucket

	if !bus.TryConsume(TypeHardwareToNetwork, &hw) || hw.UID() != 1 {
		t.Error("Hardware packet not routed correctly")
	}

	if !bus.TryConsume(TypeMQTTToNetwork, &mqtt) || mqtt.UID() != 2 {
		t.Error("MQTT packet not routed correctly")
	}

	if !bus.TryConsume(TypeSSH3ToNetwork, &ssh) || ssh.UID() != 3 {
		t.Error("SSH3 packet not routed correctly")
	}
}

func TestWheelBus_NoWheelError(t *testing.T) {
	bus := NewWheelBus()

	var b Bucket
	b.SetTypeCode(TypeHardwareToNetwork)

	// Try to publish without creating wheel
	err := bus.Publish(&b)
	if err == nil {
		t.Error("Expected error when publishing to non-existent wheel")
	}
}

func TestWheelBus_RemoveWheel(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 256)

	// Remove empty wheel
	if err := bus.RemoveWheel(TypeHardwareToNetwork); err != nil {
		t.Errorf("RemoveWheel failed: %v", err)
	}

	wheels := bus.ListWheels()
	if len(wheels) != 0 {
		t.Error("Wheel not removed")
	}
}

func TestWheelBus_RemoveNonEmptyWheel(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 256)

	// Publish packet
	var b Bucket
	b.SetTypeCode(TypeHardwareToNetwork)
	bus.Publish(&b)

	// Try to remove non-empty wheel
	err := bus.RemoveWheel(TypeHardwareToNetwork)
	if err == nil {
		t.Error("Should not be able to remove non-empty wheel")
	}
}

func TestWheelBus_Stats(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 256)

	// Publish some packets
	var b Bucket
	b.SetTypeCode(TypeHardwareToNetwork)
	for i := 0; i < 10; i++ {
		b.SetUID(uint32(i))
		bus.Publish(&b)
	}

	stats := bus.GetStats()
	if stats.WheelCount != 1 {
		t.Errorf("WheelCount = %d, want 1", stats.WheelCount)
	}

	if stats.PublishCount != 10 {
		t.Errorf("PublishCount = %d, want 10", stats.PublishCount)
	}

	wheelStats := stats.Wheels[TypeHardwareToNetwork]
	if wheelStats.Length != 10 {
		t.Errorf("Wheel length = %d, want 10", wheelStats.Length)
	}
}

func TestWorker_BasicOperation(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 256)

	// Track processed packets
	var processedCount atomic.Uint64
	var processedUIDs []uint32
	var mu sync.Mutex

	handler := func(b *Bucket) error {
		processedCount.Add(1)
		mu.Lock()
		processedUIDs = append(processedUIDs, b.UID())
		mu.Unlock()
		return nil
	}

	// Start worker
	worker := NewWorker("test-worker", TypeHardwareToNetwork, bus, handler)
	worker.Start()

	// Give worker time to start
	time.Sleep(10 * time.Millisecond)

	// Publish packets
	for i := 0; i < 100; i++ {
		var b Bucket
		b.SetTimestamp(uint64(i))
		b.SetUID(uint32(i))
		b.SetTypeCode(TypeHardwareToNetwork)
		b.SetPayload([]byte("test"))
		bus.Publish(&b)
	}

	// Wait for processing (give worker time to start and process)
	time.Sleep(500 * time.Millisecond)
	worker.Stop()

	processed := processedCount.Load()
	if processed != 100 {
		t.Errorf("Processed %d packets, want 100", processed)
		t.Logf("Wheel stats: %+v", bus.GetWheel(TypeHardwareToNetwork).Stats())
	}
}

func TestWorker_MultipleWorkers(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 1024)

	const numWorkers = 4
	const numPackets = 1000

	var totalProcessed atomic.Uint64

	handler := func(b *Bucket) error {
		totalProcessed.Add(1)
		return nil
	}

	// Start multiple workers (work-stealing from same wheel)
	workers := make([]*Worker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		workers[i] = NewWorker("worker-"+string(rune(i)), TypeHardwareToNetwork, bus, handler)
		workers[i].Start()
	}

	// Publish packets
	var b Bucket
	b.SetTypeCode(TypeHardwareToNetwork)
	for i := 0; i < numPackets; i++ {
		b.SetUID(uint32(i))
		b.SetTimestamp(uint64(i))
		b.SetPayload([]byte("test"))
		bus.Publish(&b)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Stop all workers
	for _, w := range workers {
		w.Stop()
	}

	if totalProcessed.Load() != numPackets {
		t.Errorf("Processed %d packets, want %d", totalProcessed.Load(), numPackets)
	}
}

func TestWheelBus_ConcurrentPublish(t *testing.T) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 2048)

	const numPublishers = 8
	const packetsPerPublisher = 500

	var wg sync.WaitGroup

	// Start publishers
	for p := 0; p < numPublishers; p++ {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			var b Bucket
			b.SetTypeCode(TypeHardwareToNetwork)
			for i := 0; i < packetsPerPublisher; i++ {
				b.SetUID(uint32(publisherID*1000 + i))
				b.SetTimestamp(uint64(i))
				if err := bus.Publish(&b); err != nil {
					// Wheel might be full, retry
					time.Sleep(time.Microsecond)
					bus.Publish(&b)
				}
			}
		}(p)
	}

	wg.Wait()

	stats := bus.GetStats()
	expectedPublishes := uint64(numPublishers * packetsPerPublisher)

	// Account for retries due to full wheel
	if stats.PublishCount < expectedPublishes {
		t.Errorf("PublishCount = %d, want at least %d", stats.PublishCount, expectedPublishes)
	}
}

func BenchmarkWheelBus_Publish(b *testing.B) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 4096)

	var bucket Bucket
	bucket.SetTimestamp(12345)
	bucket.SetUID(1)
	bucket.SetTypeCode(TypeHardwareToNetwork)
	bucket.SetPayload([]byte("benchmark"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := bus.Publish(&bucket); err != nil {
			// Wheel full, consume one
			bus.Consume(TypeHardwareToNetwork)
			bus.Publish(&bucket)
		}
	}
}

func BenchmarkWheelBus_Consume(b *testing.B) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 4096)

	// Pre-fill
	var bucket Bucket
	bucket.SetTimestamp(1)
	bucket.SetTypeCode(TypeHardwareToNetwork)
	for i := 0; i < 4096; i++ {
		bucket.SetUID(uint32(i))
		bus.Publish(&bucket)
	}

	b.ResetTimer()
	idx := 0
	for i := 0; i < b.N; i++ {
		result := bus.Consume(TypeHardwareToNetwork)
		if result == nil {
			// Refill
			for j := 0; j < 4096; j++ {
				bucket.SetUID(uint32(idx))
				idx++
				bus.Publish(&bucket)
			}
			result = bus.Consume(TypeHardwareToNetwork)
		}
		if result != nil {
			result.Clear()
		}
	}
}

func BenchmarkWheelBus_PublishMultiLane(b *testing.B) {
	bus := NewWheelBus()
	bus.CreateWheel(TypeHardwareToNetwork, 4096)
	bus.CreateWheel(TypeMQTTToNetwork, 4096)
	bus.CreateWheel(TypeSSH3ToNetwork, 4096)

	buckets := []Bucket{
		{},
		{},
		{},
	}
	buckets[0].SetTypeCode(TypeHardwareToNetwork)
	buckets[1].SetTypeCode(TypeMQTTToNetwork)
	buckets[2].SetTypeCode(TypeSSH3ToNetwork)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bucket := &buckets[i%3]
		bus.Publish(bucket)
	}
}
