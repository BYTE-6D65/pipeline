package engine

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all tunable parameters for the pipeline engine.
// Values can be set via:
//  1. Code (programmatic configuration)
//  2. Environment variables (PIPELINE_*)
//  3. Config file (future: YAML/TOML)
//
// Precedence: Code > Env Vars > Config File > Defaults
type Config struct {
	// Governor Thresholds
	MemoryEnterThreshold float64       `env:"PIPELINE_MEM_ENTER_PCT" default:"0.70"`     // Enter degraded mode
	MemoryExitThreshold  float64       `env:"PIPELINE_MEM_EXIT_PCT" default:"0.55"`      // Exit degraded mode
	MemoryCriticalPct    float64       `env:"PIPELINE_MEM_CRITICAL_PCT" default:"0.90"`  // Critical threshold
	GovernorPollInterval time.Duration `env:"PIPELINE_GOVERNOR_POLL_MS" default:"50ms"`  // How often to check

	// Control Lab
	ControlLoopInterval time.Duration `env:"PIPELINE_CONTROL_INTERVAL" default:"3s"`  // Control lab tick (kept as ControlLoopInterval for compatibility)
	ControlCooldown     time.Duration `env:"PIPELINE_CONTROL_COOLDOWN" default:"30s"` // Min time between actions
	MaxActionsPerLoop   int           `env:"PIPELINE_MAX_ACTIONS" default:"1"`        // Max actions per tick

	// Queue/Buffer Sizing
	QueueSizeStart int     `env:"PIPELINE_QUEUE_START" default:"128"`    // Initial queue size
	QueueSizeMin   int     `env:"PIPELINE_QUEUE_MIN" default:"8"`        // Minimum queue size
	QueueSizeMax   int     `env:"PIPELINE_QUEUE_MAX" default:"1024"`     // Maximum queue size
	REDMinFill     float64 `env:"PIPELINE_RED_MIN_FILL" default:"0.6"`   // Start dropping (60%)
	REDMaxDropProb float64 `env:"PIPELINE_RED_MAX_PROB" default:"0.3"`   // Max drop probability (30%)

	// Worker Scaling
	TargetLagMs int `env:"PIPELINE_TARGET_LAG_MS" default:"10"` // Target queue lag in ms
	MinWorkers  int `env:"PIPELINE_MIN_WORKERS" default:"2"`    // Minimum worker count
	MaxWorkers  int `env:"PIPELINE_MAX_WORKERS" default:"8"`    // Maximum worker count

	// Memory Budget
	BufferMemoryBudgetPct float64 `env:"PIPELINE_BUFFER_MEMORY_PCT" default:"0.50"` // % of limit for buffers

	// PSI (Pressure Stall Information) - Linux only
	PSIEnabled       bool          `env:"PIPELINE_PSI_ENABLED" default:"true"`       // Enable PSI monitoring
	PSIThreshold     float64       `env:"PIPELINE_PSI_THRESHOLD" default:"0.2"`      // avg10 threshold (20%)
	PSISustainWindow time.Duration `env:"PIPELINE_PSI_SUSTAIN" default:"2s"`         // Sustain duration
	PSIPollInterval  time.Duration `env:"PIPELINE_PSI_POLL_INTERVAL" default:"1s"`   // Polling interval

	// Flight Recorder
	FlightRecorderSize     int           `env:"PIPELINE_FLIGHT_RECORDER_SIZE" default:"100"` // Number of snapshots
	FlightRecorderInterval time.Duration `env:"PIPELINE_FLIGHT_RECORDER_INTERVAL" default:"1s"`

	// Error Bus
	ErrorBusBufferSize int  `env:"PIPELINE_ERROR_BUS_BUFFER" default:"32"`   // Error event buffer per sub
	ErrorBusSampling   bool `env:"PIPELINE_ERROR_SAMPLING" default:"false"`  // Sample high-frequency errors

	// AIMD Governor Tuning
	AIMDIncrStep   float64 `env:"PIPELINE_AIMD_INCR" default:"0.05"`  // Additive increase per tick
	AIMDDecrFactor float64 `env:"PIPELINE_AIMD_DECR" default:"0.5"`   // Multiplicative decrease factor
	AIMDMaxPerTick float64 `env:"PIPELINE_AIMD_MAX_TICK" default:"0.1"` // Max change per tick

	// Memory Limit Override
	MemoryLimitBytes uint64 `env:"PIPELINE_MEMORY_LIMIT_BYTES" default:"0"` // 0 = auto-detect
}

