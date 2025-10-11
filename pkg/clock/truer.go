package clock

import (
	"sync"
)

// Truer maps source timestamps (e.g., from hardware devices) to engine time
// using an affine transformation: engineTime = a * sourceTime + b.
// It uses a rolling window to continuously update the fit as new observations arrive.
type Truer interface {
	// Observe records a pair of (source, engine) timestamps to update the affine fit
	Observe(sourceTs MonoTime, engineTs MonoTime)

	// True maps a source timestamp to engine time using the current affine fit
	True(sourceTs MonoTime) MonoTime

	// Snapshot returns the current (a, b) coefficients for inspection/metrics
	Snapshot() (a float64, b float64)
}

// AffineTruer implements Truer using a rolling least-squares affine fit.
// The fit minimizes: Σ(engineTs - (a*sourceTs + b))² over recent observations.
type AffineTruer struct {
	mu sync.RWMutex

	// Affine coefficients: engineTime = a * sourceTime + b
	a float64
	b float64

	// Rolling window for least-squares fit
	window     []observation
	windowSize int
	index      int // Circular buffer index
	count      int // Number of observations (up to windowSize)

	// Running sums for efficient least-squares calculation
	sumSrc    float64
	sumEng    float64
	sumSrcSq  float64
	sumSrcEng float64
}

type observation struct {
	source MonoTime
	engine MonoTime
}

// NewAffineTruer creates a new Truer with the specified window size.
// Larger windows provide more stability but slower adaptation to drift.
// Typical values: 10-100 observations.
func NewAffineTruer(windowSize int) *AffineTruer {
	if windowSize < 2 {
		windowSize = 10 // Minimum for meaningful fit
	}
	return &AffineTruer{
		a:          1.0, // Initial identity transform
		b:          0.0,
		window:     make([]observation, windowSize),
		windowSize: windowSize,
	}
}

// Observe adds a new (source, engine) timestamp pair and updates the affine fit.
func (t *AffineTruer) Observe(sourceTs MonoTime, engineTs MonoTime) {
	t.mu.Lock()
	defer t.mu.Unlock()

	src := float64(sourceTs)
	eng := float64(engineTs)

	// If window is full, remove the oldest observation from sums
	if t.count == t.windowSize {
		old := t.window[t.index]
		oldSrc := float64(old.source)
		oldEng := float64(old.engine)

		t.sumSrc -= oldSrc
		t.sumEng -= oldEng
		t.sumSrcSq -= oldSrc * oldSrc
		t.sumSrcEng -= oldSrc * oldEng
	} else {
		t.count++
	}

	// Add new observation
	t.window[t.index] = observation{source: sourceTs, engine: engineTs}
	t.index = (t.index + 1) % t.windowSize

	t.sumSrc += src
	t.sumEng += eng
	t.sumSrcSq += src * src
	t.sumSrcEng += src * eng

	// Update affine fit using least squares
	// Solve: [sumSrcSq  sumSrc ] [a]   [sumSrcEng]
	//        [sumSrc    count  ] [b] = [sumEng   ]
	t.updateFit()
}

// updateFit calculates the affine coefficients using least-squares.
// Must be called with lock held.
func (t *AffineTruer) updateFit() {
	if t.count < 2 {
		// Not enough data, keep identity transform
		return
	}

	n := float64(t.count)

	// Calculate determinant
	det := t.sumSrcSq*n - t.sumSrc*t.sumSrc
	if det < 1e-10 {
		// Near-singular matrix (all sources identical), keep current fit
		return
	}

	// Solve for a and b
	t.a = (t.sumSrcEng*n - t.sumSrc*t.sumEng) / det
	t.b = (t.sumSrcSq*t.sumEng - t.sumSrc*t.sumSrcEng) / det

	// Clamp a to reasonable range (handle clock drift but prevent absurdity)
	// Typical clock drift is < 100 ppm, so a should be ~1.0
	if t.a < 0.999 {
		t.a = 0.999
	}
	if t.a > 1.001 {
		t.a = 1.001
	}
}

// True maps a source timestamp to engine time using the current affine fit.
func (t *AffineTruer) True(sourceTs MonoTime) MonoTime {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Apply affine transform: engineTime = a * sourceTime + b
	result := t.a*float64(sourceTs) + t.b
	return MonoTime(result)
}

// Snapshot returns the current affine coefficients (a, b).
func (t *AffineTruer) Snapshot() (a float64, b float64) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.a, t.b
}

// IdentityTruer is a trivial Truer that performs no transformation.
// Useful when source and engine clocks are already synchronized.
type IdentityTruer struct{}

// NewIdentityTruer creates a no-op Truer.
func NewIdentityTruer() *IdentityTruer {
	return &IdentityTruer{}
}

// Observe does nothing (no-op).
func (t *IdentityTruer) Observe(sourceTs MonoTime, engineTs MonoTime) {
	// No-op
}

// True returns the source timestamp unchanged.
func (t *IdentityTruer) True(sourceTs MonoTime) MonoTime {
	return sourceTs
}

// Snapshot returns identity transform (1, 0).
func (t *IdentityTruer) Snapshot() (a float64, b float64) {
	return 1.0, 0.0
}
