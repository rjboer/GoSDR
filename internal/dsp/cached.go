package dsp

import (
	"math"
	"math/cmplx"
	"sync"

	"gonum.org/v1/gonum/dsp/fourier"
)

// CachedDSP pre-computes and caches expensive DSP resources to improve performance.
// It stores a Hamming window and FFT instance that can be reused across multiple calls,
// avoiding the overhead of recreating these resources on every operation.
type CachedDSP struct {
	mu            sync.RWMutex
	hammingWindow []float64
	windowSum     float64 // Pre-computed sum for normalization
	fftSize       int
	fft           *fourier.CmplxFFT
}

// NewCachedDSP creates a DSP processor with pre-computed cached resources.
// The Hamming window and FFT instance are created once and reused for all operations.
func NewCachedDSP(size int) *CachedDSP {
	window := Hamming(size)

	// Pre-compute window sum for normalization
	sum := 0.0
	for _, v := range window {
		sum += v
	}

	return &CachedDSP{
		hammingWindow: window,
		windowSum:     sum,
		fftSize:       size,
		fft:           fourier.NewCmplxFFT(size),
	}
}

// FFTAndDBFS performs FFT using cached window and FFT instance.
// This is significantly faster than the non-cached version as it avoids:
// - Recreating the Hamming window on every call
// - Creating a new FFT instance on every call
// - Recalculating the window sum for normalization
func (c *CachedDSP) FFTAndDBFS(samples []complex64) ([]complex128, []float64) {
	if len(samples) == 0 {
		return []complex128{}, []float64{}
	}

	// Verify size matches cached resources
	if len(samples) != c.fftSize {
		// Fallback to non-cached version for mismatched sizes
		return FFTAndDBFS(samples)
	}

	// Apply cached Hamming window
	windowed := ApplyWindow(samples, c.hammingWindow)

	// Reuse FFT instance (thread-safe with mutex)
	c.mu.Lock()
	fft := c.fft.Coefficients(nil, windowed)
	c.mu.Unlock()

	// Normalize by pre-computed window sum
	for i := range fft {
		fft[i] /= complex(c.windowSum, 0)
	}

	// Shift and convert to dBFS
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

// UpdateSize recreates cached resources for a new FFT size.
// This should be called if the sample size changes during runtime.
func (c *CachedDSP) UpdateSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fftSize = size
	c.hammingWindow = Hamming(size)

	// Recompute window sum
	sum := 0.0
	for _, v := range c.hammingWindow {
		sum += v
	}
	c.windowSum = sum

	c.fft = fourier.NewCmplxFFT(size)
}

// Size returns the current FFT size for this cached DSP instance.
func (c *CachedDSP) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.fftSize
}
