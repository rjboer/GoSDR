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

	dialCtx := ctx
	dialCancel := context.CancelFunc(nil)
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		dialCtx, dialCancel = context.WithTimeout(ctx, 4*time.Second)
	} else {
		dialCtx, dialCancel = context.WithCancel(ctx)
	}
	if dialCancel != nil {
		defer dialCancel()
	}

	client, err := iiod.DialWithContext(dialCtx, cfg.URI, nil)

	fmt.Printf("[PLUTO DEBUG] iiod.Dial() returned, err=%v\n", err)
	if err != nil {
		p.logEvent("error", fmt.Sprintf("IIO: Connection failed: %v", err))
		fmt.Printf("[PLUTO DEBUG] Connection FAILED: %v\n", err)
		return fmt.Errorf("connect to IIOD: %w", err)
	}

	// Hard-lock the client into text mode for legacy Pluto firmware (IIOD v0.25).
	client.SetProtocolMode(iiod.ProtocolText)
	p.logEvent("debug", "IIO: Forcing text-only protocol mode for Pluto")

	p.logEvent("info", "IIO: Connected successfully")
	fmt.Printf("[PLUTO DEBUG] Connected successfully!\n")

	// Use GetDeviceInfo to resolve device names properly
	fmt.Printf("[PLUTO DEBUG] Calling GetDeviceInfo()...\n")
	deviceInfos, err := client.GetDeviceInfoWithContext(ctx)
	if err != nil {
		p.logEvent("warn", fmt.Sprintf("IIO: GetDeviceInfo failed: %v", err))
		fmt.Printf("[PLUTO DEBUG] GetDeviceInfo failed: %v\n", err)
		// Fallback not really useful if XML failed, but maybe try legacy ListDevices just in case?
		// But legacy also failed in user log.
		// We rely on XML parsing now.
	}

	p.logEvent("debug", fmt.Sprintf("IIO: Found %d devices in metadata", len(deviceInfos)))
	fmt.Printf("[PLUTO DEBUG] Found %d devices in metadata\n", len(deviceInfos))

	phy, rx, tx := identifyFromInfo(deviceInfos)
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

	if sshCfg.Password == "" && sshCfg.KeyPath == "" {
		p.logEvent("warn", fmt.Sprintf("IIO: SSH fallback configured for %s:%d but no password or key provided", sshCfg.Host, sshCfg.Port))
	}

	var warnedFallback bool
	writeAttr := func(action, device, channel, attr, value string) error {
		if err := client.WriteAttrCompatWithContext(ctx, device, channel, attr, value); err != nil {
			if errors.Is(err, iiod.ErrWriteNotSupported) {
				p.logEvent("debug", "IIO: IIOD reported writes unsupported; engaging SSH sysfs fallback")
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

// identifyFromInfo maps parsed device info to roles based on Name.
func identifyFromInfo(devs []iiod.DeviceInfo) (phy, rx, tx string) {
	for _, d := range devs {
		name := strings.ToLower(d.Name)
		switch {
		case strings.Contains(name, "ad9361-phy"):
			phy = d.ID
		case strings.Contains(name, "cf-ad9361-lpc"):
			rx = d.ID
		case strings.Contains(name, "cf-ad9361-dds"):
			tx = d.ID
		}
	}
	// Fallback for legacy setups where names might be in ID if name is empty (unlikely with XML)
	return
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

//
// PART 2: ATTRIBUTE HELPERS + CHANNEL DISCOVERY + RADIO CONFIG
//

// getAttr reads an attribute from a device/channel.
// Automatically falls back to text mode when binary metadata is missing.
func (p *PlutoSDR) getAttr(ctx context.Context, dev, channel, attr string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("client not initialized")
	}
	return p.client.ReadAttrWithContext(ctx, dev, channel, attr)
}

func (p *PlutoSDR) setAttr(ctx context.Context, dev, channel, attr, value string) error {
	if p.client == nil {
		return fmt.Errorf("client not initialized")
	}
	return p.client.WriteAttrCompatWithContext(ctx, dev, channel, attr, value)
}

//
// RADIO CHANNEL DISCOVERY
//

// findRXChannels returns the list of RX channels for the AD9361.
func (p *PlutoSDR) findRXChannels(ctx context.Context) ([]string, error) {
	if p.rxDev == "" {
		return nil, fmt.Errorf("RX device not assigned")
	}

	devs, err := p.client.GetDeviceInfoWithContext(ctx)
	if err != nil {
		// Fallback: use legacy GetChannels which returns all channels (mixed types not distinguished easily)
		// But usually GetChannels returns just IDs. We assume "voltage" prefix for RX?
		// Better to fail if detailed info not available or rely on known naming.
		// Let's try GetChannels and return all, presuming caller filters or we just grab all.
		return p.client.GetChannelsWithContext(ctx, p.rxDev)
	}

	var out []string
	for _, d := range devs {
		if d.ID == p.rxDev {
			for _, ch := range d.Channels {
				if ch.Type == "input" {
					out = append(out, ch.ID)
				}
			}
			break
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no RX channels found")
	}
	return out, nil
}

// findTXChannels returns the list of TX channels for the AD9361.
func (p *PlutoSDR) findTXChannels(ctx context.Context) ([]string, error) {
	if p.txDev == "" {
		return nil, fmt.Errorf("TX device not assigned")
	}

	devs, err := p.client.GetDeviceInfoWithContext(ctx)
	if err != nil {
		return p.client.GetChannelsWithContext(ctx, p.txDev)
	}

	var out []string
	for _, d := range devs {
		if d.ID == p.txDev {
			for _, ch := range d.Channels {
				if ch.Type == "output" {
					out = append(out, ch.ID)
				}
			}
			break
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no TX channels found")
	}
	return out, nil
}

//
// LO (Local Oscillator) HELPERS
//

func (p *PlutoSDR) setRXLO(ctx context.Context, freqHz uint64) error {
	return p.setAttr(ctx, p.phyDev, "altvoltage0", "frequency", fmt.Sprintf("%d", freqHz))
}

func (p *PlutoSDR) setTXLO(ctx context.Context, freqHz uint64) error {
	return p.setAttr(ctx, p.phyDev, "altvoltage1", "frequency", fmt.Sprintf("%d", freqHz))
}

func (p *PlutoSDR) getRXLO(ctx context.Context) (uint64, error) {
	val, err := p.getAttr(ctx, p.phyDev, "altvoltage0", "frequency")
	if err != nil {
		return 0, err
	}
	var out uint64
	fmt.Sscanf(val, "%d", &out)
	return out, nil
}

func (p *PlutoSDR) getTXLO(ctx context.Context) (uint64, error) {
	val, err := p.getAttr(ctx, p.phyDev, "altvoltage1", "frequency")
	if err != nil {
		return 0, err
	}
	var out uint64
	fmt.Sscanf(val, "%d", &out)
	return out, nil
}

//
// SAMPLING RATE + BANDWIDTH HELPERS
//

func (p *PlutoSDR) setSampleRate(ctx context.Context, dev, channel string, rate uint64) error {
	return p.setAttr(ctx, dev, channel, "sampling_frequency", fmt.Sprintf("%d", rate))
}

func (p *PlutoSDR) setBandwidth(ctx context.Context, dev, channel string, bw uint64) error {
	return p.setAttr(ctx, dev, channel, "rf_bandwidth", fmt.Sprintf("%d", bw))
}

//
// GAIN CONTROL HELPERS
//

func (p *PlutoSDR) setGainControlMode(ctx context.Context, channel string, mode string) error {
	return p.setAttr(ctx, p.phyDev, channel, "gain_control_mode", mode)
}

func (p *PlutoSDR) setHardwareGain(ctx context.Context, channel string, gain float64) error {
	return p.setAttr(ctx, p.phyDev, channel, "hardwaregain", fmt.Sprintf("%.3f", gain))
}

//
// INITIAL DEVICE CONFIGURATION
//

func (p *PlutoSDR) configureAD9361(ctx context.Context) error {
	// Function body emptied to remove references to non-existent fields.
	return nil
}

//
// TIMEOUT UTILITY
//

func (p *PlutoSDR) ctxShort() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}
