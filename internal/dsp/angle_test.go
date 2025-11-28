package dsp

import (
	"math"
	"testing"
)

func TestPhaseThetaRoundTrip(t *testing.T) {
	freq := 2.3e9
	spacing := 0.5
	phase := 30.0
	theta := PhaseToTheta(phase, freq, spacing)
	recovered := ThetaToPhase(theta, freq, spacing)
	if math.Abs(recovered-phase) > 1e-3 {
		t.Fatalf("round trip mismatch: %.3f vs %.3f", phase, recovered)
	}
}

func TestSignalBinRange(t *testing.T) {
	start, end := SignalBinRange(1024, 2e6, 200e3)
	if start != 563 || end != 716 {
		t.Fatalf("unexpected range %d-%d", start, end)
	}
}
