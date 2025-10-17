package framework

import (
	"context"
	"fmt"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/engine"
)

// TestCase defines the interface for all behavior tests.
type TestCase interface {
	// Name returns the test name (e.g., "1.1: Normal → Degraded Transition")
	Name() string

	// Category returns the test category (e.g., "Core AIMD Cycle Tests")
	Category() string

	// Description returns a brief description of what the test validates
	Description() string

	// Setup prepares the test environment (creates engine, allocates resources)
	Setup(ctx context.Context) error

	// Run executes the test procedure
	Run(ctx context.Context) error

	// Teardown cleans up resources
	Teardown() error

	// Validate checks pass/fail criteria and returns result
	Validate() *TestResult
}

// TestResult contains the outcome of a test execution.
type TestResult struct {
	TestName   string
	Category   string
	Passed     bool
	Duration   time.Duration
	StartTime  time.Time
	EndTime    time.Time
	Assertions []*Assertion
	Metrics    map[string]interface{}
	Errors     []error
	Warnings   []string
}

// Assertion represents a single pass/fail check.
type Assertion struct {
	Name     string
	Expected interface{}
	Actual   interface{}
	Passed   bool
	Message  string
	Critical bool // If true, test fails immediately
}

// NewTestResult creates a new test result.
func NewTestResult(testName, category string) *TestResult {
	return &TestResult{
		TestName:   testName,
		Category:   category,
		Passed:     true, // Assume pass until assertion fails
		Assertions: make([]*Assertion, 0),
		Metrics:    make(map[string]interface{}),
		Errors:     make([]error, 0),
		Warnings:   make([]string, 0),
		StartTime:  time.Now(),
	}
}

// AddAssertion adds an assertion to the result.
// If the assertion fails and is critical, marks the entire test as failed.
func (r *TestResult) AddAssertion(a *Assertion) {
	r.Assertions = append(r.Assertions, a)
	if !a.Passed {
		r.Passed = false
	}
}

// AddMetric adds a metric to track.
func (r *TestResult) AddMetric(name string, value interface{}) {
	r.Metrics[name] = value
}

// AddError adds an error (doesn't necessarily fail the test).
func (r *TestResult) AddError(err error) {
	r.Errors = append(r.Errors, err)
	r.Passed = false // Any error = test failure
}

// AddWarning adds a warning (doesn't fail the test).
func (r *TestResult) AddWarning(msg string) {
	r.Warnings = append(r.Warnings, msg)
}

// Finish marks the test as complete and calculates duration.
func (r *TestResult) Finish() {
	r.EndTime = time.Now()
	r.Duration = r.EndTime.Sub(r.StartTime)
}

// PassedAssertions returns the number of passed assertions.
func (r *TestResult) PassedAssertions() int {
	count := 0
	for _, a := range r.Assertions {
		if a.Passed {
			count++
		}
	}
	return count
}

// FailedAssertions returns the number of failed assertions.
func (r *TestResult) FailedAssertions() int {
	return len(r.Assertions) - r.PassedAssertions()
}

// String returns a human-readable summary.
func (r *TestResult) String() string {
	status := "PASS"
	if !r.Passed {
		status = "FAIL"
	}
	return fmt.Sprintf("[%s] %s (%s)", status, r.TestName, r.Duration)
}

// BaseTestCase provides common functionality for tests.
// Embed this in your test implementations.
type BaseTestCase struct {
	engine      *engine.Engine
	config      engine.Config
	memoryLimit uint64
	result      *TestResult
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewBaseTestCase creates a new base test case.
func NewBaseTestCase() *BaseTestCase {
	return &BaseTestCase{
		result: NewTestResult("", ""),
	}
}

// Engine returns the test engine.
func (b *BaseTestCase) Engine() *engine.Engine {
	return b.engine
}

// Result returns the test result.
func (b *BaseTestCase) Result() *TestResult {
	return b.result
}

// MemoryLimit returns the detected memory limit.
func (b *BaseTestCase) MemoryLimit() uint64 {
	return b.memoryLimit
}

// Context returns the test context.
func (b *BaseTestCase) Context() context.Context {
	return b.ctx
}

// SetupEngine creates an engine with the given config.
func (b *BaseTestCase) SetupEngine(ctx context.Context, cfg engine.Config) error {
	b.ctx, b.cancel = context.WithCancel(ctx)

	// Detect memory limit
	limit, _, ok := engine.DetectMemoryLimit()
	if !ok {
		return fmt.Errorf("no memory limit detected - set GOMEMLIMIT")
	}
	b.memoryLimit = limit
	b.config = cfg

	// Create engine
	eng, err := engine.NewWithConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}
	b.engine = eng

	return nil
}

// TeardownEngine shuts down the engine.
func (b *BaseTestCase) TeardownEngine() error {
	if b.cancel != nil {
		b.cancel()
	}

	if b.engine != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return b.engine.Shutdown(shutdownCtx)
	}

	return nil
}

// Assert adds an assertion to the result.
func (b *BaseTestCase) Assert(name string, expected, actual interface{}, passed bool, message string) {
	b.result.AddAssertion(&Assertion{
		Name:     name,
		Expected: expected,
		Actual:   actual,
		Passed:   passed,
		Message:  message,
		Critical: false,
	})
}

// AssertCritical adds a critical assertion (fails test immediately).
func (b *BaseTestCase) AssertCritical(name string, expected, actual interface{}, passed bool, message string) {
	b.result.AddAssertion(&Assertion{
		Name:     name,
		Expected: expected,
		Actual:   actual,
		Passed:   passed,
		Message:  message,
		Critical: true,
	})
}

// Metric adds a metric to track.
func (b *BaseTestCase) Metric(name string, value interface{}) {
	b.result.AddMetric(name, value)
}

// Error adds an error to the result.
func (b *BaseTestCase) Error(err error) {
	b.result.AddError(err)
}

// Warning adds a warning to the result.
func (b *BaseTestCase) Warning(msg string) {
	b.result.AddWarning(msg)
}
