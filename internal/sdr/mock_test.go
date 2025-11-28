package sdr

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/rjboer/GoSDR/internal/dsp"
)

func TestMockSDRGeneratesPhaseDelta(t *testing.T) {
	rand.Seed(1)
	mock := NewMock()
	cfg := Config{SampleRate: 2e6, ToneOffset: 200e3, NumSamples: 512, PhaseDelta: 45}
	if err := mock.Init(context.Background(), cfg); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	ch0, ch1, err := mock.RX(context.Background())
	if err != nil {
		t.Fatalf("rx failed: %v", err)
	}
	if len(ch0) != cfg.NumSamples || len(ch1) != cfg.NumSamples {
		t.Fatalf("unexpected sample count")
	}

	start, end := dsp.SignalBinRange(len(ch0), cfg.SampleRate, cfg.ToneOffset)
	delay, theta, _ := dsp.CoarseScan(ch0, ch1, 0, start, end, 2, 2.3e9, 0.5)
	if math.Abs(delay+cfg.PhaseDelta) > 5 {
		t.Fatalf("expected delay near -%d got %.2f", int(cfg.PhaseDelta), delay)
	}
	expectedTheta := dsp.PhaseToTheta(delay, 2.3e9, 0.5)
	if math.Abs(theta-expectedTheta) > 1e-6 {
		t.Fatalf("theta mismatch")
	}
}

func TestMockDefaulting(t *testing.T) {
	mock := NewMock()
	rand.Seed(2)
	if err := mock.Init(context.Background(), Config{}); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	ch0, ch1, err := mock.RX(context.Background())
	if err != nil {
		t.Fatalf("rx failed: %v", err)
	}
	if len(ch0) == 0 || len(ch1) == 0 {
		t.Fatalf("expected default buffer")
	}
}
