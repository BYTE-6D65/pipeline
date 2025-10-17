package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/clock"
	"github.com/BYTE-6D65/pipeline/pkg/event"
	"github.com/BYTE-6D65/pipeline/pkg/registry"
	"github.com/BYTE-6D65/pipeline/pkg/telemetry"
)

// Engine wires together core infrastructure components.
// It provides dependency injection for event buses, clock, and registry.
type Engine struct {
	internalBus event.Bus
	externalBus event.Bus
	clock       clock.Clock
	registry    registry.Registry
	metrics     *telemetry.Metrics

	// Error signaling and fault tolerance (Phase 1)
	errorBus         *event.ErrorBus
	config           Config
	flightRecorder   *FlightRecorder
	memoryLimit      uint64
	memoryLimitSrc   string
	psiMonitor       *PSIMonitor
	monitorCtx       context.Context
	monitorCancel    context.CancelFunc
	lastCrashDump    time.Time
	crashDumpMu      sync.Mutex

	// Graceful degradation (Phase 2)
	redDropper  *REDDropper
	aimdGovernor *AIMDGovernor
	controlLab  *ControlLab
}

// EngineOption configures an Engine instance.
type EngineOption func(*Engine)

// WithInternalBus sets the internal event bus (for system events).
func WithInternalBus(bus event.Bus) EngineOption {
	return func(e *Engine) {
		e.internalBus = bus
	}
}

// WithExternalBus sets the external event bus (for domain events).
func WithExternalBus(bus event.Bus) EngineOption {
	return func(e *Engine) {
		e.externalBus = bus
	}
}

// WithClock sets the clock implementation.
func WithClock(clk clock.Clock) EngineOption {
	return func(e *Engine) {
		e.clock = clk
	}
}

// WithRegistry sets the registry implementation.
func WithRegistry(reg registry.Registry) EngineOption {
	return func(e *Engine) {
		e.registry = reg
	}
}

// WithMetrics sets the telemetry metrics instance.
func WithMetrics(metrics *telemetry.Metrics) EngineOption {
	return func(e *Engine) {
		e.metrics = metrics
	}
}

// NewWithConfig creates a new Engine with the given configuration.
// This constructor enables error signaling, memory monitoring, and fault tolerance.
//
// Monitors started:
//   - Flight recorder (continuous snapshots for crash forensics)
//   - Memory monitor (emits warnings at thresholds)
//   - PSI monitor (pre-OOM detection on Linux)
//
// Use Shutdown() to clean up monitors.
func NewWithConfig(cfg Config, opts ...EngineOption) (*Engine, error) {
	// Validate config
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Detect memory limit
	memLimit, memSrc, memOk := DetectMemoryLimit()
	if !memOk {
		memLimit = 0 // Unlimited
		memSrc = "none"
	}

	// Create monitor context
	monitorCtx, monitorCancel := context.WithCancel(context.Background())

	// Create engine with clock (used by all components for consistency)
	engineClock := clock.NewSystemClock()
	errorBus := event.NewErrorBus(cfg.ErrorBusBufferSize)

	// Create Phase 2 components (graceful degradation)
	redDropper := NewDefaultREDDropper()
	aimdGovernor := NewDefaultAIMDGovernor(engineClock, cfg.ControlCooldown)

	engine := &Engine{
		clock:          engineClock,
		registry:       registry.NewInMemoryRegistry(),
		metrics:        telemetry.Default(),
		errorBus:       errorBus,
		config:         cfg,
		flightRecorder: NewFlightRecorder(cfg.FlightRecorderSize),
		memoryLimit:    memLimit,
		memoryLimitSrc: memSrc,
		monitorCtx:     monitorCtx,
		monitorCancel:  monitorCancel,
		redDropper:     redDropper,
		aimdGovernor:   aimdGovernor,
	}

	// Create control lab (analyzes state, emits to error bus for observability)
	engine.controlLab = NewControlLab(
		engineClock,
		errorBus,
		aimdGovernor,
		redDropper,
		memLimit, // Memory limit for direct polling
		cfg.GovernorPollInterval,
	)

	// Apply options
	for _, opt := range opts {
		opt(engine)
	}

	// Ensure metrics
	if engine.metrics == nil {
		engine.metrics = telemetry.Default()
	}

	// Create default buses if not provided
	if engine.internalBus == nil {
		engine.internalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("internal"),
			event.WithMetrics(engine.metrics),
		)
	}

	if engine.externalBus == nil {
		engine.externalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("external"),
			event.WithMetrics(engine.metrics),
		)
	}

	// Start monitors
	engine.startMonitors()

	return engine, nil
}

