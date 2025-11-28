package sdr

import (
	"context"
	"math"
	"math/rand"
)

// MockSDR synthesizes two-channel IQ data with a controllable phase offset.
type MockSDR struct {
	cfg Config
}

func NewMock() *MockSDR { return &MockSDR{} }

func (m *MockSDR) Init(_ context.Context, cfg Config) error {
	m.cfg = cfg
	return nil
}

func (m *MockSDR) Close() error { return nil }

func (m *MockSDR) TX(_ context.Context, _, _ []complex64) error { return nil }

func (m *MockSDR) RX(_ context.Context) ([]complex64, []complex64, error) {
	if m.cfg.NumSamples == 0 {
		m.cfg.NumSamples = 1024
	}
	if m.cfg.SampleRate == 0 {
		m.cfg.SampleRate = 2e6
	}
	n := m.cfg.NumSamples
	ch0 := make([]complex64, n)
	ch1 := make([]complex64, n)
	phaseStep := 2 * math.Pi * m.cfg.ToneOffset / m.cfg.SampleRate
	phaseDelta := m.cfg.PhaseDelta * math.Pi / 180
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
