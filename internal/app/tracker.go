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
	ID                int
	PhaseDelay        float64
	Angle             float64
	Peak              float64
	SNR               float64
	Confidence        float64
	Score             float64
	LockState         telemetry.LockState
	History           []float64
	State             TrackLifecycle
	DetectionHistory  []bool
	ConsecutiveHits   int
	ConsecutiveMisses int
	Misses            int
	TotalDetections   int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	LastSeen          time.Time
}

// Detection represents a single observation used to update a track.
type Detection struct {
	ID         int
	PhaseDelay float64
	Angle      float64
	Peak       float64
	SNR        float64
	Confidence float64
	LockState  telemetry.LockState
}

// TrackManager manages creation and lifecycle of tracks.
type TrackManager struct {
	tracks        map[int]*Track
	order         []int
	nextID        int
	maxTracks     int
	timeout       time.Duration
	minSNR        float64
	historyLimit  int
	gate          float64
	confirmHits   int
	confirmWindow int
	maxMisses     int
}

// NewTrackManager creates a track manager with lifecycle controls.
func NewTrackManager(maxTracks int, timeout time.Duration, minSNR float64, historyLimit int) *TrackManager {
	if maxTracks <= 0 {
		maxTracks = 1
	}
	return &TrackManager{
		tracks:        make(map[int]*Track),
		nextID:        1,
		maxTracks:     maxTracks,
		timeout:       timeout,
		minSNR:        minSNR,
		historyLimit:  historyLimit,
		gate:          5.0,
		confirmHits:   3,
		confirmWindow: 5,
		maxMisses:     3,
	}
}

// Update ingests a batch of detections, updates matching tracks, creates new
// ones when capacity allows, and prunes tracks based on timeouts and score.
// Returns the current list of tracks ordered by creation time.
func (tm *TrackManager) Update(detections []Detection, now time.Time) []Track {
	if tm == nil {
		return nil
	}

	tm.expire(now)

	matched := make(map[int]bool, len(detections))
	for _, det := range detections {
		if det.SNR < tm.minSNR {
			continue
		}

		track := tm.findMatch(det.Angle)
		if det.ID > 0 {
			if byID, ok := tm.tracks[det.ID]; ok {
				track = byID
			}
		}

		if track == nil {
			if len(tm.tracks) >= tm.maxTracks {
				tm.pruneExcess()
			}
			track = tm.newTrack(det.Angle, det.PhaseDelay, det.Peak, det.SNR, det.Confidence, det.LockState, now)
		} else {
			tm.updateTrack(track, det.Angle, det.PhaseDelay, det.Peak, det.SNR, det.Confidence, det.LockState, now)
		}
		matched[track.ID] = true
	}

	tm.markUnmatched(matched, now)
	tm.expire(now)
	tm.pruneExcess()

	return tm.Tracks()
}

// Upsert updates the closest matching track or creates a new one if capacity allows.
func (tm *TrackManager) Upsert(angle, phaseDelay, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) *Track {
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
		track = tm.newTrack(angle, phaseDelay, peak, snr, confidence, lock, now)
		tm.markMisses(track.ID, now)
		return track
	}

	tm.markMisses(track.ID, now)

	tm.updateTrack(track, angle, phaseDelay, peak, snr, confidence, lock, now)
	return track
}

