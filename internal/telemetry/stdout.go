package telemetry

import "github.com/rjboer/GoSDR/internal/logging"

// Reporter captures telemetry events.
type Reporter interface {
	Report(angleDeg float64, peak float64, debug *DebugInfo)
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

func (r StdoutReporter) Report(angleDeg float64, peak float64, debug *DebugInfo) {
	fields := []logging.Field{
		{Key: "subsystem", Value: "telemetry"},
		{Key: "angle_deg", Value: angleDeg},
	}
	if peak != 0 {
		fields = append(fields, logging.Field{Key: "peak_dbfs", Value: peak})
	}
	if debug != nil {
		fields = append(fields,
			logging.Field{Key: "phase_delay_deg", Value: debug.PhaseDelayDeg},
			logging.Field{Key: "monopulse_phase_rad", Value: debug.MonopulsePhaseRad},
			logging.Field{Key: "peak_bin", Value: debug.Peak.Bin},
		)
	}
	r.logger.Info("telemetry sample", fields...)
}
