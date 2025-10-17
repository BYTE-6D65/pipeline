package engine

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/event"
)

// PSIMemory represents Pressure Stall Information for memory.
// PSI is a Linux kernel feature that provides early warning of resource pressure.
// See: https://www.kernel.org/doc/html/latest/accounting/psi.html
type PSIMemory struct {
	Avg10  float64 // 10-second average (%)
	Avg60  float64 // 60-second average (%)
	Avg300 float64 // 300-second average (%)
	Total  uint64  // Total stall time (microseconds)
}

// ReadPSIMemory reads memory pressure from /proc/pressure/memory.
// Returns error if PSI is not available (non-Linux or old kernel).
func ReadPSIMemory() (PSIMemory, error) {
	f, err := os.Open("/proc/pressure/memory")
	if err != nil {
		return PSIMemory{}, fmt.Errorf("PSI not available: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Format: "some avg10=0.00 avg60=0.00 avg300=0.00 total=12345"
		// We care about "some" line (partial stalls)
		if !strings.HasPrefix(line, "some ") {
			continue
		}

		return parsePSILine(line[5:]) // Skip "some "
	}

	if err := scanner.Err(); err != nil {
		return PSIMemory{}, err
	}

	return PSIMemory{}, fmt.Errorf("no PSI data found")
}

// parsePSILine parses a PSI line like "avg10=0.50 avg60=0.25 avg300=0.10 total=123456"
func parsePSILine(line string) (PSIMemory, error) {
	psi := PSIMemory{}
	fields := strings.Fields(line)

	for _, field := range fields {
		parts := strings.Split(field, "=")
		if len(parts) != 2 {
			continue
		}

		key, val := parts[0], parts[1]

		switch key {
		case "avg10":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				psi.Avg10 = v
			}
		case "avg60":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				psi.Avg60 = v
			}
		case "avg300":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				psi.Avg300 = v
			}
		case "total":
			if v, err := strconv.ParseUint(val, 10, 64); err == nil {
				psi.Total = v
			}
		}
	}

	return psi, nil
}

// PSIMonitor polls PSI and emits pre-OOM warnings to the error bus.
type PSIMonitor struct {
	// Config
	threshold     float64       // avg10 threshold (e.g., 0.2 = 20%)
	sustainWindow time.Duration // How long above threshold before alert
	pollInterval  time.Duration // Polling interval (e.g., 1s)
	errorBus      *event.ErrorBus

	// State
	aboveThresholdSince time.Time // When threshold was first exceeded
	lastAlert           time.Time // Last alert time (rate limiting)
}

// NewPSIMonitor creates a new PSI monitor with the given configuration.
func NewPSIMonitor(threshold float64, sustainWindow time.Duration, pollInterval time.Duration, errorBus *event.ErrorBus) *PSIMonitor {
	return &PSIMonitor{
		threshold:     threshold,
		sustainWindow: sustainWindow,
		pollInterval:  pollInterval,
		errorBus:      errorBus,
	}
}

// Start begins monitoring PSI in a background goroutine.
// Stops when context is cancelled.
func (pm *PSIMonitor) Start(ctx context.Context) {
	// Check if PSI is available
	if _, err := ReadPSIMemory(); err != nil {
		// PSI not available (non-Linux or old kernel)
		// This is OK - gracefully degrade
		pm.errorBus.Publish(event.NewErrorEvent(
			event.InfoSeverity,
			event.CodeHealthCheck,
			"monitor:psi",
			"PSI not available - pre-OOM detection disabled",
		).WithContext("error", err.Error()))
		return
	}

	pm.errorBus.Publish(event.NewErrorEvent(
		event.InfoSeverity,
		event.CodeHealthCheck,
		"monitor:psi",
		"PSI monitoring started",
	).WithContext("threshold", pm.threshold).
		WithContext("sustain_window", pm.sustainWindow.String()))

	ticker := time.NewTicker(pm.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.checkPSI()
		}
	}
}

// checkPSI reads PSI and emits warnings if threshold is exceeded.
func (pm *PSIMonitor) checkPSI() {
	psi, err := ReadPSIMemory()
	if err != nil {
		// Transient error - don't spam
		return
	}

	now := time.Now()

	// Check if avg10 exceeds threshold
	if psi.Avg10 > pm.threshold {
		// Above threshold
		if pm.aboveThresholdSince.IsZero() {
			// First time above - record start time
			pm.aboveThresholdSince = now
		}

		// Check if sustained for long enough
		sustainedDuration := now.Sub(pm.aboveThresholdSince)
		if sustainedDuration >= pm.sustainWindow {
			// Sustained pressure - emit alert
			// But don't spam - only alert once per minute
			if now.Sub(pm.lastAlert) >= time.Minute {
				pm.emitPreOOMWarning(psi, sustainedDuration)
				pm.lastAlert = now
			}
		}
	} else {
		// Below threshold - reset
		if !pm.aboveThresholdSince.IsZero() {
			// Pressure relieved
			pm.errorBus.Publish(event.NewErrorEvent(
				event.InfoSeverity,
				event.CodeMemRelief,
				"monitor:psi",
				"Memory pressure relieved",
			).WithSignal(event.SignalRecovered).
				WithContext("avg10", psi.Avg10).
				WithContext("avg60", psi.Avg60))

			pm.aboveThresholdSince = time.Time{}
		}
	}
}

// emitPreOOMWarning emits a pre-OOM warning event.
func (pm *PSIMonitor) emitPreOOMWarning(psi PSIMemory, sustainedFor time.Duration) {
	// Determine severity based on avg10
	severity := event.WarningSeverity
	if psi.Avg10 > 0.5 {
		severity = event.CriticalSeverity
	} else if psi.Avg10 > 0.3 {
		severity = event.Error
	}

	pm.errorBus.Publish(event.NewErrorEvent(
		severity,
		event.CodePSIPreOOM,
		"monitor:psi",
		fmt.Sprintf("Memory pressure sustained at %.1f%% for %s - pre-OOM warning",
			psi.Avg10, sustainedFor.Round(time.Second)),
	).WithSignal(event.SignalShed).
		WithRecoverable(false).
		WithContext("avg10", psi.Avg10).
		WithContext("avg60", psi.Avg60).
		WithContext("avg300", psi.Avg300).
		WithContext("sustained_for", sustainedFor.String()))
}

// String returns a formatted string representation of PSI data.
func (p PSIMemory) String() string {
	return fmt.Sprintf("avg10=%.2f%% avg60=%.2f%% avg300=%.2f%% total=%dus",
		p.Avg10, p.Avg60, p.Avg300, p.Total)
}
