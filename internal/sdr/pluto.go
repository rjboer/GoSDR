package sdr

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rjboer/GoSDR/iiod"
)

// EventLogger defines the interface for logging events to the telemetry system.
type EventLogger interface {
	LogEvent(level, message string)
}

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

	// Debug and monitoring
	eventLogger EventLogger
	rxUnderruns uint64
	txOverruns  uint64
	debugMode   bool
}

func NewPluto() *PlutoSDR { return &PlutoSDR{} }

// SetEventLogger configures the event logger for debug messages.
func (p *PlutoSDR) SetEventLogger(logger EventLogger) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.eventLogger = logger
}

// SetDebugMode enables or disables debug logging.
func (p *PlutoSDR) SetDebugMode(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.debugMode = enabled
}

func (p *PlutoSDR) logEvent(level, message string) {
	p.mu.Lock()
	logger := p.eventLogger
	debugMode := p.debugMode
	p.mu.Unlock()

	if logger != nil && debugMode {
		logger.LogEvent(level, message)
	}
}

// DebugInfo contains IIO hardware debug information.
type DebugInfo struct {
	RSSI0       string
	RSSI1       string
	Temperature string
	RxUnderruns uint64
	TxOverruns  uint64
	SampleRate  string
	RxLO        string
	TxLO        string
}

