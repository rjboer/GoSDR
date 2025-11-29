package app

import (
	"context"
	"math"
	"time"

	"github.com/rjboer/GoSDR/internal/dsp"
	"github.com/rjboer/GoSDR/internal/logging"
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
	DebugMode         bool
}

// Tracker wires SDR input into the DSP monopulse tracking loop.
type Tracker struct {
	sdr       sdr.SDR
	reporter  telemetry.Reporter
	logger    logging.Logger
	cfg       Config
	startBin  int
	endBin    int
	lastDelay float64
	history   []float64
	dsp       *dsp.CachedDSP // Cached DSP resources for performance
	lockState telemetry.LockState
	stableCnt int
	dropCnt   int
}

func NewTracker(backend sdr.SDR, reporter telemetry.Reporter, logger logging.Logger, cfg Config) *Tracker {
	if logger == nil {
		logger = logging.Default()
	}
	return &Tracker{
		sdr:       backend,
		reporter:  reporter,
		logger:    logger,
		cfg:       cfg,
		dsp:       dsp.NewCachedDSP(cfg.NumSamples),
		lockState: telemetry.LockStateSearching,
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
// Runs continuously until context is canceled.
func (t *Tracker) Run(ctx context.Context) error {
	if t.cfg.TrackingLength == 0 {
		t.cfg.TrackingLength = 50
	}
	if err := t.warmup(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	// Run continuously
	iteration := 0
	for {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Continue to next iteration
		}

		rx0, rx1, err := t.sdr.RX(ctx)
		if err != nil {
			return err
		}
		if len(rx0) == 0 || len(rx1) == 0 {
			t.logger.Warn("received empty buffer", logging.Field{Key: "subsystem", Value: "tracker"})
			continue
		}

		// First iteration: coarse scan
		if iteration == 0 {
			// Use parallel coarse scan with cached DSP
			delay, theta, peak, monoPhase, peakBin, snr := dsp.CoarseScanParallel(rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.ScanStep, t.cfg.RxLO, t.cfg.SpacingWavelength, t.dsp)
			t.lastDelay = delay
			t.appendHistory(theta)

			confidence := t.trackingConfidence(snr, monoPhase)
			state := t.updateLockState(snr, confidence)

			var debug *telemetry.DebugInfo
			if t.cfg.DebugMode {
				debug = &telemetry.DebugInfo{
					PhaseDelayDeg:     delay,
					MonopulsePhaseRad: monoPhase,
					Peak: telemetry.PeakDebug{
						Value: peak,
						Bin:   peakBin,
						Band:  [2]int{t.startBin, t.endBin},
					},
				}
			}

			if t.reporter != nil {
				t.reporter.Report(theta, peak, snr, confidence, state, debug)
			}
			iteration++
			continue
		}

		// Subsequent iterations: monopulse tracking
		// Use parallel tracking with cached DSP
		var peak, monoPhase, snr float64
		var peakBin int
		t.lastDelay, peak, monoPhase, snr, peakBin = dsp.MonopulseTrackParallel(t.lastDelay, rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.PhaseStep, t.dsp)
		theta := dsp.PhaseToTheta(t.lastDelay, t.cfg.RxLO, t.cfg.SpacingWavelength)
		t.appendHistory(theta)

		confidence := t.trackingConfidence(snr, monoPhase)
		state := t.updateLockState(snr, confidence)

		var debug *telemetry.DebugInfo
		if t.cfg.DebugMode {
			debug = &telemetry.DebugInfo{
				PhaseDelayDeg:     t.lastDelay,
				MonopulsePhaseRad: monoPhase,
				Peak: telemetry.PeakDebug{
					Value: peak,
					Bin:   peakBin,
					Band:  [2]int{t.startBin, t.endBin},
				},
			}
		}

		if t.reporter != nil {
			t.reporter.Report(theta, peak, snr, confidence, state, debug)
		}
		iteration++
	}
}

func (t *Tracker) trackingConfidence(snr float64, monoPhase float64) float64 {
	snrScore := clamp((snr)/30.0, 0, 1)
	monoScore := clamp(1-math.Min(math.Abs(monoPhase)/(10*(math.Pi/180)), 1), 0, 1)
	confidence := 0.7*snrScore + 0.3*monoScore
	if confidence < 0 {
		return 0
	}
	if confidence > 1 {
		return 1
	}
	return confidence
}

func (t *Tracker) updateLockState(snr float64, confidence float64) telemetry.LockState {
	const (
		acquireSNR     = 6.0
		lockSNR        = 12.0
		dropSNR        = 4.0
		lockConfidence = 0.6
		acquireConf    = 0.3
		stableNeeded   = 3
		dropNeeded     = 2
	)

	switch t.lockState {
	case telemetry.LockStateLocked:
		if snr < dropSNR || confidence < acquireConf {
			t.dropCnt++
			if t.dropCnt >= dropNeeded {
				t.lockState = telemetry.LockStateTracking
				t.stableCnt = 0
			}
		} else {
			t.dropCnt = 0
		}
	case telemetry.LockStateTracking:
		if snr >= lockSNR && confidence >= lockConfidence {
			t.stableCnt++
			if t.stableCnt >= stableNeeded {
				t.lockState = telemetry.LockStateLocked
				t.dropCnt = 0
			}
		} else if snr < dropSNR || confidence < acquireConf {
			t.dropCnt++
			if t.dropCnt >= dropNeeded {
				t.lockState = telemetry.LockStateSearching
				t.stableCnt = 0
			}
		} else {
			t.stableCnt = 0
			t.dropCnt = 0
		}
	default:
		if snr >= acquireSNR && confidence >= acquireConf {
			t.lockState = telemetry.LockStateTracking
			t.stableCnt = 0
			t.dropCnt = 0
		}
	}
	return t.lockState
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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