// New creates a new Engine with sensible defaults.
// Default configuration:
// - InternalBus: InMemoryBus with 64 buffer, drop-slow disabled
// - ExternalBus: InMemoryBus with 128 buffer, drop-slow disabled
// - Clock: SystemClock (monotonic)
// - Registry: InMemoryRegistry
func New(opts ...EngineOption) *Engine {
	engine := &Engine{
		clock:    clock.NewSystemClock(),
		registry: registry.NewInMemoryRegistry(),
		metrics:  telemetry.Default(),
	}

	for _, opt := range opts {
		opt(engine)
	}

	if engine.metrics == nil {
		engine.metrics = telemetry.Default()
	}

	if engine.internalBus == nil {
		engine.internalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("internal"),
			event.WithMetrics(engine.metrics),
		)
	}

	if engine.externalBus == nil {
		engine.externalBus = event.NewInMemoryBus(
			event.WithBufferSize(32),
			event.WithDropSlow(false),
			event.WithBusName("external"),
			event.WithMetrics(engine.metrics),
		)
	}

	return engine
}

// InternalBus returns the internal event bus (for system/coordination events).
func (e *Engine) InternalBus() event.Bus {
	return e.internalBus
}

// ExternalBus returns the external event bus (for domain events).
func (e *Engine) ExternalBus() event.Bus {
	return e.externalBus
}

// Clock returns the clock implementation.
func (e *Engine) Clock() clock.Clock {
	return e.clock
}

// Registry returns the registry implementation.
func (e *Engine) Registry() registry.Registry {
	return e.registry
}

// Metrics returns the telemetry metrics instance.
func (e *Engine) Metrics() *telemetry.Metrics {
	return e.metrics
}

// ErrorBus returns the error bus for observability.
// Returns nil if engine was created with New() instead of NewWithConfig().
func (e *Engine) ErrorBus() *event.ErrorBus {
	return e.errorBus
}

// Config returns the engine configuration.
// Returns zero value if engine was created with New() instead of NewWithConfig().
func (e *Engine) Config() Config {
	return e.config
}

// Governor returns the AIMD governor.
// Returns nil if engine was created with New() instead of NewWithConfig().
func (e *Engine) Governor() *AIMDGovernor {
	return e.aimdGovernor
}

// RED returns the RED dropper.
// Returns nil if engine was created with New() instead of NewWithConfig().
func (e *Engine) RED() *REDDropper {
	return e.redDropper
}

// ControlLab returns the control lab.
// Returns nil if engine was created with New() instead of NewWithConfig().
func (e *Engine) ControlLab() *ControlLab {
	return e.controlLab
}

