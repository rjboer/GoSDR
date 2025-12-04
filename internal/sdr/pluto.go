package sdr

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/rjboer/GoSDR/iiod"
)

// PlutoSDR implements a minimal AD9361/Pluto backend using the IIOD client.
// It configures sample rate, LO, and gain attributes on initialization and
// provides dual-channel RX/TX streaming helpers.
type PlutoSDR struct {
	mu         sync.Mutex
	client     *iiod.Client
	phyDev     string
	rxDev      string
	txDev      string
	rxBuffer   *iiod.Buffer
	txBuffer   *iiod.Buffer
	numSamples int
	phaseDelta float64
}

func NewPluto() *PlutoSDR { return &PlutoSDR{} }

// Init connects to the IIOD server, discovers the AD9361 devices, programs
// key attributes, and prepares RX/TX buffers for dual-channel streaming.
func (p *PlutoSDR) Init(_ context.Context, cfg Config) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.URI == "" {
		cfg.URI = "192.168.2.1:30431"
	}
	if cfg.NumSamples <= 0 {
		cfg.NumSamples = 1024
	}
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}

	client, err := iiod.Dial(cfg.URI)
	if err != nil {
		return fmt.Errorf("connect to IIOD: %w", err)
	}

	devices, err := client.ListDevices()
	if err != nil {
		return fmt.Errorf("list devices: %w", err)
	}

	phy, rx, tx := identifyAD9361Devices(devices)
	if phy == "" || rx == "" || tx == "" {
		_ = client.Close()
		return fmt.Errorf("unable to locate AD9361 devices (phy=%q rx=%q tx=%q)", phy, rx, tx)
	}

	// Program sample rate and LOs.
	if err := client.WriteAttr(phy, "", "sampling_frequency", fmt.Sprintf("%0.f", cfg.SampleRate)); err != nil {
		_ = client.Close()
		return fmt.Errorf("set sample rate: %w", err)
	}
	if cfg.RxLO > 0 {
		if err := client.WriteAttr(phy, "altvoltage1", "frequency", fmt.Sprintf("%0.f", cfg.RxLO)); err != nil {
			_ = client.Close()
			return fmt.Errorf("set RX LO: %w", err)
		}
		if err := client.WriteAttr(phy, "altvoltage0", "frequency", fmt.Sprintf("%0.f", cfg.RxLO)); err != nil {
			_ = client.Close()
			return fmt.Errorf("set TX LO: %w", err)
		}
	}

	// Configure RX gains.
	if err := client.WriteAttr(phy, "voltage0", "gain_control_mode", "manual"); err != nil {
		_ = client.Close()
		return fmt.Errorf("set rx0 gain mode: %w", err)
	}
	if err := client.WriteAttr(phy, "voltage1", "gain_control_mode", "manual"); err != nil {
		_ = client.Close()
		return fmt.Errorf("set rx1 gain mode: %w", err)
	}
	if err := client.WriteAttr(phy, "voltage0", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain0)); err != nil {
		_ = client.Close()
		return fmt.Errorf("set rx0 gain: %w", err)
	}
	if err := client.WriteAttr(phy, "voltage1", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain1)); err != nil {
		_ = client.Close()
		return fmt.Errorf("set rx1 gain: %w", err)
	}
	if err := client.WriteAttr(phy, "out", "hardwaregain", fmt.Sprintf("%d", cfg.TxGain)); err != nil {
		// Some firmware exposes TX gain per-channel; fall back without failing hard.
	}

	rxBuf, err := client.CreateStreamBuffer(rx, cfg.NumSamples, 0x3)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("create RX buffer: %w", err)
	}
	txBuf, err := client.CreateStreamBuffer(tx, cfg.NumSamples, 0x3)
	if err != nil {
		_ = rxBuf.Close()
		_ = client.Close()
		return fmt.Errorf("create TX buffer: %w", err)
	}

	p.client = client
	p.phyDev = phy
	p.rxDev = rx
	p.txDev = tx
	p.rxBuffer = rxBuf
	p.txBuffer = txBuf
	p.numSamples = cfg.NumSamples
	p.phaseDelta = cfg.PhaseDelta

	return nil
}

