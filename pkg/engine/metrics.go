package engine

import (
	"time"

	"github.com/BYTE-6D65/pipeline/pkg/telemetry"
)

func recordEngineOperation(metrics *telemetry.Metrics, operation string, start time.Time, err error) {
	if metrics == nil {
		return
	}

	status := "success"
	if err != nil {
		status = "error"
	}

	metrics.EngineOperations.WithLabelValues(operation, status).Inc()
	metrics.EngineDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}
