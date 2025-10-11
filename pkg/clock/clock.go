package clock

import "time"

// MonoTime represents a monotonic timestamp in nanoseconds since an arbitrary epoch.
// Using int64 provides ~292 years of range with nanosecond precision.
type MonoTime int64

// Clock provides monotonic time operations for the engine.
// All timing, ordering, and interval calculations must use MonoTime.
type Clock interface {
	// Now returns the current monotonic time
	Now() MonoTime

	// Since returns the duration elapsed since the given monotonic time
	Since(t MonoTime) time.Duration
}

// ToDuration converts a MonoTime (nanoseconds) to a time.Duration.
func ToDuration(ns MonoTime) time.Duration {
	return time.Duration(ns)
}

// FromDuration converts a time.Duration to MonoTime (nanoseconds).
func FromDuration(d time.Duration) MonoTime {
	return MonoTime(d.Nanoseconds())
}

// ToUnixNano converts MonoTime to Unix nanoseconds (for external timestamps).
// Note: This assumes the MonoTime epoch aligns with Unix epoch, which is
// typically not the case. Use with caution or prefer Truer for conversions.
func ToUnixNano(m MonoTime) int64 {
	return int64(m)
}

// FromUnixNano converts Unix nanoseconds to MonoTime.
func FromUnixNano(unixNano int64) MonoTime {
	return MonoTime(unixNano)
}

// SystemClock uses the system's monotonic clock.
type SystemClock struct {
	epoch time.Time // Cached at creation to provide stable monotonic base
}

// NewSystemClock creates a new SystemClock anchored at the current time.
func NewSystemClock() *SystemClock {
	return &SystemClock{
		epoch: time.Now(),
	}
}

// Now returns the current monotonic time in nanoseconds since epoch.
func (s *SystemClock) Now() MonoTime {
	// Use time.Since which leverages monotonic clock internally
	elapsed := time.Since(s.epoch)
	return FromDuration(elapsed)
}

// Since returns the duration elapsed since the given monotonic time.
func (s *SystemClock) Since(t MonoTime) time.Duration {
	return ToDuration(s.Now() - t)
}
