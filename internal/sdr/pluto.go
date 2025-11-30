package sdr

import (
	"context"
	"fmt"
)

// PlutoSDR is a placeholder for a real AD9361/Pluto implementation.
type PlutoSDR struct{}

func NewPluto() *PlutoSDR { return &PlutoSDR{} }

func (p *PlutoSDR) Init(_ context.Context, _ Config) error { return ErrNotImplemented }
func (p *PlutoSDR) RX(_ context.Context) ([]complex64, []complex64, error) {
	return nil, nil, ErrNotImplemented
}
func (p *PlutoSDR) TX(_ context.Context, _, _ []complex64) error { return ErrNotImplemented }
func (p *PlutoSDR) Close() error                                 { return nil }

// SetPhaseDelta is a no-op for hardware backends.
func (p *PlutoSDR) SetPhaseDelta(phaseDeltaDeg float64) {}

// GetPhaseDelta returns 0 for hardware backends.
func (p *PlutoSDR) GetPhaseDelta() float64 { return 0 }

// ErrNotImplemented signals missing hardware support.
var ErrNotImplemented = fmt.Errorf("pluto backend not implemented")