// startMonitors starts background monitoring goroutines.
func (e *Engine) startMonitors() {
	// Emit startup event
	e.errorBus.Publish(event.NewErrorEvent(
		event.InfoSeverity,
		event.CodeHealthCheck,
		"engine",
		"Engine started with error signaling enabled",
	).WithContext("memory_limit", FormatBytes(e.memoryLimit)).
		WithContext("memory_source", e.memoryLimitSrc))

	// Start flight recorder
	e.WrapGoroutine("flight-recorder", func() {
		e.flightRecorder.StartRecording(
			e.monitorCtx,
			e.config.FlightRecorderInterval,
			e.memoryLimit,
		)
	})

	// Start memory monitor
	e.WrapGoroutine("memory-monitor", func() {
		e.monitorMemory()
	})

	// Start PSI monitor (will gracefully degrade on non-Linux)
	e.psiMonitor = NewPSIMonitor(
		e.config.PSIThreshold,
		e.config.PSISustainWindow,
		e.config.PSIPollInterval,
		e.errorBus,
	)
	e.WrapGoroutine("psi-monitor", func() {
		e.psiMonitor.Start(e.monitorCtx)
	})

	// Start control lab (Phase 2 - analyzes state, emits to error bus)
	if e.controlLab != nil {
		e.WrapGoroutine("control-lab", func() {
			e.controlLab.Start(e.monitorCtx)
		})
	}
}

// monitorMemory polls memory usage and emits warnings.
func (e *Engine) monitorMemory() {
	ticker := time.NewTicker(e.config.FlightRecorderInterval)
	defer ticker.Stop()

	var lastLevel int // 0=normal, 1=warn, 2=error, 3=crit

	for {
		select {
		case <-e.monitorCtx.Done():
			return
		case <-ticker.C:
			stats := ReadMemoryStatsFast(e.memoryLimit)

			// Update flight recorder with current stats
			snap := e.flightRecorder.CaptureSnapshot(e.memoryLimit, nil)
			e.flightRecorder.Record(snap)

			// Skip if no memory limit
			if e.memoryLimit == 0 {
				continue
			}

			// Determine severity level
			level := 0
			if stats.UsagePct >= e.config.MemoryEnterThreshold {
				level = 1 // Warning
			}
			if stats.UsagePct >= 0.85 {
				level = 2 // Error
			}
			if stats.UsagePct >= 0.90 {
				level = 3 // Critical
			}

			// Emit event on level change (rising)
			if level > lastLevel {
				e.emitMemoryWarning(stats, level)
			}

			// Emit relief event on drop below exit threshold
			if lastLevel > 0 && stats.UsagePct < e.config.MemoryExitThreshold {
				e.errorBus.Publish(event.NewErrorEvent(
					event.InfoSeverity,
					event.CodeMemRelief,
					"monitor:memory",
					"Memory pressure relieved",
				).WithSignal(event.SignalRecovered).
					WithContext("usage_pct", fmt.Sprintf("%.1f%%", stats.UsagePct*100)).
					WithContext("heap_alloc", FormatBytes(stats.HeapAlloc)).
					WithContext("limit", FormatBytes(stats.Limit)))
				level = 0
			}

			lastLevel = level
		}
	}
}

// emitMemoryWarning emits a memory warning event.
func (e *Engine) emitMemoryWarning(stats MemoryStats, level int) {
	severity := event.WarningSeverity
	signal := event.SignalThrottle
	code := event.CodeMemPressure

	switch level {
	case 2: // Error (85%+)
		severity = event.Error
		signal = event.SignalShed
	case 3: // Critical (90%+)
		severity = event.CriticalSeverity
		signal = event.SignalShed
		code = event.CodeMemCritical
	}

	e.errorBus.Publish(event.NewErrorEvent(
		severity,
		code,
		"monitor:memory",
		fmt.Sprintf("Memory usage at %.1f%% of limit", stats.UsagePct*100),
	).WithSignal(signal).
		WithRecoverable(true).
		WithContext("usage_pct", fmt.Sprintf("%.1f%%", stats.UsagePct*100)).
		WithContext("heap_alloc", FormatBytes(stats.HeapAlloc)).
		WithContext("limit", FormatBytes(stats.Limit)).
		WithContext("gc_count", stats.GCCount))
}

