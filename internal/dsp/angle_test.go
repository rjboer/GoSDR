package dsp

import (
	"math"
	"testing"
)

func TestPhaseThetaRoundTrip(t *testing.T) {
	tests := []struct {
		phase   float64
		freq    float64
		spacing float64
	}{
		{phase: 30, freq: 2.3e9, spacing: 0.5},
		{phase: -45, freq: 2.3e9, spacing: 0.5},
	}

	for _, tt := range tests {
		theta := PhaseToTheta(tt.phase, tt.freq, tt.spacing)
		recovered := ThetaToPhase(theta, tt.freq, tt.spacing)
		if math.Abs(recovered-tt.phase) > 1e-3 {
			t.Fatalf("round trip mismatch: %.3f vs %.3f", tt.phase, recovered)
		}
	}
}

func TestSignalBinRange(t *testing.T) {
	tests := []struct {
		n        int
		rate     float64
		offset   float64
		expected [2]int
	}{
		{n: 1024, rate: 2e6, offset: 200e3, expected: [2]int{563, 716}},
		{n: 4096, rate: 2e6, offset: 200e3, expected: [2]int{2252, 2867}},
	}

	for _, tt := range tests {
		start, end := SignalBinRange(tt.n, tt.rate, tt.offset)
		if start != tt.expected[0] || end != tt.expected[1] {
			t.Fatalf("unexpected range %d-%d", start, end)
		}
	}
}
