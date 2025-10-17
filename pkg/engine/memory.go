package engine

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
)

// MemoryStats provides fast memory statistics for monitoring.
type MemoryStats struct {
	HeapAlloc uint64  // Currently allocated heap bytes
	HeapSys   uint64  // Total heap memory from OS
	GCCount   uint32  // Number of completed GC cycles
	Limit     uint64  // Effective memory limit (0 if unlimited)
	UsagePct  float64 // Usage as percentage of limit (0.0-1.0)
}

// DetectMemoryLimit returns the effective memory limit in bytes.
// Checks in priority order:
//  1. GOMEMLIMIT (Go 1.19+ soft limit)
//  2. cgroup v2 (modern Linux containers)
//  3. cgroup v1 (legacy Linux containers)
//  4. MEMORY_LIMIT_BYTES env var (manual override)
//
// Returns (limit, source, ok) where source describes where the limit came from.
func DetectMemoryLimit() (uint64, string, bool) {
	// Priority 1: GOMEMLIMIT (Go runtime soft limit)
	// debug.SetMemoryLimit(-1) returns current limit without changing it
	if limit := debug.SetMemoryLimit(-1); limit != math.MaxInt64 {
		return uint64(limit), "GOMEMLIMIT", true
	}

	// Priority 2: cgroup v2 (modern containers)
	if limit, ok := cgroupV2MemLimit(); ok {
		return limit, "cgroup-v2", true
	}

	// Priority 3: cgroup v1 (legacy containers)
	if limit, ok := cgroupV1MemLimit(); ok {
		return limit, "cgroup-v1", true
	}

	// Priority 4: Environment variable (Apple containers, manual config)
	if envLimit := os.Getenv("MEMORY_LIMIT_BYTES"); envLimit != "" {
		if limit, err := strconv.ParseUint(envLimit, 10, 64); err == nil && limit > 0 {
			return limit, "env:MEMORY_LIMIT_BYTES", true
		}
	}

	// No limit detected
	return 0, "none", false
}

// cgroupV2MemLimit reads memory limit from cgroup v2.
// Returns (limit, ok) where ok indicates if a valid limit was found.
func cgroupV2MemLimit() (uint64, bool) {
	// cgroup v2 path
	data, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil {
		return 0, false
	}

	s := strings.TrimSpace(string(data))

	// Handle "max" (unlimited)
	if s == "max" {
		return 0, false
	}

	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}

	// Reject absurd values (sentinels for "unlimited")
	// 2^60 = 1152921504606846976 (1 exabyte) - clearly unlimited
	if v == 0 || v > (1 << 60) {
		return 0, false
	}

	return v, true
}

// cgroupV1MemLimit reads memory limit from cgroup v1.
func cgroupV1MemLimit() (uint64, bool) {
	// cgroup v1 path
	data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return 0, false
	}

	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}

	// Reject absurd values (same as v2)
	if v == 0 || v > (1 << 60) {
		return 0, false
	}

	return v, true
}

// DetectCPUQuota returns the CPU quota as a float (e.g., 1.5 CPUs).
// Returns (cpus, ok) where ok indicates if a valid quota was found.
func DetectCPUQuota() (float64, bool) {
	// cgroup v2
	if cpus, ok := cgroupV2CPUQuota(); ok {
		return cpus, true
	}

	// cgroup v1 (not implemented yet, but placeholder)
	// TODO: Add cgroup v1 CPU quota detection if needed

	// Fallback: use runtime.NumCPU() (host CPUs, not container limit)
	return float64(runtime.NumCPU()), false
}

// cgroupV2CPUQuota reads CPU quota from cgroup v2.
// Returns (cpus, ok) where cpus is the quota as a float (e.g., 1.5 for 1.5 CPUs).
func cgroupV2CPUQuota() (float64, bool) {
	// cgroup v2 cpu.max format: "<quota> <period>" or "max <period>"
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0, false
	}

	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, false
	}

	// Handle "max" quota (unlimited)
	if fields[0] == "max" {
		return 0, false
	}

	quota, err1 := strconv.ParseFloat(fields[0], 64)
	period, err2 := strconv.ParseFloat(fields[1], 64)

	if err1 != nil || err2 != nil || quota <= 0 || period <= 0 {
		return 0, false
	}

	// CPUs = quota / period
	// Example: 150000 / 100000 = 1.5 CPUs
	return quota / period, true
}

// ReadMemoryStatsFast reads current memory statistics using runtime/metrics.
// This is faster than runtime.ReadMemStats() for frequent polling.
func ReadMemoryStatsFast(limit uint64) MemoryStats {
	// Use runtime/metrics for fast, low-overhead reads
	samples := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/gc/cycles/total:gc-cycles"},
		{Name: "/memory/classes/total:bytes"},
	}

	metrics.Read(samples)

	heapAlloc := samples[0].Value.Uint64()
	gcCount := uint32(samples[1].Value.Uint64())
	heapSys := samples[2].Value.Uint64()

	var usagePct float64
	if limit > 0 {
		usagePct = float64(heapAlloc) / float64(limit)
	}

	return MemoryStats{
		HeapAlloc: heapAlloc,
		HeapSys:   heapSys,
		GCCount:   gcCount,
		Limit:     limit,
		UsagePct:  usagePct,
	}
}

// ReadMemoryStatsSlow reads detailed memory statistics using runtime.MemStats.
// This is slower than ReadMemoryStatsFast() but provides more detail.
// Use sparingly (e.g., for crash dumps, not hot path monitoring).
func ReadMemoryStatsSlow() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// FormatBytes formats bytes as human-readable string.
func FormatBytes(bytes uint64) string {
	if bytes == 0 {
		return "0 B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	base := 1024.0
	value := float64(bytes)

	for _, unit := range units {
		if value < base {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= base
	}

	return fmt.Sprintf("%.1f TB", value)
}