// UpdateByID updates an existing track directly when its ID is known, or
// falls back to Upsert when the track is missing.
func (tm *TrackManager) UpdateByID(id int, angle, phaseDelay, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) *Track {
	if tm == nil {
		return nil
	}
	tm.expire(now)
	track, ok := tm.tracks[id]
	if !ok {
		return tm.Upsert(angle, phaseDelay, peak, snr, confidence, lock, now)
	}
	tm.markMisses(track.ID, now)

	tm.updateTrack(track, angle, phaseDelay, peak, snr, confidence, lock, now)
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

// PhaseDelays returns active track IDs and their last known steering delays.
func (tm *TrackManager) PhaseDelays() (ids []int, delays []float64) {
	if tm == nil {
		return nil, nil
	}
	confirmed := make([]*Track, 0, len(tm.tracks))
	tentative := make([]*Track, 0, len(tm.tracks))

	for _, id := range tm.order {
		track, ok := tm.tracks[id]
		if !ok || track.State == TrackLost {
			continue
		}
		if track.State == TrackConfirmed {
			confirmed = append(confirmed, track)
			continue
		}
		tentative = append(tentative, track)
	}

	for _, track := range append(confirmed, tentative...) {
		ids = append(ids, track.ID)
		delays = append(delays, track.PhaseDelay)
	}
	return ids, delays
}

func (tm *TrackManager) newTrack(angle, phaseDelay, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) *Track {
	id := tm.nextID
	tm.nextID++
	track := &Track{
		ID:               id,
		PhaseDelay:       phaseDelay,
		Angle:            angle,
		Peak:             peak,
		SNR:              snr,
		Confidence:       confidence,
		Score:            tm.scoreTrack(snr, confidence, 0),
		LockState:        lock,
		State:            TrackTentative,
		CreatedAt:        now,
		UpdatedAt:        now,
		LastSeen:         now,
		History:          []float64{angle},
		DetectionHistory: []bool{true},
		ConsecutiveHits:  1,
		TotalDetections:  1,
	}
	tm.tracks[id] = track
	tm.order = append(tm.order, id)
	tm.updateLifecycle(track)
	return track
}

func (tm *TrackManager) updateTrack(track *Track, angle, phaseDelay, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) {
	track.Angle = angle
	track.PhaseDelay = phaseDelay
	track.Peak = peak
	track.SNR = snr
	track.Confidence = confidence
	track.LockState = lock
	track.UpdatedAt = now
	track.LastSeen = now
	track.History = append(track.History, angle)
	if tm.historyLimit > 0 && len(track.History) > tm.historyLimit {
		track.History = track.History[len(track.History)-tm.historyLimit:]
	}
	tm.recordDetection(track, true)
	tm.updateLifecycle(track)
}

func (tm *TrackManager) findMatch(angle float64) *Track {
	var (
		best      *Track
		bestDelta = math.MaxFloat64
	)
	for _, track := range tm.tracks {
		if track.State == TrackLost {
			continue
		}
		delta := math.Abs(track.Angle - angle)
		if delta < bestDelta && delta <= tm.gate {
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
	for id, track := range tm.tracks {
		if now.Sub(track.LastSeen) > tm.timeout {
			track.State = TrackLost
			tm.removeTrack(id)
		}
	}
}

func (tm *TrackManager) markMisses(matchedID int, now time.Time) {
	for id, track := range tm.tracks {
		if id == matchedID || track.State == TrackLost {
			continue
		}
		tm.recordDetection(track, false)
		if track.ConsecutiveMisses >= tm.maxMisses {
			track.State = TrackLost
		}
	}
}

func (tm *TrackManager) markUnmatched(matched map[int]bool, now time.Time) {
	for id, track := range tm.tracks {
		if track.State == TrackLost {
			continue
		}
		if matched[id] {
			continue
		}
		tm.recordDetection(track, false)
		if track.ConsecutiveMisses >= tm.maxMisses {
			track.State = TrackLost
		}
	}
}

func (tm *TrackManager) recordDetection(track *Track, hit bool) {
	track.DetectionHistory = append(track.DetectionHistory, hit)
	if tm.confirmWindow > 0 && len(track.DetectionHistory) > tm.confirmWindow {
		track.DetectionHistory = track.DetectionHistory[len(track.DetectionHistory)-tm.confirmWindow:]
	}

	if hit {
		track.ConsecutiveHits++
		track.ConsecutiveMisses = 0
		track.TotalDetections++
	} else {
		track.ConsecutiveMisses++
		track.ConsecutiveHits = 0
		track.Misses++
	}

	track.Score = tm.scoreTrack(track.SNR, track.Confidence, track.ConsecutiveMisses)
}

func (tm *TrackManager) updateLifecycle(track *Track) {
	hits := 0
	for _, detected := range track.DetectionHistory {
		if detected {
			hits++
		}
	}

	if hits >= tm.confirmHits && len(track.DetectionHistory) >= tm.confirmHits {
		track.State = TrackConfirmed
	}

	if track.ConsecutiveMisses >= tm.maxMisses {
		track.State = TrackLost
	}
}

func (tm *TrackManager) removeTrack(id int) {
	delete(tm.tracks, id)
	for i, orderID := range tm.order {
		if orderID == id {
			tm.order = append(tm.order[:i], tm.order[i+1:]...)
			break
		}
	}
}

func (tm *TrackManager) scoreTrack(snr, confidence float64, misses int) float64 {
	snrScore := clamp(snr/30.0, 0, 1)
	confScore := clamp(confidence, 0, 1)
	missPenalty := clamp(1-0.2*float64(misses), 0, 1)
	return 0.6*snrScore + 0.3*confScore + 0.1*missPenalty
}

func (tm *TrackManager) pruneExcess() {
	for len(tm.tracks) > tm.maxTracks {
		var (
			dropID    int
			dropScore = math.MaxFloat64
		)
		for _, track := range tm.tracks {
			if track.State == TrackLost {
				dropID = track.ID
				break
			}
			if track.Score < dropScore {
				dropScore = track.Score
				dropID = track.ID
			}
		}
		if dropID == 0 {
			return
		}
		tm.removeTrack(dropID)
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
	mode      string
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

	t.applyTrackingMode(t.cfg.TrackingMode)

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
	multiMode := t.mode == "multi"
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
			coarsePeaks := dsp.CoarseScanParallel(rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.ScanStep, t.cfg.RxLO, t.cfg.SpacingWavelength, t.dsp)
			if len(coarsePeaks) == 0 {
				t.logger.Warn("coarse scan produced no peaks", logging.Field{Key: "subsystem", Value: "tracker"})
				iteration++
				continue
			}

			primary := coarsePeaks[0]
			delay := primary.Phase
			theta := primary.Angle
			peak := primary.Peak
			monoPhase := primary.MonoPhase
			peakBin := primary.Bin
			snr := primary.SNR
			coarseDuration := time.Since(coarseStart)
			t.lastDelay = delay
			t.appendHistory(theta)

			confidence := t.trackingConfidence(snr, monoPhase)
			state := t.updateLockState(snr, confidence)
			t.lockState = state

			if multiMode && t.manager != nil {
				now := time.Now()
				detections := make([]Detection, 0, min(len(coarsePeaks), t.cfg.MaxTracks))
				for i, pk := range coarsePeaks {
					if i >= t.cfg.MaxTracks {
						break
					}
					conf := t.trackingConfidence(pk.SNR, pk.MonoPhase)
					detections = append(detections, Detection{
						PhaseDelay: pk.Phase,
						Angle:      pk.Angle,
						Peak:       pk.Peak,
						SNR:        pk.SNR,
						Confidence: conf,
						LockState:  state,
					})
				}
				t.manager.Update(detections, now)
			}

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
		// Use shared FFTs with cached DSP
		trackStart := time.Now()
		trackIDs, trackDelays := t.manager.PhaseDelays()
		if !multiMode || t.manager == nil {
			trackDelays = []float64{t.lastDelay}
			trackIDs = []int{-1}
		} else if len(trackDelays) == 0 {
			trackDelays = []float64{t.lastDelay}
			trackIDs = []int{-1}
		}

		targets := make([]dsp.TrackTarget, 0, len(trackDelays))
		for i, delay := range trackDelays {
			id := -1
			if i < len(trackIDs) {
				id = trackIDs[i]
			}
			targets = append(targets, dsp.TrackTarget{ID: id, Delay: delay})
		}

		measurements := dsp.MonopulseTrackParallel(targets, rx0, rx1, t.cfg.PhaseCal, t.startBin, t.endBin, t.cfg.PhaseStep, t.dsp)
		trackDuration := time.Since(trackStart)
		if len(measurements) == 0 {
			t.logger.Warn("tracking produced no measurements", logging.Field{Key: "subsystem", Value: "tracker"})
			iteration++
			continue
		}

		bestIdx := 0
		for i := 1; i < len(measurements); i++ {
			if measurements[i].SNR > measurements[bestIdx].SNR {
				bestIdx = i
			}
		}

		best := measurements[bestIdx]
		theta := dsp.PhaseToTheta(best.Delay, t.cfg.RxLO, t.cfg.SpacingWavelength)
		confidence := t.trackingConfidence(best.SNR, best.MonoPhase)
		state := t.updateLockState(best.SNR, confidence)
		t.lockState = state
		t.lastDelay = best.Delay
		t.appendHistory(theta)

		now := time.Now()
		if multiMode && t.manager != nil {
			detections := make([]Detection, 0, len(measurements))
			for i, m := range measurements {
				angle := dsp.PhaseToTheta(m.Delay, t.cfg.RxLO, t.cfg.SpacingWavelength)
				conf := t.trackingConfidence(m.SNR, m.MonoPhase)
				trackID := -1
				if i < len(trackIDs) {
					trackID = trackIDs[i]
				}
				detections = append(detections, Detection{
					ID:         trackID,
					PhaseDelay: m.Delay,
					Angle:      angle,
					Peak:       m.Peak,
					SNR:        m.SNR,
					Confidence: conf,
					LockState:  state,
				})
			}
			t.manager.Update(detections, now)
		}

		var debug *telemetry.DebugInfo
		if t.cfg.DebugMode {
			debug = &telemetry.DebugInfo{
				PhaseDelayDeg:     best.Delay,
				MonopulsePhaseRad: best.MonoPhase,
				Peak: telemetry.PeakDebug{
					Value: best.Peak,
					Bin:   best.PeakBin,
					Band:  [2]int{t.startBin, t.endBin},
				},
			}
		}

		if t.reporter != nil {
			t.reporter.Report(theta, best.Peak, best.SNR, confidence, state, debug)
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

func (t *Tracker) applyTrackingMode(mode string) {
	prevMode := t.mode

	if mode != "multi" {
		mode = "single"
	}

	if prevMode != mode {
		t.history = nil
		t.lastDelay = 0
		t.lockState = telemetry.LockStateSearching
		t.stableCnt = 0
		t.dropCnt = 0
	}

	if mode == "multi" {
		t.manager = NewTrackManager(t.cfg.MaxTracks, t.cfg.TrackTimeout, t.cfg.MinSNRThreshold, t.cfg.HistoryLimit)
	} else {
		t.manager = nil
	}

	t.mode = mode
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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

func (t *Tracker) updateTracks(trackID int, theta, delay, peak, snr, confidence float64, lock telemetry.LockState, now time.Time) {
	if t.manager == nil {
		return
	}
	if trackID > 0 {
		t.manager.UpdateByID(trackID, theta, delay, peak, snr, confidence, lock, now)
		return
	}
	t.manager.Upsert(theta, delay, peak, snr, confidence, lock, now)
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