// RX reads a buffer from the SDR and returns deinterleaved complex64 slices for
// channels 0 and 1.
func (p *PlutoSDR) RX(_ context.Context) ([]complex64, []complex64, error) {
	p.mu.Lock()
	buf := p.rxBuffer
	p.mu.Unlock()

	if buf == nil {
		return nil, nil, fmt.Errorf("RX buffer not initialized")
	}

	data, err := buf.ReadSamples()
	if err != nil {
		return nil, nil, fmt.Errorf("read RX buffer: %w", err)
	}

	samples, err := iiod.ParseInt16Samples(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse RX samples: %w", err)
	}

	i0, q0, err := iiod.DeinterleaveIQ(samples, 2, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("deinterleave chan0: %w", err)
	}
	i1, q1, err := iiod.DeinterleaveIQ(samples, 2, 1)
	if err != nil {
		return nil, nil, fmt.Errorf("deinterleave chan1: %w", err)
	}

	return iqToComplex(i0, q0), iqToComplex(i1, q1), nil
}

// TX writes interleaved complex samples for both channels to the SDR.
func (p *PlutoSDR) TX(_ context.Context, iq0, iq1 []complex64) error {
	p.mu.Lock()
	buf := p.txBuffer
	p.mu.Unlock()

	if buf == nil {
		return fmt.Errorf("TX buffer not initialized")
	}
	if len(iq0) != len(iq1) {
		return fmt.Errorf("TX channel lengths differ: %d vs %d", len(iq0), len(iq1))
	}

	i0, q0 := complexToIQ(iq0)
	i1, q1 := complexToIQ(iq1)
	interleaved, err := iiod.InterleaveIQ([][][]int16{{i0, q0}, {i1, q1}})
	if err != nil {
		return fmt.Errorf("interleave TX IQ: %w", err)
	}

	data := iiod.FormatInt16Samples(interleaved)
	if err := buf.WriteSamples(data); err != nil {
		return fmt.Errorf("write TX buffer: %w", err)
	}

	return nil
}

// Close releases buffers and the underlying IIOD connection.
func (p *PlutoSDR) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var firstErr error
	if p.rxBuffer != nil {
		if err := p.rxBuffer.Close(); err != nil {
			firstErr = err
		}
		p.rxBuffer = nil
	}
	if p.txBuffer != nil {
		if err := p.txBuffer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.txBuffer = nil
	}
	if p.client != nil {
		if err := p.client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		p.client = nil
	}
	return firstErr
}

// SetPhaseDelta records the requested phase delta without applying it to
// hardware. Storing the value allows callers to inspect the requested offset
// for telemetry even though the AD9361 does not expose a direct control.
func (p *PlutoSDR) SetPhaseDelta(phaseDeltaDeg float64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.phaseDelta = phaseDeltaDeg
}

// GetPhaseDelta returns the last requested phase delta. Hardware backends do
// not apply this value, but tracking it keeps the interface consistent with
// simulators.
func (p *PlutoSDR) GetPhaseDelta() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.phaseDelta
}

// identifyAD9361Devices finds the PHY, RX, and TX device identifiers.
func identifyAD9361Devices(devices []string) (phy, rx, tx string) {
	for _, dev := range devices {
		lower := strings.ToLower(dev)
		switch {
		case strings.Contains(lower, "ad9361-phy"):
			phy = dev
		case strings.Contains(lower, "cf-ad9361-dds"):
			tx = dev
		case strings.Contains(lower, "cf-ad9361-lpc"):
			rx = dev
		}
	}
	return phy, rx, tx
}

func iqToComplex(iSamples, qSamples []int16) []complex64 {
	n := len(iSamples)
	out := make([]complex64, n)
	scale := float32(1.0 / 32768.0)
	for i := 0; i < n; i++ {
		out[i] = complex(float32(iSamples[i])*scale, float32(qSamples[i])*scale)
	}
	return out
}

func complexToIQ(samples []complex64) ([]int16, []int16) {
	iSamples := make([]int16, len(samples))
	qSamples := make([]int16, len(samples))
	for i, v := range samples {
		iSamples[i] = floatToInt16(real(v))
		qSamples[i] = floatToInt16(imag(v))
	}
	return iSamples, qSamples
}

func floatToInt16(v float32) int16 {
	scaled := int(math.Round(float64(v * 32767)))
	if scaled > math.MaxInt16 {
		return math.MaxInt16
	}
	if scaled < math.MinInt16 {
		return math.MinInt16
	}
	return int16(scaled)
}
