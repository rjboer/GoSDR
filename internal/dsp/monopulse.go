package dsp

import (
	"math"
	"math/cmplx"
	"runtime"
	"sort"
)

const (
	degToRad        = math.Pi / 180.0
	monoDeadbandRad = 0.5 * math.Pi / 180.0 // ~0.5° deadband for tracking
)

// scanResult is used by the worker-pool coarse scan.
type scanResult struct {
	idx       int
	phase     float64
	peak      float64
	monoPhase float64
	snr       float64
	peakBin   int
	ok        bool
}

// Peak captures the metadata for a detected spectral peak.
type Peak struct {
	Bin        int
	Level      float64
	Prominence float64
	SNR        float64
}

// PeakInfo captures metadata for a detected coarse-scan candidate.
type PeakInfo struct {
	// Phase is the steering phase delay (degrees) used during the scan.
	Phase float64
	// Angle is the corresponding estimated angle of arrival (degrees).
	Angle float64
	// Peak is the peak dBFS value observed in the sum spectrum.
	Peak float64
	// SNR is the estimated signal-to-noise ratio within the search band.
	SNR float64
	// Bin is the FFT bin associated with the detected peak.
	Bin int
	// MonoPhase is the monopulse phase (radians) computed for this delay.
	MonoPhase float64
}

// TrackMeasurement captures the per-target results of a monopulse tracking
// update.
type TrackMeasurement struct {
	Delay     float64
	Peak      float64
	MonoPhase float64
	SNR       float64
	PeakBin   int
}

// binRange clamps [start,end) to [0,n).
// If the resulting interval is empty, it returns (0,0).
func binRange(n, start, end int) (int, int) {
	if n <= 0 {
		return 0, 0
	}
	if start < 0 {
		start = 0
	}
	if end <= 0 || end > n {
		end = n
	}
	if start >= end {
		return 0, 0
	}
	return start, end
}

// peakInBand returns the maximum value of db in [start,end).
// ok is false if the band is empty or db is empty.
// bin reports the index of the peak within db (or 0 if unavailable).
func peakInBand(db []float64, start, end int) (peak float64, bin int, ok bool) {
	s, e := binRange(len(db), start, end)
	if s == e {
		return 0, 0, false
	}
	peak = -math.MaxFloat64
	for i := s; i < e; i++ {
		if db[i] > peak {
			peak = db[i]
			bin = i
		}
	}
	if peak == -math.MaxFloat64 {
		return 0, bin, false
	}
	return peak, bin, true
}

