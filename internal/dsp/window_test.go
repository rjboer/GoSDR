package dsp

import (
	"math"
	"testing"
)

func TestHamming(t *testing.T) {
	win := Hamming(4)
	expected := []float64{0.08, 0.77, 0.77, 0.08}
	if len(win) != len(expected) {
		t.Fatalf("unexpected length: %d", len(win))
	}
	for i := range expected {
		if math.Abs(win[i]-expected[i]) > 1e-6 {
			t.Fatalf("index %d expected %.2f got %.6f", i, expected[i], win[i])
		}
	}
}

func TestApplyWindow(t *testing.T) {
	samples := []complex64{1 + 1i, 2 + 0i}
	win := []float64{0.5, 0.25}
	out := ApplyWindow(samples, win)
	if len(out) != 2 {
		t.Fatalf("length mismatch")
	}
	if real(out[0]) != 0.5 || imag(out[0]) != 0.5 {
		t.Fatalf("unexpected first value %v", out[0])
	}
	if len(ApplyWindow(samples, []float64{1})) != 0 {
		t.Fatalf("expected empty slice when lengths differ")
	}
}
