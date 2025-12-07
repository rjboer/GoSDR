// PATCHED pluto.go — WriteAttrCompat uses ctx, ReadAttrCompat added,
// IQ helpers are referenced and will be provided next.

package sdr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rjboer/GoSDR/iiod"
)

// -------------------------
// PlutoSDR struct unchanged
// -------------------------
type PlutoSDR struct {
	mu         sync.Mutex
	client     *iiod.Client
	phyDev     string
	rxDev      string
	txDev      string
	rxBuffer   *iiod.Buffer
	txBuffer   *iiod.Buffer
	numSamples int

	eventLogger EventLogger
	rxUnderruns uint64
	txOverruns  uint64
	debugMode   bool
	sshWriter   *SSHAttributeWriter
}

// -------------------------
// Init (patched WriteAttrCompat)
// -------------------------
func (p *PlutoSDR) Init(ctx context.Context, cfg Config) error {
	fmt.Printf("[PLUTO DEBUG] Init() called with URI=%s, SampleRate=%.0f\n", cfg.URI, cfg.SampleRate)
	p.mu.Lock()
	defer p.mu.Unlock()

	if cfg.URI == "" {
		cfg.URI = "192.168.2.1:30431"
	}
	if !strings.Contains(cfg.URI, ":") {
		cfg.URI = cfg.URI + ":30431"
	}
	if cfg.NumSamples <= 0 {
		cfg.NumSamples = 1024
	}
	if cfg.SampleRate <= 0 {
		return fmt.Errorf("sample rate must be positive")
	}

	// ------------- Connect ------------
	client, err := iiod.Dial(cfg.URI)
	fmt.Printf("[PLUTO DEBUG] iiod.Dial() returned, err=%v\n", err)
	if err != nil {
		return fmt.Errorf("connect to IIOD: %w", err)
	}

	// ---------------- Device Discovery ----------------
	listCtx, listCancel := context.WithTimeout(ctx, 2*time.Second)
	defer listCancel()

	devices, err := client.ListDevicesWithContext(listCtx)
	if err != nil || len(devices) == 0 {
		xmlDevices, _ := client.ListDevicesFromXML(context.Background())
		if len(xmlDevices) > 0 {
			devices = xmlDevices
		} else {
			devices = []string{"ad9361-phy", "cf-ad9361-lpc", "cf-ad9361-dds-core-lpc"}
		}
	}

	phy, rx, tx := identifyAD9361Devices(devices)
	if phy == "" || rx == "" || tx == "" {
		_ = client.Close()
		return fmt.Errorf("unable to locate AD9361 devices (phy=%q rx=%q tx=%q)", phy, rx, tx)
	}

	p.phyDev = phy
	p.rxDev = rx
	p.txDev = tx

	// ---------------- Fallback writer ----------------
	sshCfg := SSHConfig{
		Host:      extractHostFromURI(cfg.URI),
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
					return fmt.Errorf("%s: SSH fallback unavailable: %w", action, sshErr)
				}
				if !warnedFallback {
					warnedFallback = true
				}
				return writer.WriteAttribute(ctx, device, channel, attr, value)
			}
			return fmt.Errorf("%s: %w", action, err)
		}
		return nil
	}

	// ---------------- Program attributes ----------------
	if err := writeAttr("set sample rate", phy, "", "sampling_frequency", fmt.Sprintf("%.0f", cfg.SampleRate)); err != nil {
		return err
	}

	if cfg.RxLO > 0 {
		if err := writeAttr("set RX LO", phy, "altvoltage1", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			return err
		}
		if err := writeAttr("set TX LO", phy, "altvoltage0", "frequency", fmt.Sprintf("%.0f", cfg.RxLO)); err != nil {
			return err
		}
	}

	// Gain setup
	_ = writeAttr("set rx0 gain mode", phy, "voltage0", "gain_control_mode", "manual")
	_ = writeAttr("set rx1 gain mode", phy, "voltage1", "gain_control_mode", "manual")
	_ = writeAttr("set rx0 gain", phy, "voltage0", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain0))
	_ = writeAttr("set rx1 gain", phy, "voltage1", "hardwaregain", fmt.Sprintf("%d", cfg.RxGain1))
	_ = writeAttr("set tx gain", phy, "out", "hardwaregain", fmt.Sprintf("%d", cfg.TxGain))

	// ---------------- Create buffers ----------------
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
	p.rxBuffer = rxBuf
	p.txBuffer = txBuf
	p.numSamples = cfg.NumSamples

	return nil
}

// -----------------------------
// RX — patched to use IQ helpers
// -----------------------------
func (p *PlutoSDR) RX(ctx context.Context) ([]complex64, []complex64, error) {
	p.mu.Lock()
	buf := p.rxBuffer
	p.mu.Unlock()

	if buf == nil {
		return nil, nil, fmt.Errorf("RX buffer not initialized")
	}

	data, err := buf.ReadSamples()
	if err != nil {
		atomic.AddUint64(&p.rxUnderruns, 1)
		return nil, nil, fmt.Errorf("read RX buffer: %w", err)
	}

	samples, err := iiod.ParseInt16Samples(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse RX samples: %w", err)
	}

	// Use missing helpers — will be added next
	i0, q0, err := iiod.DeinterleaveIQ(samples, 2, 0)
	if err != nil {
		return nil, nil, err
	}

	i1, q1, err := iiod.DeinterleaveIQ(samples, 2, 1)
	if err != nil {
		return nil, nil, err
	}

	return iqToComplex(i0, q0), iqToComplex(i1, q1), nil
}

// -----------------------------
// TX — patched to use IQ helpers
// -----------------------------
func (p *PlutoSDR) TX(ctx context.Context, iq0, iq1 []complex64) error {
	p.mu.Lock()
	buf := p.txBuffer
	p.mu.Unlock()

	if buf == nil {
		return fmt.Errorf("TX buffer not initialized")
	}
	if len(iq0) != len(iq1) {
		return fmt.Errorf("TX channel lengths differ")
	}

	i0, q0 := complexToIQ(iq0)
	i1, q1 := complexToIQ(iq1)

	// Missing helper — will be implemented next
	interleaved, err := iiod.InterleaveIQ([][][]int16{
		{i0, q0},
		{i1, q1},
	})
	if err != nil {
		return fmt.Errorf("interleave TX IQ: %w", err)
	}

	data := iiod.FormatInt16Samples(interleaved)
	return buf.WriteSamples(data)
}
