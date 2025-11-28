package sdr

import (
	"context"
)

// Config carries parameters required to initialize an SDR backend.
type Config struct {
	SampleRate float64
	RxLO       float64
	ToneOffset float64
	NumSamples int
	PhaseDelta float64 // phase offset between channels in degrees
	URI        string
}

// SDR captures the minimal radio operations required by the tracker.
type SDR interface {
	Init(ctx context.Context, cfg Config) error
	RX(ctx context.Context) (chan0 []complex64, chan1 []complex64, err error)
	TX(ctx context.Context, iq0, iq1 []complex64) error
	Close() error
}
