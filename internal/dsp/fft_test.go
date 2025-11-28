package dsp

import (
	"math"
	"testing"
)

func TestFFTAndDBFS(t *testing.T) {
	n := 8
	data := make([]complex64, n)
	for i := 0; i < n; i++ {
		phase := 2 * math.Pi * float64(i) / float64(n)
		data[i] = complex64(complex(math.Cos(phase), math.Sin(phase)))
	}
	fft, db := FFTAndDBFS(data)
	if len(fft) != n || len(db) != n {
		t.Fatalf("unexpected lengths")
	}
	maxIdx := 0
	maxMag := math.Inf(-1)
	for i, v := range fft {
		mag := real(v)*real(v) + imag(v)*imag(v)
		if mag > maxMag {
			maxMag = mag
			maxIdx = i
		}
	}
	expectedIdx := n/2 + 1
	if maxIdx != expectedIdx {
		t.Fatalf("expected peak at %d got %d", expectedIdx, maxIdx)
	}
	for _, v := range db {
		if math.IsNaN(v) {
			t.Fatalf("dbfs contains NaN")
		}
	}
}

func TestFFTShift(t *testing.T) {
	in := []complex128{0, 1, 2, 3}
	out := FFTShift(in)
	expected := []complex128{2, 3, 0, 1}
	for i := range expected {
		if out[i] != expected[i] {
			t.Fatalf("index %d expected %v got %v", i, expected[i], out[i])
		}
	}
}
