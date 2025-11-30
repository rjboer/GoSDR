package app

import (
	"context"
	"fmt"
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
	TrackingMode      string
	MaxTracks         int
	TrackTimeout      time.Duration
	MinSNRThreshold   float64
}

// TrackLifecycle represents the lifecycle of a track.
type TrackLifecycle int

const (
	TrackTentative TrackLifecycle = iota
	TrackConfirmed
	TrackLost
)

// Track holds state for a single target being tracked.
type Track struct {
	ID         int
	Angle      float64
	Peak       float64
	SNR        float64
	Confidence float64
	LockState  telemetry.LockState
	History    []float64
	State      TrackLifecycle
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastSeen   time.Time
}

// TrackManager manages creation and lifecycle of tracks.
type TrackManager struct {
	tracks       map[int]*Track
	order        []int
	nextID       int
	maxTracks    int
	timeout      time.Duration
	minSNR       float64
	historyLimit int
}

// NewTrackManager creates a track manager with lifecycle controls.
func NewTrackManager(maxTracks int, timeout time.Duration, minSNR float64, historyLimit int) *TrackManager {
	if maxTracks <= 0 {
		maxTracks = 1
	}
	return &TrackManager{
		tracks:       make(map[int]*Track),
		nextID:       1,
		maxTracks:    maxTracks,
		timeout:      timeout,
		minSNR:       minSNR,
		historyLimit: historyLimit,
	}
}

// Upsert updates the closest matching track or creates a new one if capacity allows.
func (tm *TrackManager) Upsert(angle, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) *Track {
	if tm == nil {
		return nil
	}
	tm.expire(now)
	if snr < tm.minSNR {
		return nil
	}

	track := tm.findMatch(angle)
	if track == nil {
		if len(tm.tracks) >= tm.maxTracks {
			tm.dropOldest()
		}
		track = tm.newTrack(angle, peak, snr, confidence, lock, now)
	} else {
		track.Angle = angle
		track.Peak = peak
		track.SNR = snr
		track.Confidence = confidence
		track.LockState = lock
		track.State = tm.nextLifecycle(track.State)
		track.UpdatedAt = now
		track.LastSeen = now
		track.History = append(track.History, angle)
		if tm.historyLimit > 0 && len(track.History) > tm.historyLimit {
			track.History = track.History[len(track.History)-tm.historyLimit:]
		}
	}

	return track
}

// Tracks returns a copy of managed tracks ordered by creation.
func (tm *TrackManager) Tracks() []Track {
	if tm == nil {
		return nil
	}
	result := make([]Track, 0, len(tm.tracks))
	for _, id := range tm.order {
		if track, ok := tm.tracks[id]; ok {
			result = append(result, *track)
		}
	}
	return result
}

func (tm *TrackManager) newTrack(angle, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) *Track {
	id := tm.nextID
	tm.nextID++
	track := &Track{
		ID:         id,
		Angle:      angle,
		Peak:       peak,
		SNR:        snr,
		Confidence: confidence,
		LockState:  lock,
		State:      TrackTentative,
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeen:   now,
		History:    []float64{angle},
	}
	tm.tracks[id] = track
	tm.order = append(tm.order, id)
	return track
}

func (tm *TrackManager) findMatch(angle float64) *Track {
	const angleMatchThreshold = 5.0
	var (
		best      *Track
		bestDelta = math.MaxFloat64
	)
	for _, track := range tm.tracks {
		if track.State == TrackLost {
			continue
		}
		delta := math.Abs(track.Angle - angle)
		if delta < bestDelta && delta <= angleMatchThreshold {
			best = track
			bestDelta = delta
		}
	}
	return best
}

func (tm *TrackManager) dropOldest() {
	for len(tm.order) > 0 {
		id := tm.order[0]
		tm.order = tm.order[1:]
		if _, ok := tm.tracks[id]; ok {
			delete(tm.tracks, id)
			return
		}
	}
}

func (tm *TrackManager) expire(now time.Time) {
	if tm.timeout <= 0 {
		return
	}
	for _, track := range tm.tracks {
		if track.State == TrackLost {
			continue
		}
		if now.Sub(track.LastSeen) > tm.timeout {
			track.State = TrackLost
		}
	}
}