// DefaultConfig returns a configuration with sensible production defaults.
// These values are tuned for:
//   - 1GB container
//   - Moderate load (1000-10000 events/sec)
//   - Sub-10ms latency requirements
func DefaultConfig() Config {
	return Config{
		// Governor
		MemoryEnterThreshold: 0.70,
		MemoryExitThreshold:  0.55,
		MemoryCriticalPct:    0.90,
		GovernorPollInterval: 50 * time.Millisecond,

		// Control Lab
		ControlLoopInterval: 3 * time.Second,
		ControlCooldown:     30 * time.Second,
		MaxActionsPerLoop:   1,

		// Queues
		QueueSizeStart: 128,
		QueueSizeMin:   8,
		QueueSizeMax:   1024,
		REDMinFill:     0.6,
		REDMaxDropProb: 0.3,

		// Workers
		TargetLagMs: 10,
		MinWorkers:  2,
		MaxWorkers:  8,

		// Memory
		BufferMemoryBudgetPct: 0.50,

		// PSI
		PSIEnabled:       true,
		PSIThreshold:     0.2,
		PSISustainWindow: 2 * time.Second,
		PSIPollInterval:  1 * time.Second,

		// Flight Recorder
		FlightRecorderSize:     100,
		FlightRecorderInterval: 1 * time.Second,

		// Error Bus
		ErrorBusBufferSize: 32,
		ErrorBusSampling:   false,

		// AIMD
		AIMDIncrStep:   0.05,
		AIMDDecrFactor: 0.5,
		AIMDMaxPerTick: 0.1,

		// Memory Limit (0 = auto-detect)
		MemoryLimitBytes: 0,
	}
}

// LoadFromEnv loads configuration from environment variables.
// Returns a Config with defaults, overridden by any PIPELINE_* env vars found.
func LoadFromEnv() (Config, error) {
	cfg := DefaultConfig()

	// Memory thresholds
	if v := os.Getenv("PIPELINE_MEM_ENTER_PCT"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct > 0 && pct < 1 {
			cfg.MemoryEnterThreshold = pct
		}
	}
	if v := os.Getenv("PIPELINE_MEM_EXIT_PCT"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct > 0 && pct < 1 {
			cfg.MemoryExitThreshold = pct
		}
	}
	if v := os.Getenv("PIPELINE_MEM_CRITICAL_PCT"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct > 0 && pct < 1 {
			cfg.MemoryCriticalPct = pct
		}
	}
	if v := os.Getenv("PIPELINE_GOVERNOR_POLL_MS"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.GovernorPollInterval = d
		}
	}

	// Control lab
	if v := os.Getenv("PIPELINE_CONTROL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ControlLoopInterval = d
		}
	}
	if v := os.Getenv("PIPELINE_CONTROL_COOLDOWN"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.ControlCooldown = d
		}
	}
	if v := os.Getenv("PIPELINE_MAX_ACTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxActionsPerLoop = n
		}
	}

	// Queue sizing
	if v := os.Getenv("PIPELINE_QUEUE_START"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QueueSizeStart = n
		}
	}
	if v := os.Getenv("PIPELINE_QUEUE_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QueueSizeMin = n
		}
	}
	if v := os.Getenv("PIPELINE_QUEUE_MAX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QueueSizeMax = n
		}
	}
	if v := os.Getenv("PIPELINE_RED_MIN_FILL"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct >= 0 && pct <= 1 {
			cfg.REDMinFill = pct
		}
	}
	if v := os.Getenv("PIPELINE_RED_MAX_PROB"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct >= 0 && pct <= 1 {
			cfg.REDMaxDropProb = pct
		}
	}

	// Workers
	if v := os.Getenv("PIPELINE_TARGET_LAG_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.TargetLagMs = n
		}
	}
	if v := os.Getenv("PIPELINE_MIN_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MinWorkers = n
		}
	}
	if v := os.Getenv("PIPELINE_MAX_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxWorkers = n
		}
	}

	// Memory budget
	if v := os.Getenv("PIPELINE_BUFFER_MEMORY_PCT"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct > 0 && pct <= 1 {
			cfg.BufferMemoryBudgetPct = pct
		}
	}

	// PSI
	if v := os.Getenv("PIPELINE_PSI_ENABLED"); v != "" {
		cfg.PSIEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("PIPELINE_PSI_THRESHOLD"); v != "" {
		if pct, err := strconv.ParseFloat(v, 64); err == nil && pct >= 0 && pct <= 1 {
			cfg.PSIThreshold = pct
		}
	}
	if v := os.Getenv("PIPELINE_PSI_SUSTAIN"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PSISustainWindow = d
		}
	}
	if v := os.Getenv("PIPELINE_PSI_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.PSIPollInterval = d
		}
	}

	// Flight recorder
	if v := os.Getenv("PIPELINE_FLIGHT_RECORDER_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.FlightRecorderSize = n
		}
	}
	if v := os.Getenv("PIPELINE_FLIGHT_RECORDER_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.FlightRecorderInterval = d
		}
	}

	// Error bus
	if v := os.Getenv("PIPELINE_ERROR_BUS_BUFFER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ErrorBusBufferSize = n
		}
	}
	if v := os.Getenv("PIPELINE_ERROR_SAMPLING"); v != "" {
		cfg.ErrorBusSampling = v == "true" || v == "1"
	}

	// AIMD
	if v := os.Getenv("PIPELINE_AIMD_INCR"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil && val > 0 {
			cfg.AIMDIncrStep = val
		}
	}
	if v := os.Getenv("PIPELINE_AIMD_DECR"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil && val > 0 && val <= 1 {
			cfg.AIMDDecrFactor = val
		}
	}
	if v := os.Getenv("PIPELINE_AIMD_MAX_TICK"); v != "" {
		if val, err := strconv.ParseFloat(v, 64); err == nil && val > 0 {
			cfg.AIMDMaxPerTick = val
		}
	}

	// Memory limit override
	if v := os.Getenv("PIPELINE_MEMORY_LIMIT_BYTES"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			cfg.MemoryLimitBytes = n
		}
	}

	return cfg, cfg.Validate()
}

