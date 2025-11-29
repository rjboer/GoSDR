package dsp

import (
	"math"
	"math/cmplx"
	"runtime"
	"sync"
)

const (
	degToRad        = math.Pi / 180.0
	monoDeadbandRad = 0.5 * math.Pi / 180.0 // ~0.5° deadband for tracking
)

// scanResult is used by the worker-pool coarse scan.
type scanResult struct {
	phase     float64
	peak      float64
	monoPhase float64
	snr       float64
	peakBin   int
	ok        bool
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
) (bestDelay float64, bestTheta float64, peakDBFS float64, monoPhase float64, peakBin int, snr float64) {
	if stepDeg == 0 {
		stepDeg = 2
	}

	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 {
		return 0, 0, 0, 0, 0, 0
	}

	// Build the phase grid.
	var phases []float64
	for phase := -180.0; phase < 180.0; phase += stepDeg {
		phases = append(phases, phase)
	}
	if len(phases) == 0 {
		return 0, 0, 0, 0, 0, 0
	}

	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}

	jobs := make(chan float64)
	results := make(chan scanResult, numWorkers)

	// Start workers.
	for w := 0; w < numWorkers; w++ {
		go func() {
			adjusted := make([]complex64, n)
			sumBuf := make([]complex64, n)
			deltaBuf := make([]complex64, n)

			for phase := range jobs {
				peak, monoPhase, snr, peakBin, ok := doPhaseScan(
					phase, rx0, rx1, n, phaseCal,
					startBin, endBin, dsp,
					adjusted, sumBuf, deltaBuf,
				)
				results <- scanResult{
					phase:     phase,
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
		for _, p := range phases {
			jobs <- p
		}
		close(jobs)
	}()

	peakDBFS = -math.MaxFloat64
	bestMonoPhase := math.MaxFloat64
	bestPeakBin := 0
	bestSNR := 0.0

	// Collect results.
	for range phases {
		res := <-results
		if !res.ok {
			continue
		}
		if res.peak > peakDBFS || (res.peak == peakDBFS && math.Abs(res.monoPhase) < math.Abs(bestMonoPhase)) {
			peakDBFS = res.peak
			bestDelay = res.phase
			bestTheta = PhaseToTheta(res.phase, freqHz, spacingWavelength)
			bestMonoPhase = res.monoPhase
			bestPeakBin = res.peakBin
			bestSNR = res.snr
		}
	}

	if peakDBFS == -math.MaxFloat64 {
		peakDBFS = 0
	}
	return bestDelay, bestTheta, peakDBFS, bestMonoPhase, bestPeakBin, bestSNR
}

// --------- Tracking (parallel FFTs for a single step) ---------

// MonopulseTrackParallel performs tracking with parallel FFT computation.
// This version computes sum and delta FFTs concurrently using goroutines.
func MonopulseTrackParallel(
	lastDelay float64,
	rx0, rx1 []complex64,
	phaseCal float64,
	startBin, endBin int,
	phaseStep float64,
	dsp *CachedDSP,
) (float64, float64, float64, float64, int) {
	n := len(rx0)
	if len(rx1) < n {
		n = len(rx1)
	}
	if n == 0 {
		return lastDelay, 0, 0, 0, 0
	}

	adjusted := make([]complex64, n)
	sumBuf := make([]complex64, n)
	deltaBuf := make([]complex64, n)

	phaseRad := (lastDelay + phaseCal) * degToRad
	phaseFactor := complex64(cmplx.Exp(complex(0, phaseRad)))

	complexScale(adjusted, rx1[:n], phaseFactor)
	sumDeltaForms(sumBuf, deltaBuf, rx0[:n], adjusted)

	// Parallel FFT computation
	var sumFFT, deltaFFT []complex128
	var sumDBFS []float64
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		sumFFT, sumDBFS = dsp.FFTAndDBFS(sumBuf)
	}()

	go func() {
		defer wg.Done()
		deltaFFT, _ = dsp.FFTAndDBFS(deltaBuf)
	}()

	wg.Wait()

	if len(sumDBFS) == 0 || len(sumFFT) == 0 || len(deltaFFT) == 0 {
		return lastDelay, 0, 0, 0, 0
	}

	monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
	// monoPhase := MonopulsePhaseRatio(sumFFT, deltaFFT, startBin, endBin)

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

	newDelay := lastDelay
	if monoPhase > monoDeadbandRad {
		newDelay = lastDelay + phaseStep
	} else if monoPhase < -monoDeadbandRad {
		newDelay = lastDelay - phaseStep
	}
	return newDelay, peak, monoPhase, snr, peakBin
}
