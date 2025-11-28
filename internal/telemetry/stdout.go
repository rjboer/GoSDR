package telemetry

import "log"

// Reporter captures telemetry events.
type Reporter interface {
	Report(angleDeg float64, peak float64)
}

// StdoutReporter prints tracking updates to stdout.
type StdoutReporter struct{}

func (StdoutReporter) Report(angleDeg float64, peak float64) {
	if peak != 0 {
		log.Printf("steering angle=%.2f deg peak=%.2f dBFS", angleDeg, peak)
		return
	}
	log.Printf("steering angle=%.2f deg", angleDeg)
}
