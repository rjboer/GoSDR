package sdr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	sshWriter   *SSHAttributeWriter
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
	// Don't lock mutex here - this is called from within locked sections
	// Just read the fields directly (they're set before Init is called)
	if p.eventLogger != nil && p.debugMode {
		p.eventLogger.LogEvent(level, message)
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
func (p *PlutoSDR) Init(ctx context.Context, cfg Config) error {
	fmt.Printf("[PLUTO DEBUG] Init() called with URI=%s, SampleRate=%.0f\n", cfg.URI, cfg.SampleRate)
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.URI == "" {
		cfg.URI = "192.168.2.1:30431"
	}

	sshHost := cfg.SSHHost
	if sshHost == "" {
		sshHost = extractHostFromURI(cfg.URI)
	}

	// Add default IIOD port if not specified
	if !strings.Contains(cfg.URI, ":") {
		cfg.URI = cfg.URI + ":30431"
	}

	if cfg.NumSamples <= 0 {
		cfg.NumSamples = 1024
	}
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}

	p.logEvent("info", fmt.Sprintf("IIO: Connecting to %s", cfg.URI))
	fmt.Printf("[PLUTO DEBUG] Attempting to connect to %s...\n", cfg.URI)
	fmt.Printf("[PLUTO DEBUG] About to call iiod.Dial()...\n")

	client, err := iiod.Dial(cfg.URI)

	fmt.Printf("[PLUTO DEBUG] iiod.Dial() returned, err=%v\n", err)
	if err != nil {
		p.logEvent("error", fmt.Sprintf("IIO: Connection failed: %v", err))
		fmt.Printf("[PLUTO DEBUG] Connection FAILED: %v\n", err)
		return fmt.Errorf("connect to IIOD: %w", err)
	}

	p.logEvent("info", "IIO: Connected successfully")
	fmt.Printf("[PLUTO DEBUG] Connected successfully!\n")

	fmt.Printf("[PLUTO DEBUG] Calling ListDevices() with 2s timeout...\n")
	listCtx, listCancel := context.WithTimeout(ctx, 2*time.Second)
	defer listCancel()

	devices, err := client.ListDevicesWithContext(listCtx)
	fmt.Printf("[PLUTO DEBUG] ListDevices() returned: devices=%v, err=%v\n", devices, err)

	if err != nil || len(devices) == 0 {
		// Older IIOD versions: try XML parsing, then fall back to hardcoded names
		if err != nil {
			p.logEvent("warn", fmt.Sprintf("IIO: LIST_DEVICES failed (%v), trying XML context", err))
			fmt.Printf("[PLUTO DEBUG] LIST_DEVICES failed: %v, trying XML context\n", err)
		} else {
			p.logEvent("warn", "IIO: LIST_DEVICES returned empty, trying XML context")
			fmt.Printf("[PLUTO DEBUG] LIST_DEVICES returned empty, trying XML context\n")
		}

		// Try to get devices from XML
		fmt.Printf("[PLUTO DEBUG] Calling ListDevicesFromXML()...\n")
		xmlDevices, xmlErr := client.ListDevicesFromXML(context.Background())
		fmt.Printf("[PLUTO DEBUG] ListDevicesFromXML() returned: devices=%v, err=%v\n", xmlDevices, xmlErr)

		if xmlErr == nil && len(xmlDevices) > 0 {
			devices = xmlDevices
			p.logEvent("info", "IIO: Successfully parsed devices from XML context")
			fmt.Printf("[PLUTO DEBUG] Parsed %d devices from XML\n", len(xmlDevices))
		} else {
			// Final fallback: hardcoded AD9361 device names
			p.logEvent("warn", "IIO: XML parsing failed, using hardcoded AD9361 device names")
			fmt.Printf("[PLUTO DEBUG] XML parsing failed, using hardcoded device names\n")
			devices = []string{"ad9361-phy", "cf-ad9361-lpc", "cf-ad9361-dds-core-lpc"}
		}
		xml, err := client.DumpRawXML()
		if err != nil {
			fmt.Printf("Failed to dump XML: %v", err)
		}

		fmt.Println("=== RAW PLUTO XML ===")
		fmt.Println(xml)

	}

	p.logEvent("debug", fmt.Sprintf("IIO: Found %d devices", len(devices)))
	fmt.Printf("[PLUTO DEBUG] Found %d devices: %v\n", len(devices), devices)

	phy, rx, tx := identifyAD9361Devices(devices)
	if phy == "" || rx == "" || tx == "" {
		_ = client.Close()
		p.logEvent("error", fmt.Sprintf("IIO: AD9361 devices not found (phy=%q rx=%q tx=%q)", phy, rx, tx))
		fmt.Printf("[PLUTO DEBUG] AD9361 devices not found (phy=%q rx=%q tx=%q)\n", phy, rx, tx)
		return fmt.Errorf("unable to locate AD9361 devices (phy=%q rx=%q tx=%q)", phy, rx, tx)
	}

	iiodWriteSupported := client.SupportsWrite()
	if !iiodWriteSupported {
		p.logEvent("warn", fmt.Sprintf("IIO: Remote IIOD protocol v0.%d does not support attribute writes; enabling SSH sysfs fallback", client.ProtocolVersion.Minor))
	}

	sshCfg := SSHConfig{
		Host:      sshHost,
		User:      cfg.SSHUser,
		Password:  cfg.SSHPassword,
		KeyPath:   cfg.SSHKeyPath,
		Port:      cfg.SSHPort,
		SysfsRoot: cfg.SysfsRoot,
	}

	var warnedFallback bool
	writeAttr := func(action, device, channel, attr, value string) error {
		if err := client.WriteAttrCompat(ctx, device, channel, attr, value); err != nil {
			if errors.Is(err, iiod.ErrWriteNotSupported) {
				writer, sshErr := p.ensureSSHFallbackLocked(sshCfg)
				if sshErr != nil {
					p.logEvent("error", fmt.Sprintf("IIO: %s unsupported via IIOD and SSH fallback unavailable: %v", action, sshErr))
					return fmt.Errorf("%s: %w", action, err)
				}
				if !warnedFallback {
					p.logEvent("warn", fmt.Sprintf("IIO: %s unsupported via IIOD; using SSH sysfs fallback to %s", action, sshHost))
					warnedFallback = true
				}
				if sshErr := writer.WriteAttribute(ctx, device, channel, attr, value); sshErr != nil {
					p.logEvent("error", fmt.Sprintf("IIO: SSH sysfs %s failed: %v", action, sshErr))
					return fmt.Errorf("%s via ssh: %w", action, sshErr)
				}
				return nil
			}

			p.logEvent("error", fmt.Sprintf("IIO: Failed to %s: %v", action, err))
			return fmt.Errorf("%s: %w", action, err)
		}

		return nil
	}

	p.logEvent("info", fmt.Sprintf("IIO: Found AD9361 devices - PHY: %s, RX: %s, TX: %s", phy, rx, tx))
	fmt.Printf("[PLUTO DEBUG] Found AD9361: PHY=%s, RX=%s, TX=%s\n", phy, rx, tx)

	// Program sample rate and LOs.
	p.logEvent("debug", fmt.Sprintf("IIO: Setting sample rate to %.0f Hz", cfg.SampleRate))
	if err := writeAttr("set sample rate", phy, "", "sampling_frequency", fmt.Sprintf("%.0f", cfg.SampleRate)); err != nil {
		_ = client.Close()
		return err
	}

	if cfg.RxLO > 0 {
		p.logEvent("debug", fmt.Sprintf("IIO: Setting RX LO to %.0f Hz", cfg.RxLO))
		if err := writeAttr("set RX LO", phy, "altvoltage1", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			_ = client.Close()
			return err
		}

		p.logEvent("debug", fmt.Sprintf("IIO: Setting TX LO to %.0f Hz", cfg.RxLO))
		if err := writeAttr("set TX LO", phy, "altvoltage0", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			_ = client.Close()
			return err
		}
	}

	// Configure RX gains.
	p.logEvent("debug", "IIO: Configuring RX gains")
	if err := writeAttr("set rx0 gain mode", phy, "voltage0", "gain_control_mode", "manual"); err != nil {
		_ = client.Close()
		return err
	}
	if err := writeAttr("set rx1 gain mode", phy, "voltage1", "gain_control_mode", "manual"); err != nil {
		_ = client.Close()
		return err
	}
	if err := writeAttr("set rx0 gain", phy, "voltage0", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain0)); err != nil {
		_ = client.Close()
		return err
	}
	if err := writeAttr("set rx1 gain", phy, "voltage1", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain1)); err != nil {
		_ = client.Close()
		return err
	}
	if err := writeAttr("set tx gain", phy, "out", "hardwaregain", fmt.Sprintf("%d", cfg.TxGain)); err != nil {
		// Some firmware exposes TX gain per-channel; fall back without failing hard.
		p.logEvent("warn", fmt.Sprintf("IIO: TX gain not applied: %v", err))
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

func (p *PlutoSDR) ensureSSHFallbackLocked(cfg SSHConfig) (*SSHAttributeWriter, error) {
	if p.sshWriter != nil {
		return p.sshWriter, nil
	}

	writer, err := NewSSHAttributeWriter(cfg)
	if err != nil {
		return nil, err
	}
	p.sshWriter = writer
	return p.sshWriter, nil
}

func extractHostFromURI(uri string) string {
	parts := strings.Split(uri, ":")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if len(parts) >= 2 {
		if _, err := strconv.Atoi(last); err == nil {
			return parts[len(parts)-2]
		}
	}
	return last
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
