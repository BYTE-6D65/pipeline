package event

import (
	"fmt"
	"time"
)

// ErrorEvent represents a pipeline internal error, warning, or control signal.
// These events flow through a dedicated error bus (separate from data events)
// to provide observability into pipeline health without impacting data flow.
type ErrorEvent struct {
	// Severity indicates log level and urgency
	Severity ErrorSeverity

	// Signal indicates control intent (separate from severity for routing)
	Signal ControlSignal

	// Code is a terse, stable identifier (e.g., "MEM_PRESSURE")
	Code string

	// Message is human-readable description
	Message string

	// Component identifies the source (e.g., "bus:internal", "monitor:memory")
	Component string

	// Timestamp when the error occurred
	Timestamp time.Time

	// Context provides additional structured data
	Context map[string]any

	// Recoverable indicates if the system can continue operating
	Recoverable bool
}

// ErrorSeverity represents the severity level of an error event.
// Maps to standard log levels for easy integration with logging systems.
type ErrorSeverity int

const (
	DebugSeverity    ErrorSeverity = iota // Verbose debugging info
	InfoSeverity                           // Informational (e.g., "back pressure activated")
	WarningSeverity                        // Warning but not critical
	Error                                  // Error but recoverable
	CriticalSeverity                       // Critical, may cause crash
)

func (s ErrorSeverity) String() string {
	switch s {
	case DebugSeverity:
		return "DEBUG"
	case InfoSeverity:
		return "INFO"
	case WarningSeverity:
		return "WARNING"
	case Error:
		return "ERROR"
	case CriticalSeverity:
		return "CRITICAL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// ControlSignal represents a control intent separate from severity.
// Allows routing decisions without string-matching messages.
type ControlSignal int

const (
	SignalNone         ControlSignal = iota // No control signal
	SignalThrottle                           // Slow down ingestion
	SignalShed                               // Drop low-priority work
	SignalBreakerOpen                        // Circuit breaker opened
	SignalBreakerHalf                        // Circuit breaker half-open
	SignalBreakerClose                       // Circuit breaker closed
	SignalDegraded                           // Entered degraded mode
	SignalRecovered                          // Recovered from degraded mode
)

func (s ControlSignal) String() string {
	switch s {
	case SignalNone:
		return "NONE"
	case SignalThrottle:
		return "THROTTLE"
	case SignalShed:
		return "SHED"
	case SignalBreakerOpen:
		return "BREAKER_OPEN"
	case SignalBreakerHalf:
		return "BREAKER_HALF"
	case SignalBreakerClose:
		return "BREAKER_CLOSE"
	case SignalDegraded:
		return "DEGRADED"
	case SignalRecovered:
		return "RECOVERED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", s)
	}
}

// Error Code Constants
//
// These are terse, refactor-stable codes for common error conditions.
// They survive code changes better than string messages.
const (
	// Memory & Resources
	CodeMemPressure     = "MEM_PRESSURE"      // Memory usage approaching limit
	CodeMemRelief       = "MEM_RELIEF"        // Memory pressure relieved
	CodeMemCritical     = "MEM_CRITICAL"      // Critical memory pressure
	CodePSIPreOOM       = "PSI_PRE_OOM"       // PSI pre-OOM warning
	CodeOOMImminent     = "OOM_IMMINENT"      // OOM likely within seconds
	CodeDegradedMode    = "DEGRADED_MODE"     // Entered degraded mode
	CodeRecoveredMode   = "RECOVERED_MODE"    // Exited degraded mode

	// Buffering & Flow Control
	CodeBufSat          = "BUF_SAT"           // Buffer saturation detected
	CodeBufGrow         = "BUF_GROW"          // Buffer size increased
	CodeBufShrink       = "BUF_SHRINK"        // Buffer size decreased
	CodePublishBlock    = "PUBLISH_BLOCK"     // Publisher blocked (back pressure)
	CodeBackPressure    = "BACK_PRESSURE"     // Back pressure activated
	CodeDropSlow        = "DROP_SLOW"         // Event dropped (slow subscriber)
	CodeDropRED         = "DROP_RED"          // Event dropped (RED algorithm)
	CodeDropFull        = "DROP_FULL"         // Event dropped (queue full)

	// Component Failures
	CodeAdapterFail     = "ADAPTER_FAIL"      // Adapter encountered error
	CodeAdapterStart    = "ADAPTER_START"     // Adapter started
	CodeAdapterStop     = "ADAPTER_STOP"      // Adapter stopped
	CodeEmitterFail     = "EMITTER_FAIL"      // Emitter encountered error
	CodeEmitterStart    = "EMITTER_START"     // Emitter started
	CodeEmitterStop     = "EMITTER_STOP"      // Emitter stopped

	// Circuit Breaker
	CodeBreakerOpen     = "BREAKER_OPEN"      // Circuit breaker opened
	CodeBreakerHalf     = "BREAKER_HALF"      // Circuit breaker half-open
	CodeBreakerClose    = "BREAKER_CLOSE"     // Circuit breaker closed

	// Worker Scaling
	CodeWorkerScaleUp   = "WORKER_SCALE_UP"   // Workers scaled up
	CodeWorkerScaleDown = "WORKER_SCALE_DOWN" // Workers scaled down
	CodeWorkerIdle      = "WORKER_IDLE"       // Worker pool idle
	CodeWorkerSaturated = "WORKER_SATURATED"  // Worker pool saturated

	// System Health
	CodeHealthCheck     = "HEALTH_CHECK"      // Periodic health check
	CodePanic           = "PANIC"             // Panic recovered
	CodeShutdown        = "SHUTDOWN"          // Graceful shutdown initiated
)

// NewErrorEvent creates an error event with timestamp set to now.
func NewErrorEvent(severity ErrorSeverity, code, component, message string) ErrorEvent {
	return ErrorEvent{
		Severity:    severity,
		Signal:      SignalNone,
		Code:        code,
		Component:   component,
		Message:     message,
		Timestamp:   time.Now(),
		Context:     make(map[string]any),
		Recoverable: true, // Default to recoverable
	}
}

// WithSignal adds a control signal to the error event.
func (e ErrorEvent) WithSignal(signal ControlSignal) ErrorEvent {
	e.Signal = signal
	return e
}

// WithContext adds a context key-value pair.
func (e ErrorEvent) WithContext(key string, value any) ErrorEvent {
	if e.Context == nil {
		e.Context = make(map[string]any)
	}
	e.Context[key] = value
	return e
}

// WithRecoverable sets whether this error is recoverable.
func (e ErrorEvent) WithRecoverable(recoverable bool) ErrorEvent {
	e.Recoverable = recoverable
	return e
}

// String returns a formatted string representation of the error event.
func (e ErrorEvent) String() string {
	return fmt.Sprintf("[%s] %s: %s - %s (component=%s, recoverable=%t)",
		e.Severity, e.Code, e.Message, e.Signal, e.Component, e.Recoverable)
}
