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
	RxGain0           int
	RxGain1           int
	TxGain            int
	ToneOffset        float64
	NumSamples        int
	SpacingWavelength float64
	TrackingLength    int
	PhaseStep         float64
	PhaseCal          float64
	ScanStep          float64
	PhaseDelta        float64
	WarmupBuffers     int
	HistoryLimit      int
}

// Tracker wires SDR input into the DSP monopulse tracking loop.
type Tracker struct {
	sdr       sdr.SDR
	reporter  telemetry.Reporter
	cfg       Config
	startBin  int
	endBin    int
	lastDelay float64
	history   []float64
	dsp       *dsp.CachedDSP // Cached DSP resources for performance
}

func NewTracker(backend sdr.SDR, reporter telemetry.Reporter, cfg Config) *Tracker {
	return &Tracker{
		sdr:      backend,
		reporter: reporter,
		cfg:      cfg,
		dsp:      dsp.NewCachedDSP(cfg.NumSamples),
	}
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
	if t.cfg.WarmupBuffers == 0 {
		t.cfg.WarmupBuffers = 3
	}
	if t.cfg.HistoryLimit == 0 {
		t.cfg.HistoryLimit = t.cfg.TrackingLength
	}
	// Update cached DSP size if needed
	t.dsp.UpdateSize(t.cfg.NumSamples)
	return t.sdr.Init(ctx, sdr.Config{
		SampleRate: t.cfg.SampleRate,
		RxLO:       t.cfg.RxLO,
		RxGain0:    t.cfg.RxGain0,
		RxGain1:    t.cfg.RxGain1,
		TxGain:     t.cfg.TxGain,
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
	if err := t.warmup(ctx); err != nil {
		return err
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
			// Use parallel coarse scan with cached DSP
			delay, theta, peak := dsp.CoarseScanParallel(rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.ScanStep, t.cfg.RxLO, t.cfg.SpacingWavelength, t.dsp)
			t.lastDelay = delay
			t.appendHistory(theta)
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
		// Use parallel tracking with cached DSP
		var peak float64
		t.lastDelay, peak = dsp.MonopulseTrackParallel(t.lastDelay, rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.PhaseStep, t.dsp)
		theta := dsp.PhaseToTheta(t.lastDelay, t.cfg.RxLO, t.cfg.SpacingWavelength)
		t.appendHistory(theta)
		if t.reporter != nil {
			t.reporter.Report(theta, peak)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// LastDelay returns the most recent phase delay used by the tracker.
func (t *Tracker) LastDelay() float64 {
	return t.lastDelay
}

// AngleHistory returns the collected steering angles from coarse scan and monopulse updates.
func (t *Tracker) AngleHistory() []float64 {
	out := make([]float64, len(t.history))
	copy(out, t.history)
	return out
}

func (t *Tracker) appendHistory(theta float64) {
	t.history = append(t.history, theta)
	if len(t.history) > t.cfg.HistoryLimit && t.cfg.HistoryLimit > 0 {
		t.history = t.history[len(t.history)-t.cfg.HistoryLimit:]
	}
}

func (t *Tracker) warmup(ctx context.Context) error {
	if t.cfg.WarmupBuffers <= 0 {
		return nil
	}
	for i := 0; i < t.cfg.WarmupBuffers; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if _, _, err := t.sdr.RX(ctx); err != nil {
			return err
		}
	}
	return nil
}
