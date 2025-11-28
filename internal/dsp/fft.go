package dsp

import (
	"math"
	"math/cmplx"

	"gonum.org/v1/gonum/dsp/fourier"
)

const adcScale = 2048.0 // 2^11 for 12-bit signed ADC

// FFTShift returns the FFT output shifted so that DC is centered.
func FFTShift(data []complex128) []complex128 {
	n := len(data)
	if n == 0 {
		return []complex128{}
	}
	half := n / 2
	shifted := append(data[half:], data[:half]...)
	return shifted
}

// FFTAndDBFS performs an FFT on the provided complex64 samples, applies a Hamming window,
// normalizes by the window sum, and converts the magnitude to dBFS.
func FFTAndDBFS(samples []complex64) ([]complex128, []float64) {
	if len(samples) == 0 {
		return []complex128{}, []float64{}
	}
	win := Hamming(len(samples))
	windowed := ApplyWindow(samples, win)
	fft := fourier.NewCmplxFFT(len(samples)).Coefficients(nil, windowed)
	sumWin := 0.0
	for _, v := range win {
		sumWin += v
	}
	for i := range fft {
		fft[i] /= complex(sumWin, 0)
	}
	shifted := FFTShift(fft)
	dbfs := make([]float64, len(shifted))
	for i, v := range shifted {
		mag := cmplx.Abs(v)
		if mag == 0 {
			dbfs[i] = -math.Inf(1)
			continue
		}
		dbfs[i] = 20 * math.Log10(mag/adcScale)
	}
	return shifted, dbfs
}
