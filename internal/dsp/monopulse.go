package dsp

import (
	"math"
	"math/cmplx"
)

// MonopulsePhase correlates sum and delta FFT bins and returns the resulting phase.
func MonopulsePhase(sumFFT, deltaFFT []complex128, start, end int) float64 {
	if start < 0 {
		start = 0
	}
	if end > len(sumFFT) {
		end = len(sumFFT)
	}
	if end > len(deltaFFT) {
		end = len(deltaFFT)
	}
	if start >= end || start >= len(sumFFT) || start >= len(deltaFFT) {
		return 0
	}
	var corr complex128
	for i := start; i < end; i++ {
		corr += cmplx.Conj(sumFFT[i]) * deltaFFT[i]
	}
	return cmplx.Phase(corr)
}

// CoarseScan iterates across candidate phase delays to find the best steering angle.
func CoarseScan(rx0, rx1 []complex64, phaseCal float64, startBin, endBin int, stepDeg float64, freqHz float64, spacingWavelength float64) (bestDelay float64, bestTheta float64, peakDBFS float64) {
	if stepDeg == 0 {
		stepDeg = 2
	}
	peakDBFS = -math.MaxFloat64
	for phase := -180.0; phase < 180.0; phase += stepDeg {
		phaseRad := (phase + phaseCal) * math.Pi / 180
		adjusted := make([]complex64, len(rx1))
		for i, v := range rx1 {
			adj := complex64(cmplx.Exp(complex(0, phaseRad)))
			adjusted[i] = v * adj
		}
		sumBuf := make([]complex64, len(rx0))
		deltaBuf := make([]complex64, len(rx0))
		for i := range rx0 {
			sumBuf[i] = rx0[i] + adjusted[i]
			deltaBuf[i] = rx0[i] - adjusted[i]
		}
		sumFFT, sumDBFS := FFTAndDBFS(sumBuf)
		deltaFFT, _ := FFTAndDBFS(deltaBuf)
		monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
		if len(sumDBFS) == 0 {
			continue
		}
		peak := sumDBFS[0]
		for _, v := range sumDBFS {
			if v > peak {
				peak = v
			}
		}
		if peak > peakDBFS {
			peakDBFS = peak
			bestDelay = phase
			bestTheta = PhaseToTheta(phase, freqHz, spacingWavelength)
		}
		_ = monoPhase // retained for future tie-breaking; ensures parity with Python flow
	}
	return bestDelay, bestTheta, peakDBFS
}

// MonopulseTrack applies a monopulse correction step based on the sign of the correlation phase.
func MonopulseTrack(lastDelay float64, rx0, rx1 []complex64, phaseCal float64, startBin, endBin int, phaseStep float64) float64 {
	phaseRad := (lastDelay + phaseCal) * math.Pi / 180
	adjusted := make([]complex64, len(rx1))
	for i, v := range rx1 {
		adj := complex64(cmplx.Exp(complex(0, phaseRad)))
		adjusted[i] = v * adj
	}
	sumBuf := make([]complex64, len(rx0))
	deltaBuf := make([]complex64, len(rx0))
	for i := range rx0 {
		sumBuf[i] = rx0[i] + adjusted[i]
		deltaBuf[i] = rx0[i] - adjusted[i]
	}
	sumFFT, _ := FFTAndDBFS(sumBuf)
	deltaFFT, _ := FFTAndDBFS(deltaBuf)
	monoPhase := MonopulsePhase(sumFFT, deltaFFT, startBin, endBin)
	if math.Signbit(monoPhase) {
		return lastDelay + phaseStep
	}
	return lastDelay - phaseStep
}
