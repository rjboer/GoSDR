package dsp

import (
	"math"
	"testing"
)

func tonePair(num int, freqOffset float64, sampleRate float64, phaseDelta float64) ([]complex64, []complex64) {
	rx0 := make([]complex64, num)
	rx1 := make([]complex64, num)
	step := 2 * math.Pi * freqOffset / sampleRate
	shift := phaseDelta * math.Pi / 180
	for i := 0; i < num; i++ {
		phase := step * float64(i)
		val0 := complex64(complex(math.Cos(phase), math.Sin(phase)))
		val1 := complex64(complex(math.Cos(phase+shift), math.Sin(phase+shift)))
		rx0[i] = val0
		rx1[i] = val1
	}
	return rx0, rx1
}

func TestMonopulsePhase(t *testing.T) {
	sumFFT := []complex128{1, 1}
	deltaFFT := []complex128{1i, 1i}
	phase := MonopulsePhase(sumFFT, deltaFFT, 0, 2)
	if math.Abs(phase-math.Pi/2) > 1e-9 {
		t.Fatalf("expected pi/2 got %f", phase)
	}
}

func TestCoarseScanFindsDelay(t *testing.T) {
	rx0, rx1 := tonePair(512, 200e3, 2e6, 45)
	start, end := SignalBinRange(len(rx0), 2e6, 200e3)
	delay, theta, _ := CoarseScan(rx0, rx1, 0, start, end, 5, 2.3e9, 0.5)
	if math.Abs(delay+45) > 5 {
		t.Fatalf("expected delay near -45, got %.2f", delay)
	}
	if math.Abs(theta-PhaseToTheta(delay, 2.3e9, 0.5)) > 1e-6 {
		t.Fatalf("theta mismatch")
	}
}

func TestMonopulseTrackStep(t *testing.T) {
	rx0, rx1 := tonePair(256, 200e3, 2e6, 20)
	start, end := SignalBinRange(len(rx0), 2e6, 200e3)
	next := MonopulseTrack(0, rx0, rx1, 0, start, end, 1)
	if math.Abs(next) < 0.5 || math.Abs(next) > 5 {
		t.Fatalf("unexpected next delay %.2f", next)
	}
}
