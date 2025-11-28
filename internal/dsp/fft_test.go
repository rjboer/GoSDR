package dsp

import (
	"math"
	"testing"
)

func TestFFTAndDBFS(t *testing.T) {
	tests := []struct {
		name string
		n    int
		offs float64
	}{
		{name: "single_bin_python_match", n: 8, offs: 1},
		{name: "dc_only", n: 4, offs: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]complex64, tt.n)
			for i := 0; i < tt.n; i++ {
				phase := 2 * math.Pi * float64(i) * tt.offs / float64(tt.n)
				data[i] = complex64(complex(math.Cos(phase), math.Sin(phase)))
			}
			fft, db := FFTAndDBFS(data)
			if len(fft) != tt.n || len(db) != tt.n {
				t.Fatalf("unexpected lengths")
			}

			// Manual DFT to compare against the library to mimic the python reference.
			ref := make([]complex128, tt.n)
			win := Hamming(tt.n)
			var winSum float64
			for _, w := range win {
				winSum += w
			}
			for k := 0; k < tt.n; k++ {
				var acc complex128
				for n := 0; n < tt.n; n++ {
					phi := -2 * math.Pi * float64(k*n) / float64(tt.n)
					acc += complex(float64(real(data[n]))*win[n], float64(imag(data[n]))*win[n]) * complex(math.Cos(phi), math.Sin(phi))
				}
				ref[k] = acc / complex(winSum, 0)
			}

			ref = FFTShift(ref)
			if peakIndex(fft) != peakIndex(ref) {
				t.Fatalf("peak bin mismatch between fft and reference")
			}
			for i := range fft {
				if math.IsNaN(db[i]) {
					t.Fatalf("dbfs contains NaN")
				}
			}
		})
	}
}

func peakIndex(values []complex128) int {
	maxIdx := 0
	maxMag := -1.0
	for i, v := range values {
		mag := real(v)*real(v) + imag(v)*imag(v)
		if mag > maxMag {
			maxMag = mag
			maxIdx = i
		}
	}
	return maxIdx
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
