package telemetry

import "github.com/rjboer/GoSDR/internal/logging"

// Reporter captures telemetry events.
type Reporter interface {
	Report(angleDeg float64, peak float64)
}

// StdoutReporter prints tracking updates to stdout.
type StdoutReporter struct {
	logger logging.Logger
}

// NewStdoutReporter builds a stdout reporter with the provided logger.
func NewStdoutReporter(logger logging.Logger) StdoutReporter {
	if logger == nil {
		logger = logging.Default()
	}
	return StdoutReporter{logger: logger}
}

func (r StdoutReporter) Report(angleDeg float64, peak float64) {
	fields := []logging.Field{
		{Key: "subsystem", Value: "telemetry"},
		{Key: "angle_deg", Value: angleDeg},
	}
	if peak != 0 {
		fields = append(fields, logging.Field{Key: "peak_dbfs", Value: peak})
		r.logger.Info("telemetry sample", fields...)
		return
	}
	r.logger.Info("telemetry sample", fields...)
}
