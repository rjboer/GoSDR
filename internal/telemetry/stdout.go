package telemetry

import (
	"fmt"

	"github.com/rjboer/GoSDR/internal/logging"
)

// Reporter captures telemetry events.
type Reporter interface {
	Report(angleDeg float64, peak float64, snr float64, confidence float64, lockState LockState, debug *DebugInfo)
	ReportMultiTrack(sample MultiTrackSample)
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

func (r StdoutReporter) Report(angleDeg float64, peak float64, snr float64, confidence float64, lockState LockState, debug *DebugInfo) {
	fields := []logging.Field{
		{Key: "subsystem", Value: "telemetry"},
		{Key: "angle_deg", Value: angleDeg},
	}
	if peak != 0 {
		fields = append(fields, logging.Field{Key: "peak_dbfs", Value: peak})
	}
	if snr != 0 {
		fields = append(fields, logging.Field{Key: "snr_db", Value: snr})
	}
	if confidence != 0 {
		fields = append(fields, logging.Field{Key: "tracking_confidence", Value: confidence})
	}
	if lockState != "" {
		fields = append(fields, logging.Field{Key: "lock_state", Value: lockState})
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

// ReportMultiTrack prints multi-track telemetry data while preserving the
// single-track code path for convenience.
func (r StdoutReporter) ReportMultiTrack(sample MultiTrackSample) {
	if len(sample.Tracks) == 0 {
		return
	}

	if len(sample.Tracks) == 1 {
		track := sample.Tracks[0]
		r.Report(track.AngleDeg, track.Peak, track.SNR, track.Confidence, track.LockState, track.Debug)
		return
	}

	fields := []logging.Field{{Key: "subsystem", Value: "telemetry"}, {Key: "track_count", Value: len(sample.Tracks)}}
	for idx, track := range sample.Tracks {
		fields = append(fields, logging.Field{
			Key:   fmt.Sprintf("track_%d", idx),
			Value: track,
		})
	}

	r.logger.Info("telemetry multi-track sample", fields...)
}
