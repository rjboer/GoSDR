//go:build !js
// +build !js

package dsp

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
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

	rng := rand.New(rand.NewSource(42))
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

	peaks := CoarseScanParallel(
		rx0, rx1,
		0, // phaseCal
		startBin, endBin,
		stepDeg,
		freqHz,
		spacingWavelength,
		dsp, // Already a pointer from NewCachedDSP
	)

	if len(peaks) == 0 {
		t.Fatalf("no peaks returned")
	}

	primary := peaks[0]
	peak := primary.Peak
	estTheta := primary.Angle
	snr := primary.SNR

	if peak == 0 {
		t.Fatalf("no peak detected")
	}
	if snr <= 0 {
		t.Fatalf("expected positive SNR estimate, got %.2f", snr)
	}

	errDeg := math.Abs(math.Abs(estTheta) - math.Abs(trueThetaDeg))
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
		_ = CoarseScanParallel(
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

func TestFindMultiplePeaksProminenceAndOrdering(t *testing.T) {
	spectrum := []float64{0, 2, 0, 5, 0, 3, 0, 4, 0}

	peaks := FindMultiplePeaks(spectrum, 0.5, 0)

	if len(peaks) != 4 {
		t.Fatalf("expected 4 peaks, got %d", len(peaks))
	}

	// Sorted by SNR/level descending.
	expectedBins := []int{3, 7, 5, 1}
	for i, p := range peaks {
		if p.Bin != expectedBins[i] {
			t.Fatalf("peak %d bin mismatch: got %d, want %d", i, p.Bin, expectedBins[i])
		}
		if p.Prominence <= 0.5 {
			t.Fatalf("expected prominence > 0.5, got %.2f", p.Prominence)
		}
	}
}

func TestFindMultiplePeaksSeparation(t *testing.T) {
	spectrum := []float64{0, 6, 0, 5, 0, 4, 0}

	peaks := FindMultiplePeaks(spectrum, 0.1, 3)
	if len(peaks) != 2 {
		t.Fatalf("expected 2 peaks after separation filtering, got %d", len(peaks))
	}

	if peaks[0].Bin != 1 || peaks[1].Bin != 5 {
		t.Fatalf("unexpected peak bins: got (%d, %d)", peaks[0].Bin, peaks[1].Bin)
	}
}

func TestFindMultiplePeaksProminenceThreshold(t *testing.T) {
	spectrum := []float64{0, 1, 0.9, 0.8, 0.7, 2, 0, 0.5}

	peaks := FindMultiplePeaks(spectrum, 1.0, 1)
	if len(peaks) != 1 {
		t.Fatalf("expected only the strong peak to pass prominence, got %d peaks", len(peaks))
	}

	if peaks[0].Bin != 5 {
		t.Fatalf("expected strong peak at bin 5, got %d", peaks[0].Bin)
	}
}

func TestFindMultiplePeaksEmpty(t *testing.T) {
	peaks := FindMultiplePeaks(nil, 0.1, 2)
	if len(peaks) != 0 {
		t.Fatalf("expected no peaks for empty input, got %d", len(peaks))
	}
}
