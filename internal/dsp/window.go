package dsp

import "math"

// Hamming returns a Hamming window of length n.
// If n is zero or negative, an empty slice is returned.
func Hamming(n int) []float64 {
	if n <= 0 {
		return []float64{}
	}
	win := make([]float64, n)
	for i := 0; i < n; i++ {
		win[i] = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	}
	return win
}

// ApplyWindow multiplies the input complex samples with the provided window.
// The window length must match the input length.
func ApplyWindow(samples []complex64, window []float64) []complex128 {
	if len(samples) != len(window) {
		return []complex128{}
	}
	out := make([]complex128, len(samples))
	for i, v := range samples {
		out[i] = complex(float64(real(v))*window[i], float64(imag(v))*window[i])
	}
	return out
}
