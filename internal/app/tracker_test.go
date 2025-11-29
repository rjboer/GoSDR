package app

import (
	"context"
	"io"
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/rjboer/GoSDR/internal/logging"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

type recordingReporter struct {
	angles []float64
}

func (r *recordingReporter) Report(angleDeg float64, _ float64, _ float64, _ float64, _ telemetry.LockState, _ *telemetry.DebugInfo) {
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
		WarmupBuffers:     0,
		HistoryLimit:      20,
	}
	tracker := NewTracker(backend, reporter, logging.New(logging.Info, logging.Text, io.Discard), cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := tracker.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Tracker now runs continuously, so it will timeout
	err := tracker.Run(ctx)
	if err != nil && err != context.DeadlineExceeded {
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

	// History should have at least some entries (not exact count since continuous)
	if got := len(tracker.AngleHistory()); got < 10 {
		t.Fatalf("expected at least 10 history entries got %d", got)
	}
}
