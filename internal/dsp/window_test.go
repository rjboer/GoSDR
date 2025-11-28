package dsp

import (
	"math"
	"testing"
)

func TestHamming(t *testing.T) {
	tests := []struct {
		name string
		n    int
		exp  []float64
	}{
		{name: "python_vector_4", n: 4, exp: []float64{0.08, 0.77, 0.77, 0.08}},
		{name: "zero_length", n: 0, exp: []float64{}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			win := Hamming(tt.n)
			if len(win) != len(tt.exp) {
				t.Fatalf("unexpected length: %d", len(win))
			}
			for i := range tt.exp {
				if math.Abs(win[i]-tt.exp[i]) > 1e-6 {
					t.Fatalf("index %d expected %.2f got %.6f", i, tt.exp[i], win[i])
				}
			}
		})
	}
}

func TestApplyWindow(t *testing.T) {
	tests := []struct {
		name    string
		samples []complex64
		win     []float64
		exp     []complex128
	}{
		{name: "python_two_point", samples: []complex64{1 + 1i, 2 + 0i}, win: []float64{0.5, 0.25}, exp: []complex128{0.5 + 0.5i, 0.5 + 0i}},
		{name: "mismatched_lengths", samples: []complex64{1 + 0i}, win: []float64{}, exp: []complex128{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := ApplyWindow(tt.samples, tt.win)
			if len(out) != len(tt.exp) {
				t.Fatalf("length mismatch got %d want %d", len(out), len(tt.exp))
			}
			for i := range out {
				if real(out[i]) != real(tt.exp[i]) || imag(out[i]) != imag(tt.exp[i]) {
					t.Fatalf("index %d got %v want %v", i, out[i], tt.exp[i])
				}
			}
		})
	}
}
