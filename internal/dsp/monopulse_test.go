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
	tests := []struct {
		name       string
		sumFFT     []complex128
		deltaFFT   []complex128
		start, end int
		expected   float64
	}{
		{name: "quadrature", sumFFT: []complex128{1, 1}, deltaFFT: []complex128{1i, 1i}, start: 0, end: 2, expected: math.Pi / 2},
		{name: "neg_quadrature", sumFFT: []complex128{1i, 1i}, deltaFFT: []complex128{1, 1}, start: 0, end: 2, expected: -math.Pi / 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			phase := MonopulsePhase(tt.sumFFT, tt.deltaFFT, tt.start, tt.end)
			if math.Abs(phase-tt.expected) > 1e-9 {
				t.Fatalf("expected %f got %f", tt.expected, phase)
			}
		})
	}
}

func TestCoarseScanFindsDelay(t *testing.T) {
	rx0, rx1 := tonePair(512, 200e3, 2e6, 30)
	start, end := SignalBinRange(len(rx0), 2e6, 200e3)
	delay, theta, peak := CoarseScan(rx0, rx1, 0, start, end, 2, 2.3e9, 0.5)
	if math.Abs(delay+30) > 2 {
		t.Fatalf("expected delay near -30, got %.2f", delay)
	}
	if math.Abs(theta-PhaseToTheta(delay, 2.3e9, 0.5)) > 1e-6 {
		t.Fatalf("theta mismatch")
	}
	if peak <= -math.MaxFloat64/4 {
		t.Fatalf("peak was not updated")
	}
}

func TestMonopulseTrackStep(t *testing.T) {
	rx0, rx1 := tonePair(256, 200e3, 2e6, 20)
	start, end := SignalBinRange(len(rx0), 2e6, 200e3)
	next, peak := MonopulseTrack(-10, rx0, rx1, 0, start, end, 1)
	if next >= -10 {
		t.Fatalf("expected negative step toward phase delta got %.2f", next)
	}
	if peak == 0 {
		t.Fatalf("expected non-zero peak telemetry")
	}
}
