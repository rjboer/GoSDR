package app

import (
	"context"
	"log"
	"time"

	"github.com/rjboer/GoSDR/internal/dsp"
	"github.com/rjboer/GoSDR/internal/sdr"
	"github.com/rjboer/GoSDR/internal/telemetry"
)

// Config captures application level configuration.
type Config struct {
	SampleRate        float64
	RxLO              float64
	ToneOffset        float64
	NumSamples        int
	SpacingWavelength float64
	TrackingLength    int
	PhaseStep         float64
	PhaseCal          float64
	ScanStep          float64
	PhaseDelta        float64
}

// Tracker wires SDR input into the DSP monopulse tracking loop.
type Tracker struct {
	sdr       sdr.SDR
	reporter  telemetry.Reporter
	cfg       Config
	startBin  int
	endBin    int
	lastDelay float64
}

func NewTracker(backend sdr.SDR, reporter telemetry.Reporter, cfg Config) *Tracker {
	return &Tracker{sdr: backend, reporter: reporter, cfg: cfg}
}

// Init configures the SDR and precomputes FFT bin indices.
func (t *Tracker) Init(ctx context.Context) error {
	start, end := dsp.SignalBinRange(t.cfg.NumSamples, t.cfg.SampleRate, t.cfg.ToneOffset)
	t.startBin = start
	t.endBin = end
	if t.cfg.ScanStep == 0 {
		t.cfg.ScanStep = 2
	}
	if t.cfg.PhaseStep == 0 {
		t.cfg.PhaseStep = 1
	}
	return t.sdr.Init(ctx, sdr.Config{
		SampleRate: t.cfg.SampleRate,
		RxLO:       t.cfg.RxLO,
		ToneOffset: t.cfg.ToneOffset,
		NumSamples: t.cfg.NumSamples,
		PhaseDelta: t.cfg.PhaseDelta,
	})
}

// Run executes a coarse scan and then a monopulse tracking loop.
func (t *Tracker) Run(ctx context.Context) error {
	if t.cfg.TrackingLength == 0 {
		t.cfg.TrackingLength = 50
	}
	for i := 0; i < t.cfg.TrackingLength; i++ {
		rx0, rx1, err := t.sdr.RX(ctx)
		if err != nil {
			return err
		}
		if len(rx0) == 0 || len(rx1) == 0 {
			log.Printf("received empty buffer")
			continue
		}
		if i == 0 {
			delay, theta, peak := dsp.CoarseScan(rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.ScanStep, t.cfg.RxLO, t.cfg.SpacingWavelength)
			t.lastDelay = delay
			if t.reporter != nil {
				t.reporter.Report(theta, peak)
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		t.lastDelay = dsp.MonopulseTrack(t.lastDelay, rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.PhaseStep)
		theta := dsp.PhaseToTheta(t.lastDelay, t.cfg.RxLO, t.cfg.SpacingWavelength)
		if t.reporter != nil {
			t.reporter.Report(theta, 0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
