package app

import (
	"context"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/rjboer/GoSDR/internal/sdr"
)

type recordingReporter struct {
	angles []float64
}

func (r *recordingReporter) Report(angleDeg float64, _ float64) {
	r.angles = append(r.angles, angleDeg)
}

func TestTrackerConvergesWithMock(t *testing.T) {
	rand.Seed(3)
	backend := sdr.NewMock()
	reporter := &recordingReporter{}
	cfg := Config{
		SampleRate:        2e6,
		RxLO:              2.3e9,
		ToneOffset:        200e3,
		NumSamples:        512,
		SpacingWavelength: 0.5,
		TrackingLength:    12,
		PhaseStep:         1,
		ScanStep:          2,
		PhaseDelta:        35,
	}
	tracker := NewTracker(backend, reporter, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := tracker.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := tracker.Run(ctx); err != nil {
		t.Fatalf("run failed: %v", err)
	}

	if len(reporter.angles) == 0 {
		t.Fatalf("expected telemetry output")
	}

	expectedDelay := -cfg.PhaseDelta
	finalDelay := tracker.LastDelay()
	if math.Abs(finalDelay-expectedDelay) > 5 {
		t.Fatalf("expected delay near %.2f got %.2f", expectedDelay, finalDelay)
	}
}