func (tm *TrackManager) nextLifecycle(current TrackLifecycle) TrackLifecycle {
	switch current {
	case TrackTentative:
		return TrackConfirmed
	case TrackConfirmed:
		return TrackConfirmed
	case TrackLost:
		return TrackLost
	default:
		return TrackTentative
	}
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
	manager   *TrackManager
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
	if t.cfg.TrackingMode == "" {
		t.cfg.TrackingMode = "single"
	}
	if t.cfg.MaxTracks == 0 {
		if t.cfg.TrackingMode == "multi" {
			t.cfg.MaxTracks = 10
		} else {
			t.cfg.MaxTracks = 1
		}
	}
	if t.cfg.TrackTimeout == 0 {
		t.cfg.TrackTimeout = 3 * time.Second
	}
	if t.cfg.MinSNRThreshold == 0 {
		t.cfg.MinSNRThreshold = 3
	}
	t.manager = NewTrackManager(t.cfg.MaxTracks, t.cfg.TrackTimeout, t.cfg.MinSNRThreshold, t.cfg.HistoryLimit)
	// Update cached DSP size if needed
	t.dsp.UpdateSize(t.cfg.NumSamples)
	if err := t.sdr.Init(ctx, sdr.Config{
		SampleRate: t.cfg.SampleRate,
		RxLO:       t.cfg.RxLO,
		RxGain0:    t.cfg.RxGain0,
		RxGain1:    t.cfg.RxGain1,
		TxGain:     t.cfg.TxGain,
		ToneOffset: t.cfg.ToneOffset,
		NumSamples: t.cfg.NumSamples,
		PhaseDelta: t.cfg.PhaseDelta,
	}); err != nil {
		return fmt.Errorf("init SDR: %w", err)
	}
	return nil
}

// Run executes a coarse scan and then a monopulse tracking loop.
// Runs continuously until context is canceled.
func (t *Tracker) Run(ctx context.Context) error {
	if t.cfg.TrackingLength == 0 {
		t.cfg.TrackingLength = 50
	}
	if err := t.warmup(ctx); err != nil {
		return fmt.Errorf("warmup: %w", err)
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

		iterationStart := time.Now()
		rx0, rx1, err := t.sdr.RX(ctx)
		if err != nil {
			return fmt.Errorf("receive samples: %w", err)
		}
		if len(rx0) == 0 || len(rx1) == 0 {
			t.logger.Warn("received empty buffer", logging.Field{Key: "subsystem", Value: "tracker"})
			continue
		}

		// First iteration: coarse scan
		if iteration == 0 {
			coarseStart := time.Now()
			// Use parallel coarse scan with cached DSP
			delay, theta, peak, monoPhase, peakBin, snr := dsp.CoarseScanParallel(rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.ScanStep, t.cfg.RxLO, t.cfg.SpacingWavelength, t.dsp)
			coarseDuration := time.Since(coarseStart)
			t.lastDelay = delay
			t.appendHistory(theta)

			confidence := t.trackingConfidence(snr, monoPhase)
			state := t.updateLockState(snr, confidence)
			t.updateTracks(theta, peak, snr, confidence)

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
			t.logger.Debug("coarse scan iteration", logging.Field{Key: "iteration", Value: iteration}, logging.Field{Key: "duration_ms", Value: coarseDuration.Seconds() * 1000})
			iteration++
			t.logger.Debug("iteration complete", logging.Field{Key: "iteration", Value: iteration}, logging.Field{Key: "elapsed_ms", Value: time.Since(iterationStart).Seconds() * 1000})
			continue
		}

		// Subsequent iterations: monopulse tracking
		// Use parallel tracking with cached DSP
		trackStart := time.Now()
		var peak, monoPhase, snr float64
		var peakBin int
		t.lastDelay, peak, monoPhase, snr, peakBin = dsp.MonopulseTrackParallel(t.lastDelay, rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.PhaseStep, t.dsp)
		trackDuration := time.Since(trackStart)
		theta := dsp.PhaseToTheta(t.lastDelay, t.cfg.RxLO, t.cfg.SpacingWavelength)
		t.appendHistory(theta)

		confidence := t.trackingConfidence(snr, monoPhase)
		state := t.updateLockState(snr, confidence)
		t.updateTracks(theta, peak, snr, confidence)

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
		t.logger.Debug("tracking iteration", logging.Field{Key: "iteration", Value: iteration}, logging.Field{Key: "duration_ms", Value: trackDuration.Seconds() * 1000})
		iteration++
		t.logger.Debug("iteration complete", logging.Field{Key: "iteration", Value: iteration}, logging.Field{Key: "elapsed_ms", Value: time.Since(iterationStart).Seconds() * 1000})
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

func (t *Tracker) updateTracks(theta, peak, snr, confidence float64) {
	if t.manager == nil {
		return
	}
	t.manager.Upsert(theta, peak, snr, confidence, t.lockState, time.Now())
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
		warmupStart := time.Now()
		if _, _, err := t.sdr.RX(ctx); err != nil {
			return fmt.Errorf("warmup RX buffer %d: %w", i, err)
		}
		t.logger.Debug("warmup buffer processed", logging.Field{Key: "index", Value: i}, logging.Field{Key: "duration_ms", Value: time.Since(warmupStart).Seconds() * 1000})
	}
	return nil
}