// Validate checks that configuration values are sensible.
func (c *Config) Validate() error {
	if c.MemoryEnterThreshold <= c.MemoryExitThreshold {
		return fmt.Errorf("memory enter threshold (%.2f) must be > exit threshold (%.2f)",
			c.MemoryEnterThreshold, c.MemoryExitThreshold)
	}

	if c.MemoryCriticalPct < c.MemoryEnterThreshold {
		return fmt.Errorf("critical threshold (%.2f) must be >= enter threshold (%.2f)",
			c.MemoryCriticalPct, c.MemoryEnterThreshold)
	}

	if c.QueueSizeMin > c.QueueSizeStart {
		return fmt.Errorf("queue min (%d) must be <= start (%d)", c.QueueSizeMin, c.QueueSizeStart)
	}

	if c.QueueSizeStart > c.QueueSizeMax {
		return fmt.Errorf("queue start (%d) must be <= max (%d)", c.QueueSizeStart, c.QueueSizeMax)
	}

	if c.MinWorkers > c.MaxWorkers {
		return fmt.Errorf("min workers (%d) must be <= max workers (%d)", c.MinWorkers, c.MaxWorkers)
	}

	if c.BufferMemoryBudgetPct <= 0 || c.BufferMemoryBudgetPct > 1 {
		return fmt.Errorf("buffer memory budget must be 0 < pct <= 1, got %.2f", c.BufferMemoryBudgetPct)
	}

	if c.REDMinFill >= 1.0 {
		return fmt.Errorf("RED min fill must be < 1.0, got %.2f", c.REDMinFill)
	}

	if c.AIMDDecrFactor <= 0 || c.AIMDDecrFactor > 1 {
		return fmt.Errorf("AIMD decrease factor must be 0 < factor <= 1, got %.2f", c.AIMDDecrFactor)
	}

	return nil
}

// String returns a human-readable summary of the configuration.
func (c *Config) String() string {
	return fmt.Sprintf(`Pipeline Configuration:
  Memory Thresholds:
    Enter Degraded: %.0f%%
    Exit Degraded:  %.0f%%
    Critical:       %.0f%%

  Queues:
    Start Size: %d
    Min Size:   %d
    Max Size:   %d
    RED Start:  %.0f%%

  Workers:
    Min: %d
    Max: %d
    Target Lag: %dms

  Control Lab:
    Interval: %s
    Cooldown: %s

  Memory Limit: %s
`,
		c.MemoryEnterThreshold*100,
		c.MemoryExitThreshold*100,
		c.MemoryCriticalPct*100,
		c.QueueSizeStart,
		c.QueueSizeMin,
		c.QueueSizeMax,
		c.REDMinFill*100,
		c.MinWorkers,
		c.MaxWorkers,
		c.TargetLagMs,
		c.ControlLoopInterval,
		c.ControlCooldown,
		formatMemoryLimit(c.MemoryLimitBytes),
	)
}

func formatMemoryLimit(bytes uint64) string {
	if bytes == 0 {
		return "auto-detect"
	}
	mb := bytes / (1024 * 1024)
	if mb < 1024 {
		return fmt.Sprintf("%d MB", mb)
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1024*1024*1024))
}