// noiseFloor computes the average power in [start, end) excluding a small guard
// region around the signal bin to avoid biasing the estimate.
func noiseFloor(db []float64, start, end, signalBin int) (float64, bool) {
	s, e := binRange(len(db), start, end)
	if s == e {
		return 0, false
	}

	var sum float64
	var count int
	for i := s; i < e; i++ {
		if i == signalBin || i == signalBin-1 || i == signalBin+1 {
			continue
		}
		v := db[i]
		if math.IsInf(v, 0) || math.IsNaN(v) {
			continue
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

// estimateSNR computes the SNR as peak - noise floor for the given band.
func estimateSNR(db []float64, peak float64, peakBin int, start, end int) float64 {
	noise, ok := noiseFloor(db, start, end, peakBin)
	if !ok {
		return 0
	}
	snr := peak - noise
	if math.IsNaN(snr) || math.IsInf(snr, 0) {
		return 0
	}
	return snr
}

// FindMultiplePeaks returns local maxima whose prominence exceeds the given threshold.
// Prominence is measured as the drop from the peak to the highest valley on either side
// before encountering a higher peak (or the boundary). Peaks are returned in descending
// SNR/level order. minSeparation enforces a minimum bin distance between reported peaks.
func FindMultiplePeaks(db []float64, prominence float64, minSeparation int) []Peak {
	if len(db) == 0 {
		return nil
	}

	if minSeparation < 0 {
		minSeparation = 0
	}

	// Identify local maxima (handling boundaries).
	var candidates []int
	for i, v := range db {
		leftOK := i == 0 || v > db[i-1]
		rightOK := i == len(db)-1 || v >= db[i+1]
		if leftOK && rightOK {
			candidates = append(candidates, i)
		}
	}

	var peaks []Peak
	for _, idx := range candidates {
		val := db[idx]

		leftMin := val
		for l := idx - 1; l >= 0; l-- {
			if db[l] > val {
				break
			}
			if db[l] < leftMin {
				leftMin = db[l]
			}
		}

		rightMin := val
		for r := idx + 1; r < len(db); r++ {
			if db[r] > val {
				break
			}
			if db[r] < rightMin {
				rightMin = db[r]
			}
		}

		prom := val - math.Max(leftMin, rightMin)
		if prom < prominence {
			continue
		}

		snr := prom
		peaks = append(peaks, Peak{Bin: idx, Level: val, Prominence: prom, SNR: snr})
	}

	// Sort by SNR/level descending so greedy spacing keeps strongest peaks.
	sort.Slice(peaks, func(i, j int) bool {
		if peaks[i].SNR == peaks[j].SNR {
			return peaks[i].Level > peaks[j].Level
		}
		return peaks[i].SNR > peaks[j].SNR
	})

	if minSeparation == 0 {
		return peaks
	}

	var filtered []Peak
	for _, p := range peaks {
		tooClose := false
		for _, f := range filtered {
			if abs := p.Bin - f.Bin; abs < 0 {
				if -abs < minSeparation {
					tooClose = true
					break
				}
			} else if abs < minSeparation {
				tooClose = true
				break
			}
		}
		if !tooClose {
			filtered = append(filtered, p)
		}
	}

	return filtered
}

// MonopulsePhase correlates sum and delta FFT bins and returns the resulting phase (radians).
// This is the classic correlation-based monopulse: angle ∝ arg( Σ conj(S) * Δ ).
func MonopulsePhase(sumFFT, deltaFFT []complex128, start, end int) float64 {
	n := len(sumFFT)
	if len(deltaFFT) < n {
		n = len(deltaFFT)
	}
	if n == 0 {
		return 0
	}

	s, e := binRange(n, start, end)
	if s == e {
		return 0
	}

	var corr complex128
	for i := s; i < e; i++ {
		corr += cmplx.Conj(sumFFT[i]) * deltaFFT[i]
	}
	return cmplx.Phase(corr)
}

func fftToDBFS(fft []complex128) []float64 {
	if len(fft) == 0 {
		return nil
	}
	dbfs := make([]float64, len(fft))
	for i, v := range fft {
		mag := cmplx.Abs(v)
		if mag == 0 {
			dbfs[i] = -math.Inf(1)
			continue
		}
		dbfs[i] = 20 * math.Log10(mag/adcScale)
	}
	return dbfs
}

// MonopulsePhaseRatio implements an alternative monopulse estimator:
//
//	r_k = Δ_k / S_k
//	r̄  = weighted average of r_k over bins
//	φ   = arg(r̄)
//
// Bins with very small |S_k| are ignored. Bins are weighted by |S_k| to
// stabilise against low-SNR spectral lines.
func MonopulsePhaseRatio(sumFFT, deltaFFT []complex128, start, end int) float64 {
	n := len(sumFFT)
	if len(deltaFFT) < n {
		n = len(deltaFFT)
	}
	if n == 0 {
		return 0
	}

	s, e := binRange(n, start, end)
	if s == e {
		return 0
	}

	var acc complex128
	var wSum float64

	for i := s; i < e; i++ {
		sv := sumFFT[i]
		dv := deltaFFT[i]

		mag := cmplx.Abs(sv)
		if mag < 1e-12 {
			continue
		}
		ratio := dv / sv
		acc += ratio * complex(mag, 0)
		wSum += mag
	}

	if wSum == 0 {
		return 0
	}
	avg := acc / complex(wSum, 0)
	return cmplx.Phase(avg)
}

// --------- Small SIMD-friendly helpers (pure Go, auto-vectorisable) ---------

// complexScale multiplies src by scale into dst.
func complexScale(dst, src []complex64, scale complex64) {
	for i := 0; i < len(src); i++ {
		dst[i] = src[i] * scale
	}
}

// sumDeltaForms computes sumBuf = a + b, deltaBuf = a - b.
//
// Written as one pass over memory to increase the chance of auto-vectorisation.
func sumDeltaForms(sumBuf, deltaBuf, a, b []complex64) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if len(sumBuf) < n {
		n = len(sumBuf)
	}
	if len(deltaBuf) < n {
		n = len(deltaBuf)
	}
	for i := 0; i < n; i++ {
		ai := a[i]
		bi := b[i]
		sumBuf[i] = ai + bi
		deltaBuf[i] = ai - bi
	}
}

// --------- Coarse Scan (single-threaded) ---------

// CoarseScan iterates across candidate phase delays to find the best steering angle.
func CoarseScan(
	rx0, rx1 []complex64,
	phaseCal float64,
	startBin, endBin int,
	stepDeg float64,
	freqHz float64,
	spacingWavelength float64,
) (bestDelay float64, bestTheta float64, peakDBFS float64) {
	if stepDeg == 0 {
		stepDeg = 2
	}

	// Use only as many samples as are available on both channels.
	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 {
		return 0, 0, 0
	}

	adjusted := make([]complex64, n)
	sumBuf := make([]complex64, n)
	deltaBuf := make([]complex64, n)

	peakDBFS = -math.MaxFloat64
	bestMonoPhase := math.MaxFloat64

	for phase := -180.0; phase < 180.0; phase += stepDeg {
		phaseRad := (phase + phaseCal) * degToRad
		phaseFactor := complex64(cmplx.Exp(complex(0, phaseRad)))

		complexScale(adjusted, rx1[:n], phaseFactor)
		sumDeltaForms(sumBuf, deltaBuf, rx0[:n], adjusted)

		sumFFT, sumDBFS := FFTAndDBFS(sumBuf)
		deltaFFT, _ := FFTAndDBFS(deltaBuf)

		if len(sumDBFS) == 0 || len(sumFFT) == 0 || len(deltaFFT) == 0 {
			continue
		}

		// Choose which monopulse algorithm you like more:
		monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
		// monoPhase := MonopulsePhaseRatio(sumFFT, deltaFFT, startBin, endBin)

		peak, _, ok := peakInBand(sumDBFS, startBin, endBin)
		if !ok {
			// fall back to full-band search
			peak, _, ok = peakInBand(sumDBFS, 0, len(sumDBFS))
		}
		if !ok {
			continue
		}

		// Primary criterion: highest peak.
		// Secondary (tie-break): smallest |monopulse phase|.
		if peak > peakDBFS || (peak == peakDBFS && math.Abs(monoPhase) < math.Abs(bestMonoPhase)) {
			peakDBFS = peak
			bestDelay = phase
			bestTheta = PhaseToTheta(phase, freqHz, spacingWavelength)
			bestMonoPhase = monoPhase
		}
	}

	if peakDBFS == -math.MaxFloat64 {
		peakDBFS = 0
	}
	return bestDelay, bestTheta, peakDBFS
}

// --------- Tracking (single-threaded) ---------

// MonopulseTrack applies a monopulse correction step based on the sign/magnitude of the
// correlation phase and returns the updated delay along with the observed peak in the
// sum spectrum (dBFS).
func MonopulseTrack(
	lastDelay float64,
	rx0, rx1 []complex64,
	phaseCal float64,
	startBin, endBin int,
	phaseStep float64,
) (float64, float64) {
	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 {
		return lastDelay, 0
	}

	adjusted := make([]complex64, n)
	sumBuf := make([]complex64, n)
	deltaBuf := make([]complex64, n)

	phaseRad := (lastDelay + phaseCal) * degToRad
	phaseFactor := complex64(cmplx.Exp(complex(0, phaseRad)))

	complexScale(adjusted, rx1[:n], phaseFactor)
	sumDeltaForms(sumBuf, deltaBuf, rx0[:n], adjusted)

	sumFFT, sumDBFS := FFTAndDBFS(sumBuf)
	deltaFFT, _ := FFTAndDBFS(deltaBuf)

	if len(sumDBFS) == 0 || len(sumFFT) == 0 || len(deltaFFT) == 0 {
		return lastDelay, 0
	}

	// Same choice as above: correlation or ratio-based.
	monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
	// monoPhase := MonopulsePhaseRatio(sumFFT, deltaFFT, startBin, endBin)

	peak, _, ok := peakInBand(sumDBFS, startBin, endBin)
	if !ok {
		peak, _, ok = peakInBand(sumDBFS, 0, len(sumDBFS))
	}
	if !ok {
		peak = 0
	}

	newDelay := lastDelay
	if monoPhase > monoDeadbandRad {
		newDelay = lastDelay + phaseStep
	} else if monoPhase < -monoDeadbandRad {
		newDelay = lastDelay - phaseStep
	}
	return newDelay, peak
}

// --------- Coarse Scan (parallel with worker pool) ---------

// doPhaseScan is the per-phase workhorse used by the worker pool.
func doPhaseScan(
	phase float64,
	rx0, rx1 []complex64,
	n int,
	phaseCal float64,
	startBin, endBin int,
	dsp *CachedDSP,
	adjusted, sumBuf, deltaBuf []complex64,
) (peak float64, monoPhase float64, snr float64, peakBin int, ok bool) {
	phaseRad := (phase + phaseCal) * degToRad
	phaseFactor := complex64(cmplx.Exp(complex(0, phaseRad)))

	complexScale(adjusted, rx1[:n], phaseFactor)
	sumDeltaForms(sumBuf, deltaBuf, rx0[:n], adjusted)

	sumFFT, sumDBFS := dsp.FFTAndDBFS(sumBuf)
	deltaFFT, _ := dsp.FFTAndDBFS(deltaBuf)

	if len(sumDBFS) == 0 || len(sumFFT) == 0 || len(deltaFFT) == 0 {
		return 0, 0, 0, 0, false
	}

	// Choose correlation or ratio-based monopulse:
	monoPhase = MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
	// monoPhase = MonopulsePhaseRatio(sumFFT, deltaFFT, startBin, endBin)

	bandStart := startBin
	bandEnd := endBin
	peak, peakBin, ok = peakInBand(sumDBFS, startBin, endBin)
	if !ok {
		bandStart = 0
		bandEnd = len(sumDBFS)
		peak, peakBin, ok = peakInBand(sumDBFS, 0, len(sumDBFS))
	}
	snr = estimateSNR(sumDBFS, peak, peakBin, bandStart, bandEnd)
	return peak, monoPhase, snr, peakBin, ok
}

// CoarseScanParallel performs coarse scan with parallel FFT processing using a worker pool.
// It parallelises across phase hypotheses instead of only inside each phase, which usually
// scales better for large phase grids.
func CoarseScanParallel(
	rx0, rx1 []complex64,
	phaseCal float64,
	startBin, endBin int,
	stepDeg float64,
	freqHz float64,
	spacingWavelength float64,
	dsp *CachedDSP,
) []PeakInfo {
	if stepDeg == 0 {
		stepDeg = 2
	}

	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 {
		return nil
	}

	// Build the phase grid.
	var phases []float64
	for phase := -180.0; phase < 180.0; phase += stepDeg {
		phases = append(phases, phase)
	}
	if len(phases) == 0 {
		return nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	type scanJob struct {
		idx   int
		phase float64
	}

	jobs := make(chan scanJob)
	results := make(chan scanResult, numWorkers)

	// Start workers.
	for w := 0; w < numWorkers; w++ {
		go func() {
			adjusted := make([]complex64, n)
			sumBuf := make([]complex64, n)
			deltaBuf := make([]complex64, n)

			for job := range jobs {
				peak, monoPhase, snr, peakBin, ok := doPhaseScan(
					job.phase, rx0, rx1, n, phaseCal,
					startBin, endBin, dsp,
					adjusted, sumBuf, deltaBuf,
				)
				results <- scanResult{
					idx:       job.idx,
					phase:     job.phase,
					peak:      peak,
					monoPhase: monoPhase,
					snr:       snr,
					peakBin:   peakBin,
					ok:        ok,
				}
			}
		}()
	}

	// Feed jobs.
	go func() {
		for i, p := range phases {
			jobs <- scanJob{idx: i, phase: p}
		}
		close(jobs)
	}()

	phaseResults := make([]scanResult, len(phases))
	valid := make([]bool, len(phases))

	// Collect results.
	for range phases {
		res := <-results
		if !res.ok {
			continue
		}
		if res.idx >= 0 && res.idx < len(phaseResults) {
			phaseResults[res.idx] = res
			valid[res.idx] = true
		}
	}

	var scanValues []float64
	var scanMeta []scanResult
	bestPeak := -math.MaxFloat64
	bestMono := math.MaxFloat64
	bestIdx := -1
	bestPhase := 0.0
	for i, ok := range valid {
		if !ok {
			continue
		}
		metric := phaseResults[i].snr
		if metric == 0 {
			metric = phaseResults[i].peak
		}
		scanValues = append(scanValues, metric)
		scanMeta = append(scanMeta, phaseResults[i])

		if phaseResults[i].peak > bestPeak || (phaseResults[i].peak == bestPeak && math.Abs(phaseResults[i].monoPhase) < math.Abs(bestMono)) {
			bestPeak = phaseResults[i].peak
			bestMono = phaseResults[i].monoPhase
			bestIdx = len(scanMeta) - 1
			bestPhase = phaseResults[i].phase
		}
	}

	if len(scanValues) == 0 {
		return nil
	}

	// Use peak finding on the SNR trace across phases to identify promising candidates.
	prominence := 0.1
	minSeparation := 1
	coarsePeaks := FindMultiplePeaks(scanValues, prominence, minSeparation)

	// Fallback to the single best value if prominence filtering removed everything.
	if len(coarsePeaks) == 0 {
		bestIdx := 0
		bestVal := -math.MaxFloat64
		for i, val := range scanValues {
			if val > bestVal {
				bestVal = val
				bestIdx = i
			}
		}
		coarsePeaks = []Peak{{Bin: bestIdx}}
	}

	hasBest := false
	for _, p := range coarsePeaks {
		if p.Bin == bestIdx {
			hasBest = true
			break
		}
	}
	if bestIdx >= 0 && !hasBest {
		coarsePeaks = append(coarsePeaks, Peak{Bin: bestIdx})
	}

	peakInfos := make([]PeakInfo, 0, len(coarsePeaks))
	for _, p := range coarsePeaks {
		if p.Bin < 0 || p.Bin >= len(scanMeta) {
			continue
		}
		res := scanMeta[p.Bin]
		peakInfos = append(peakInfos, PeakInfo{
			Phase:     res.phase,
			Angle:     PhaseToTheta(res.phase, freqHz, spacingWavelength),
			Peak:      res.peak,
			SNR:       res.snr,
			Bin:       res.peakBin,
			MonoPhase: res.monoPhase,
		})
	}

	const snrTieTol = 1e-3

	sort.Slice(peakInfos, func(i, j int) bool {
		snrDiff := peakInfos[i].SNR - peakInfos[j].SNR
		if math.Abs(snrDiff) < snrTieTol {
			if peakInfos[i].Peak == peakInfos[j].Peak {
				return math.Abs(peakInfos[i].MonoPhase) < math.Abs(peakInfos[j].MonoPhase)
			}
			return peakInfos[i].Peak > peakInfos[j].Peak
		}
		return snrDiff > 0
	})

	if bestIdx >= 0 {
		for i, p := range peakInfos {
			if p.Phase == bestPhase {
				if i != 0 {
					peakInfos[0], peakInfos[i] = peakInfos[i], peakInfos[0]
				}
				break
			}
		}
	}

	return peakInfos
}

// --------- Tracking (parallel FFTs for a single step) ---------

// MonopulseTrackParallel performs tracking for one or more targets using shared
// FFT results. RX channel FFTs are computed once, then reused to form the sum
// and delta spectra for each steering hypothesis. The return slice is ordered
// to match the provided delays.
func MonopulseTrackParallel(
	delays []float64,
	rx0, rx1 []complex64,
	phaseCal float64,
	startBin, endBin int,
	phaseStep float64,
	dsp *CachedDSP,
) []TrackMeasurement {
	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 || len(delays) == 0 {
		return nil
	}

	fft0 := dsp.ShiftedFFT(rx0[:n])
	fft1 := dsp.ShiftedFFT(rx1[:n])
	if len(fft0) == 0 || len(fft1) == 0 {
		return nil
	}

	sumFFT := make([]complex128, len(fft0))
	deltaFFT := make([]complex128, len(fft0))
	results := make([]TrackMeasurement, 0, len(delays))

	for _, delay := range delays {
		phaseRad := (delay + phaseCal) * degToRad
		phaseFactor := cmplx.Exp(complex(0, phaseRad))

		for i := range fft0 {
			shifted := phaseFactor * fft1[i]
			sumFFT[i] = fft0[i] + shifted
			deltaFFT[i] = fft0[i] - shifted
		}

		sumDBFS := fftToDBFS(sumFFT)
		if len(sumDBFS) == 0 {
			results = append(results, TrackMeasurement{Delay: delay})
			continue
		}

		monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
		bandStart := startBin
		bandEnd := endBin
		peak, peakBin, ok := peakInBand(sumDBFS, startBin, endBin)
		if !ok {
			bandStart = 0
			bandEnd = len(sumDBFS)
			peak, peakBin, ok = peakInBand(sumDBFS, 0, len(sumDBFS))
		}
		if !ok {
			peak = 0
		}
		snr := estimateSNR(sumDBFS, peak, peakBin, bandStart, bandEnd)

		newDelay := delay
		if monoPhase > monoDeadbandRad {
			newDelay = delay + phaseStep
		} else if monoPhase < -monoDeadbandRad {
			newDelay = delay - phaseStep
		}

		results = append(results, TrackMeasurement{
			Delay:     newDelay,
			Peak:      peak,
			MonoPhase: monoPhase,
			SNR:       snr,
			PeakBin:   peakBin,
		})
	}

	return results
}
