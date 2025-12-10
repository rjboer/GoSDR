package sdr

import (
	"context"
	"fmt"
	"time"
)

//
// PART 2: ATTRIBUTE HELPERS + CHANNEL DISCOVERY + RADIO CONFIG
//

// getAttr reads an attribute from a device/channel.
// Automatically falls back to text mode when binary metadata is missing.
func (p *PlutoSDR) getAttr(ctx context.Context, dev, channel, attr string) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("client not initialized")
	}
	return p.client.ReadAttrCompat(ctx, dev, channel, attr)
}

// setAttr writes an attribute to a device/channel.
func (p *PlutoSDR) setAttr(ctx context.Context, dev, channel, attr, value string) error {
	if p.client == nil {
		return fmt.Errorf("client not initialized")
	}
	return p.client.WriteAttrCompat(ctx, dev, channel, attr, value)
}

//
// RADIO CHANNEL DISCOVERY
//

// findRXChannels returns the list of RX channels for the AD9361.
func (p *PlutoSDR) findRXChannels(ctx context.Context) ([]string, error) {
	if p.rxDev == "" {
		return nil, fmt.Errorf("RX device not assigned")
	}

	chs, err := p.client.GetChannelsWithContext(ctx, p.rxDev)
	if err != nil {
		return nil, fmt.Errorf("GetChannels failed: %w", err)
	}

	var out []string
	for _, ch := range chs {
		if ch.Type == "input" && ch.ID != "" {
			out = append(out, ch.ID)
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

	chs, err := p.client.GetChannelsWithContext(ctx, p.txDev)
	if err != nil {
		return nil, fmt.Errorf("GetChannels failed: %w", err)
	}

	var out []string
	for _, ch := range chs {
		if ch.Type == "output" && ch.ID != "" {
			out = append(out, ch.ID)
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

// configureAD9361 performs full radio configuration flow.
func (p *PlutoSDR) configureAD9361(ctx context.Context) error {
	if p.phyDev == "" || p.rxDev == "" || p.txDev == "" {
		return fmt.Errorf("devices not assigned")
	}

	// 1) Configure LO frequencies
	if p.sampleRate > 0 {
		// RX LO is p.rxLO (external), but fallback to sampleRate*1000 if not set externally
		if p.rxLO == 0 {
			p.rxLO = p.sampleRate * 1000
		}
		if p.txLO == 0 {
			p.txLO = p.rxLO
		}
	}

	if err := p.setRXLO(ctx, p.rxLO); err != nil {
		return err
	}
	if err := p.setTXLO(ctx, p.txLO); err != nil {
		return err
	}

	// 2) Configure sampling rate for RX/TX
	rxChs, err := p.findRXChannels(ctx)
	if err != nil {
		return err
	}
	txChs, err := p.findTXChannels(ctx)
	if err != nil {
		return err
	}

	for _, ch := range rxChs {
		if err := p.setSampleRate(ctx, p.rxDev, ch, p.sampleRate); err != nil {
			return fmt.Errorf("setSampleRate RX[%s] failed: %w", ch, err)
		}
	}

	for _, ch := range txChs {
		if err := p.setSampleRate(ctx, p.txDev, ch, p.sampleRate); err != nil {
			return fmt.Errorf("setSampleRate TX[%s] failed: %w", ch, err)
		}
	}

	// 3) Configure bandwidth (5/6 of sample rate)
	bw := p.sampleRate * 5 / 6
	for _, ch := range rxChs {
		if err := p.setBandwidth(ctx, p.rxDev, ch, bw); err != nil {
			return fmt.Errorf("setBandwidth RX[%s] failed: %w", ch, err)
		}
	}

	// 4) Configure gain
	for _, ch := range []string{"voltage0", "voltage1"} {
		if err := p.setGainControlMode(ctx, ch, "manual"); err != nil {
			return fmt.Errorf("set gain_control_mode failed: %w", err)
		}
		if err := p.setHardwareGain(ctx, ch, float64(p.rxGain)); err != nil {
			return fmt.Errorf("set hardwaregain failed: %w", err)
		}
	}

	return nil
}

//
// TIMEOUT UTILITY
//

func (p *PlutoSDR) ctxShort() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Second)
}
