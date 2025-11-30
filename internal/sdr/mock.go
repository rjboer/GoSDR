package sdr

import (
	"context"
	"math"
	"math/rand"
	"sync"
)

// MockSDR synthesizes two-channel IQ data with a controllable phase offset.
type MockSDR struct {
	mu  sync.RWMutex
	cfg Config
}

func NewMock() *MockSDR { return &MockSDR{} }

func (m *MockSDR) Init(_ context.Context, cfg Config) error {
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return nil
}

func (m *MockSDR) Close() error { return nil }

func (m *MockSDR) TX(_ context.Context, _, _ []complex64) error { return nil }

// SetPhaseDelta updates the simulated phase delta in degrees, allowing
// real-time angle changes during operation.
func (m *MockSDR) SetPhaseDelta(phaseDeltaDeg float64) {
	m.mu.Lock()
	m.cfg.PhaseDelta = phaseDeltaDeg
	m.mu.Unlock()
}

// GetPhaseDelta returns the current phase delta setting.
func (m *MockSDR) GetPhaseDelta() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg.PhaseDelta
}

func (m *MockSDR) RX(_ context.Context) ([]complex64, []complex64, error) {
	m.mu.RLock()
	cfg := m.cfg
	m.mu.RUnlock()

	if cfg.NumSamples == 0 {
		cfg.NumSamples = 1024
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 2e6
	}
	n := cfg.NumSamples
	ch0 := make([]complex64, n)
	ch1 := make([]complex64, n)
	phaseStep := 2 * math.Pi * cfg.ToneOffset / cfg.SampleRate
	phaseDelta := cfg.PhaseDelta * math.Pi / 180
	for i := 0; i < n; i++ {
		phase := phaseStep * float64(i)
		val := complex64(complex(math.Cos(phase), math.Sin(phase)))
		noiseI := rand.NormFloat64() * 1e-4
		noiseQ := rand.NormFloat64() * 1e-4
		ch0[i] = val + complex64(complex(noiseI, noiseQ))
		shifted := phase + phaseDelta
		ch1[i] = complex64(complex(math.Cos(shifted), math.Sin(shifted))) + complex64(complex(noiseI, noiseQ))
	}
	return ch0, ch1, nil
}