// GetDebugInfo retrieves hardware debug information from the Pluto SDR.
// Only works when debug mode is enabled.
func (p *PlutoSDR) GetDebugInfo() (*DebugInfo, error) {
	p.mu.Lock()
	client := p.client
	phyDev := p.phyDev
	debugMode := p.debugMode
	p.mu.Unlock()

	if !debugMode {
		return nil, fmt.Errorf("debug mode disabled")
	}

	if client == nil {
		return nil, fmt.Errorf("not connected")
	}

	info := &DebugInfo{
		RxUnderruns: atomic.LoadUint64(&p.rxUnderruns),
		TxOverruns:  atomic.LoadUint64(&p.txOverruns),
	}

	// Read RSSI (signal strength)
	if rssi0, err := client.ReadAttr(phyDev, "voltage0", "rssi"); err == nil {
		info.RSSI0 = rssi0
		p.logEvent("debug", fmt.Sprintf("IIO: RSSI Ch0 = %s dB", rssi0))
	}

	if rssi1, err := client.ReadAttr(phyDev, "voltage1", "rssi"); err == nil {
		info.RSSI1 = rssi1
		p.logEvent("debug", fmt.Sprintf("IIO: RSSI Ch1 = %s dB", rssi1))
	}

	// Read temperature
	if temp, err := client.ReadAttr(phyDev, "", "in_temp0_input"); err == nil {
		info.Temperature = temp
		p.logEvent("debug", fmt.Sprintf("IIO: Temperature = %s mC", temp))
	}

	// Read current sample rate
	if sr, err := client.ReadAttr(phyDev, "", "sampling_frequency"); err == nil {
		info.SampleRate = sr
	}

	// Read LO frequencies
	if rxLO, err := client.ReadAttr(phyDev, "altvoltage1", "frequency"); err == nil {
		info.RxLO = rxLO
	}

	if txLO, err := client.ReadAttr(phyDev, "altvoltage0", "frequency"); err == nil {
		info.TxLO = txLO
	}

	// Log buffer health
	if info.RxUnderruns > 0 {
		p.logEvent("warn", fmt.Sprintf("IIO: RX buffer underruns detected: %d", info.RxUnderruns))
	}

	return info, nil
}

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

	p.logEvent("info", fmt.Sprintf("IIO: Connecting to %s", cfg.URI))

	client, err := iiod.Dial(cfg.URI)
	if err != nil {
		p.logEvent("error", fmt.Sprintf("IIO: Connection failed: %v", err))
		return fmt.Errorf("connect to IIOD: %w", err)
	}

	p.logEvent("info", "IIO: Connected successfully")

	devices, err := client.ListDevices()
	if err != nil {
		p.logEvent("error", fmt.Sprintf("IIO: Failed to list devices: %v", err))
		return fmt.Errorf("list devices: %w", err)
	}

	p.logEvent("debug", fmt.Sprintf("IIO: Found %d devices", len(devices)))

	phy, rx, tx := identifyAD9361Devices(devices)
	if phy == "" || rx == "" || tx == "" {
		_ = client.Close()
		p.logEvent("error", fmt.Sprintf("IIO: AD9361 devices not found (phy=%q rx=%q tx=%q)", phy, rx, tx))
		return fmt.Errorf("unable to locate AD9361 devices (phy=%q rx=%q tx=%q)", phy, rx, tx)
	}

	p.logEvent("info", fmt.Sprintf("IIO: Found AD9361 devices - PHY: %s, RX: %s, TX: %s", phy, rx, tx))

	// Program sample rate and LOs.
	p.logEvent("debug", fmt.Sprintf("IIO: Setting sample rate to %.0f Hz", cfg.SampleRate))
	if err := client.WriteAttr(phy, "", "sampling_frequency", fmt.Sprintf("%.0f", cfg.SampleRate)); err != nil {
		_ = client.Close()
		p.logEvent("error", fmt.Sprintf("IIO: Failed to set sample rate: %v", err))
		return fmt.Errorf("set sample rate: %w", err)
	}

	if cfg.RxLO > 0 {
		p.logEvent("debug", fmt.Sprintf("IIO: Setting RX LO to %.0f Hz", cfg.RxLO))
		if err := client.WriteAttr(phy, "altvoltage1", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			_ = client.Close()
			p.logEvent("error", fmt.Sprintf("IIO: Failed to set RX LO: %v", err))
			return fmt.Errorf("set RX LO: %w", err)
		}

		p.logEvent("debug", fmt.Sprintf("IIO: Setting TX LO to %.0f Hz", cfg.RxLO))
		if err := client.WriteAttr(phy, "altvoltage0", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			_ = client.Close()
			p.logEvent("error", fmt.Sprintf("IIO: Failed to set TX LO: %v", err))
			return fmt.Errorf("set TX LO: %w", err)
		}
	}

	// Configure RX gains.
	p.logEvent("debug", "IIO: Configuring RX gains")
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

	p.logEvent("info", fmt.Sprintf("IIO: Creating RX buffer (%d samples)", cfg.NumSamples))
	rxBuf, err := client.CreateStreamBuffer(rx, cfg.NumSamples, 0x3)
	if err != nil {
		_ = client.Close()
		p.logEvent("error", fmt.Sprintf("IIO: Failed to create RX buffer: %v", err))
		return fmt.Errorf("create RX buffer: %w", err)
	}

	p.logEvent("info", fmt.Sprintf("IIO: Creating TX buffer (%d samples)", cfg.NumSamples))
	txBuf, err := client.CreateStreamBuffer(tx, cfg.NumSamples, 0x3)
	if err != nil {
		_ = rxBuf.Close()
		_ = client.Close()
		p.logEvent("error", fmt.Sprintf("IIO: Failed to create TX buffer: %v", err))
		return fmt.Errorf("create TX buffer: %w", err)
	}

	p.client = client
	p.phyDev = phy
	p.rxDev = rx
	p.txDev = tx
	p.rxBuffer = rxBuf
	p.txBuffer = txBuf
	p.numSamples = cfg.NumSamples

	p.logEvent("info", "IIO: Pluto SDR initialized successfully")

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
		atomic.AddUint64(&p.rxUnderruns, 1)
		p.logEvent("warn", fmt.Sprintf("IIO: RX buffer read failed: %v", err))
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
		atomic.AddUint64(&p.txOverruns, 1)
		p.logEvent("warn", fmt.Sprintf("IIO: TX buffer write failed: %v", err))
		return fmt.Errorf("write TX buffer: %w", err)
	}

	return nil
}

// Close releases buffers and the underlying IIOD connection.
func (p *PlutoSDR) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.logEvent("info", "IIO: Closing Pluto SDR")

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

	if firstErr == nil {
		p.logEvent("info", "IIO: Pluto SDR closed successfully")
	} else {
		p.logEvent("error", fmt.Sprintf("IIO: Error during close: %v", firstErr))
	}

	return firstErr
}

// SetPhaseDelta is a no-op for hardware backends.
func (p *PlutoSDR) SetPhaseDelta(phaseDeltaDeg float64) {}

// GetPhaseDelta returns 0 for hardware backends.
func (p *PlutoSDR) GetPhaseDelta() float64 { return 0 }

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
