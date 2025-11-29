//go:build !js
// +build !js

package dsp

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
	"time"
)

// simulateTwoElementArray generates a simple narrowband plane-wave signal
// for a two-element array with spacing expressed in wavelengths. The phase
// difference is 2π * spacing * sin(theta).
func simulateTwoElementArray(
	thetaDeg float64,
	n int,
	snrDB float64,
	spacingWavelength float64,
) ([]complex64, []complex64) {
	thetaRad := thetaDeg * degToRad
	phaseDiff := 2 * math.Pi * spacingWavelength * math.Sin(thetaRad)

	rx0 := make([]complex64, n)
	rx1 := make([]complex64, n)

	sigma := math.Pow(10.0, -snrDB/20.0) // linear noise std dev ~ 1/SNR

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < n; i++ {
		// Baseband constant-amplitude signal (can also be a tone if you want).
		s := complex(1.0, 0.0)
		s1 := s * cmplx.Exp(complex(0, phaseDiff))

		noise0 := complex(
			rng.NormFloat64()*sigma,
			rng.NormFloat64()*sigma,
		)
		noise1 := complex(
			rng.NormFloat64()*sigma,
			rng.NormFloat64()*sigma,
		)

		rx0[i] = complex64(s + noise0)
		rx1[i] = complex64(s1 + noise1)
	}
	return rx0, rx1
}

func TestCoarseScanParallel_SingleTarget(t *testing.T) {
	const (
		nSamples          = 1024
		trueThetaDeg      = 20.0
		spacingWavelength = 0.5
		snrDB             = 20.0
		stepDeg           = 1.0
	)

	rx0, rx1 := simulateTwoElementArray(trueThetaDeg, nSamples, snrDB, spacingWavelength)

	// Dummy values – adapt to your FFT size / bin mapping.
	const (
		startBin = 0
		endBin   = 0   // 0 means "auto/full" in our helper, we fall back to full-band
		freqHz   = 1.0 // baseband
	)

	dsp := NewCachedDSP(nSamples) // Properly initialize with FFT size

	_, estTheta, peak := CoarseScanParallel(
		rx0, rx1,
		0, // phaseCal
		startBin, endBin,
		stepDeg,
		freqHz,
		spacingWavelength,
		dsp, // Already a pointer from NewCachedDSP
	)

	if peak == 0 {
		t.Fatalf("no peak detected")
	}

	errDeg := math.Abs(estTheta - trueThetaDeg)
	if errDeg > 3.0 {
		t.Fatalf("angle error too large: got %.2f°, want %.2f° (err=%.2f°)", estTheta, trueThetaDeg, errDeg)
	}
}

func BenchmarkCoarseScanParallel(b *testing.B) {
	const (
		nSamples          = 4096
		thetaDeg          = 10.0
		spacingWavelength = 0.5
		snrDB             = 20.0
		stepDeg           = 1.0
		startBin          = 0
		endBin            = 0
		freqHz            = 1.0
	)

	rx0, rx1 := simulateTwoElementArray(thetaDeg, nSamples, snrDB, spacingWavelength)
	dsp := NewCachedDSP(nSamples)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = CoarseScanParallel(
			rx0, rx1,
			0, // phaseCal
			startBin, endBin,
			stepDeg,
			freqHz,
			spacingWavelength,
			dsp, // Already a pointer from NewCachedDSP
		)
	}
}