// WrapGoroutine wraps a goroutine with panic recovery.
// If a panic occurs, it dumps a crash report with flight recorder data.
// Rate limited to 1 dump per minute to prevent disk saturation.
//
// Usage:
//
//	e.WrapGoroutine("worker-pool", func() {
//	    // Your goroutine code
//	})
func (e *Engine) WrapGoroutine(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if e.canDumpCrash() {
					e.dumpCrashReport(name, r)
				}

				// Re-emit as error event
				if e.errorBus != nil {
					e.errorBus.Publish(event.NewErrorEvent(
						event.CriticalSeverity,
						event.CodePanic,
						name,
						fmt.Sprintf("Goroutine panic: %v", r),
					).WithRecoverable(false).
						WithContext("stack", string(debug.Stack())))
				}
			}
		}()

		fn()
	}()
}

// canDumpCrash checks if a crash dump is allowed (rate limiting).
func (e *Engine) canDumpCrash() bool {
	e.crashDumpMu.Lock()
	defer e.crashDumpMu.Unlock()

	now := time.Now()
	if now.Sub(e.lastCrashDump) < time.Minute {
		return false // Rate limited
	}

	e.lastCrashDump = now
	return true
}

// dumpCrashReport writes a crash report to disk.
func (e *Engine) dumpCrashReport(goroutineName string, panicValue any) {
	// Create crash-logs directory
	logDir := "crash-logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create crash-logs directory: %v\n", err)
		return
	}

	// Generate filename
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(logDir, fmt.Sprintf("crash_%s_%s.log", timestamp, goroutineName))

	// Create file
	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create crash log: %v\n", err)
		return
	}
	defer f.Close()

	// Write crash header
	fmt.Fprintf(f, "=== PIPELINE CRASH REPORT ===\n")
	fmt.Fprintf(f, "Time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "Goroutine: %s\n", goroutineName)
	fmt.Fprintf(f, "Panic: %v\n\n", panicValue)

	// Write stack trace
	fmt.Fprintf(f, "=== Stack Trace ===\n")
	fmt.Fprintf(f, "%s\n\n", debug.Stack())

	// Dump flight recorder
	if e.flightRecorder != nil {
		if err := e.flightRecorder.Dump(f); err != nil {
			fmt.Fprintf(f, "Error dumping flight recorder: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Crash report written to: %s\n", filename)
}

// Shutdown gracefully shuts down the engine and releases resources.
// It closes both event buses and waits for the context to complete.
// Also stops monitors if created with NewWithConfig.
func (e *Engine) Shutdown(ctx context.Context) (err error) {
	start := time.Now()
	defer func() {
		recordEngineOperation(e.metrics, "engine.shutdown", start, err)
	}()

	// Stop monitors if they exist
	if e.monitorCancel != nil {
		e.monitorCancel()
	}

	// Emit shutdown event
	if e.errorBus != nil {
		e.errorBus.Publish(event.NewErrorEvent(
			event.InfoSeverity,
			event.CodeHealthCheck,
			"engine",
			"Engine shutting down",
		))
	}

	errCh := make(chan error, 3)

	// Close internal bus
	go func() {
		if err := e.internalBus.Close(); err != nil {
			errCh <- fmt.Errorf("internal bus shutdown: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Close external bus
	go func() {
		if err := e.externalBus.Close(); err != nil {
			errCh <- fmt.Errorf("external bus shutdown: %w", err)
		} else {
			errCh <- nil
		}
	}()

	// Close error bus
	go func() {
		if e.errorBus != nil {
			if err := e.errorBus.Close(); err != nil {
				errCh <- fmt.Errorf("error bus shutdown: %w", err)
			} else {
				errCh <- nil
			}
		} else {
			errCh <- nil
		}
	}()

	// Wait for all to complete or context to cancel
	var errors []error
	for i := 0; i < 3; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				errors = append(errors, err)
			}
		case <-ctx.Done():
			err = fmt.Errorf("shutdown cancelled: %w", ctx.Err())
			return
		}
	}

	if len(errors) > 0 {
		err = fmt.Errorf("shutdown errors: %v", errors)
		return
	}

	return
}
