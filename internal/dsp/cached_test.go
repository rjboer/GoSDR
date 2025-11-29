package dsp

import (
	"math/cmplx"
	"testing"
)

func TestCachedDSP_Correctness(t *testing.T) {
	size := 512
	cached := NewCachedDSP(size)

	// Generate test signal (simple ramp)
	samples := make([]complex64, size)
	for i := range samples {
		samples[i] = complex(float32(i)/float32(size), 0)
	}

	// Compare cached vs non-cached
	fft1, dbfs1 := cached.FFTAndDBFS(samples)
	fft2, dbfs2 := FFTAndDBFS(samples)

	// Results should be identical (within floating point precision)
	if len(fft1) != len(fft2) {
		t.Fatalf("FFT length mismatch: %d vs %d", len(fft1), len(fft2))
	}

	for i := range fft1 {
		diff := cmplx.Abs(fft1[i] - fft2[i])
		if diff > 1e-10 {
			t.Errorf("FFT mismatch at index %d: diff=%g", i, diff)
		}
	}

	if len(dbfs1) != len(dbfs2) {
		t.Fatalf("dBFS length mismatch: %d vs %d", len(dbfs1), len(dbfs2))
	}

	for i := range dbfs1 {
		diff := dbfs1[i] - dbfs2[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > 1e-6 {
			t.Errorf("dBFS mismatch at index %d: diff=%g", i, diff)
		}
	}
}

func TestCachedDSP_UpdateSize(t *testing.T) {
	cached := NewCachedDSP(256)

	if cached.Size() != 256 {
		t.Errorf("Initial size mismatch: got %d, want 256", cached.Size())
	}

	// Update to new size
	cached.UpdateSize(512)

	if cached.Size() != 512 {
		t.Errorf("Updated size mismatch: got %d, want 512", cached.Size())
	}

	// Verify it works with new size
	samples := make([]complex64, 512)
	fft, dbfs := cached.FFTAndDBFS(samples)

	if len(fft) != 512 {
		t.Errorf("FFT size after update: got %d, want 512", len(fft))
	}
	if len(dbfs) != 512 {
		t.Errorf("dBFS size after update: got %d, want 512", len(dbfs))
	}
}

func TestCachedDSP_WrongSize(t *testing.T) {
	cached := NewCachedDSP(512)

	// Try with wrong size - should fallback to non-cached
	samples := make([]complex64, 256)
	fft, dbfs := cached.FFTAndDBFS(samples)

	// Should still work, just using fallback
	if len(fft) != 256 {
		t.Errorf("Fallback FFT size: got %d, want 256", len(fft))
	}
	if len(dbfs) != 256 {
		t.Errorf("Fallback dBFS size: got %d, want 256", len(dbfs))
	}
}

func TestCachedDSP_EmptyInput(t *testing.T) {
	cached := NewCachedDSP(512)

	fft, dbfs := cached.FFTAndDBFS([]complex64{})

	if len(fft) != 0 {
		t.Errorf("Empty input FFT: got %d, want 0", len(fft))
	}
	if len(dbfs) != 0 {
		t.Errorf("Empty input dBFS: got %d, want 0", len(dbfs))
	}
}

// Benchmark cached DSP
func BenchmarkCachedDSP(b *testing.B) {
	size := 4096
	cached := NewCachedDSP(size)
	samples := make([]complex64, size)

	// Fill with some data
	for i := range samples {
		samples[i] = complex(float32(i), float32(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cached.FFTAndDBFS(samples)
	}
}

// Benchmark non-cached DSP for comparison
func BenchmarkNonCachedDSP(b *testing.B) {
	size := 4096
	samples := make([]complex64, size)

	// Fill with some data
	for i := range samples {
		samples[i] = complex(float32(i), float32(i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FFTAndDBFS(samples)
	}
}

// Benchmark parallel access to cached DSP
func BenchmarkCachedDSP_Parallel(b *testing.B) {
	size := 4096
	cached := NewCachedDSP(size)
	samples := make([]complex64, size)

	for i := range samples {
		samples[i] = complex(float32(i), float32(i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cached.FFTAndDBFS(samples)
		}
	})
}
