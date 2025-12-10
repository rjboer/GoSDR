package sdr

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/rjboer/GoSDR/iiod"
)

//
// PART 4: PUBLIC API + DEBUG LOGGING + LIFECYCLE MGMT
//

// EnableDebug toggles verbose PlutoSDR logging.
func (p *PlutoSDR) EnableDebug(enable bool) {
	p.debug = enable
}

func (p *PlutoSDR) dbg(msg string, args ...interface{}) {
	if p.debug {
		log.Printf("[PLUTO DEBUG] "+msg, args...)
	}
}

//
// PUBLIC: ATTRIBUTES
//

func (p *PlutoSDR) GetAttr(dev, channel, attr string) (string, error) {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.getAttr(ctx, dev, channel, attr)
}

func (p *PlutoSDR) SetAttr(dev, channel, attr, value string) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.setAttr(ctx, dev, channel, attr, value)
}

//
// PUBLIC: FREQUENCY CONTROL
//

func (p *PlutoSDR) SetRXLO(freq uint64) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.setRXLO(ctx, freq)
}

func (p *PlutoSDR) SetTXLO(freq uint64) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.setTXLO(ctx, freq)
}

func (p *PlutoSDR) GetRXLO() (uint64, error) {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.getRXLO(ctx)
}

func (p *PlutoSDR) GetTXLO() (uint64, error) {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.getTXLO(ctx)
}

//
// PUBLIC: GAIN CONTROL
//

func (p *PlutoSDR) SetRXGain(channel string, gain float64) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.setHardwareGain(ctx, channel, gain)
}

func (p *PlutoSDR) SetRXGainControlMode(channel string, mode string) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.setGainControlMode(ctx, channel, mode)
}

//
// PUBLIC: VERSION
//

// GetVersion returns IIOD major/minor/git.
func (p *PlutoSDR) GetVersion() (int, int, string) {
	if p.client == nil {
		return 0, 0, ""
	}
	return p.client.VersionMajor, p.client.VersionMinor, p.client.VersionGit
}

//
// RX/TX RUNTIME MANAGEMENT
//

// StartRX allocates an RX buffer for continuous reads.
func (p *PlutoSDR) StartRX(bufferLen int) (*PlutoBuffer, error) {
	p.dbg("Starting RX buffer, len=%d", bufferLen)

	ctx, cancel := p.ctxIO()
	defer cancel()

	buf, err := p.createRXBuffer(ctx, bufferLen)
	if err != nil {
		return nil, fmt.Errorf("StartRX failed: %w", err)
	}
	return buf, nil
}

// StartTX allocates a TX buffer. cyclic=true enables continuous transmit.
func (p *PlutoSDR) StartTX(bufferLen int, cyclic bool) (*PlutoBuffer, error) {
	p.dbg("Starting TX buffer, len=%d cyclic=%v", bufferLen, cyclic)

	ctx, cancel := p.ctxIO()
	defer cancel()

	buf, err := p.createTXBuffer(ctx, bufferLen, cyclic)
	if err != nil {
		return nil, fmt.Errorf("StartTX failed: %w", err)
	}
	return buf, nil
}

//
// READ/WRITE WRAPPERS
//

func (p *PlutoSDR) ReadSamples(buf *PlutoBuffer) ([]byte, error) {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.readRX(ctx, buf)
}

func (p *PlutoSDR) ReadIQSamples(buf *PlutoBuffer) ([]complex64, error) {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.ReadIQ(ctx, buf)
}

func (p *PlutoSDR) WriteSamples(buf *PlutoBuffer, data []byte) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.writeTX(ctx, buf, data)
}

func (p *PlutoSDR) WriteIQSamples(buf *PlutoBuffer, samples []complex64) error {
	ctx, cancel := p.ctxIO()
	defer cancel()
	return p.WriteIQ(ctx, buf, samples)
}

//
// STOP / SHUTDOWN
//

func (p *PlutoSDR) StopBuffer(buf *PlutoBuffer) error {
	ctx, cancel := p.ctxIO()
	defer cancel()

	if buf == nil {
		return nil
	}

	p.dbg("Closing buffer on %s", buf.dev)
	return p.closeBuffer(ctx, buf)
}

func (p *PlutoSDR) Close() error {
	p.dbg("Closing PlutoSDR")
	if p.client != nil {
		_ = p.client.Close()
	}
	return nil
}

//
// HIGH-LEVEL INIT ENTRYPOINT
//

// Init configures the whole PlutoSDR pipeline.
func (p *PlutoSDR) Init() error {
	p.dbg("Init() called with URI=%s SampleRate=%d", p.uri, p.sampleRate)

	if p.client != nil {
		return fmt.Errorf("Init called twice")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	client, err := iiod.Dial(ctx, p.uri)
	if err != nil {
		return fmt.Errorf("Dial failed: %w", err)
	}
	p.client = client

	p.dbg("Connected to IIOD: v%d.%d (%s)",
		p.client.VersionMajor, p.client.VersionMinor, p.client.VersionGit)

	// Device discovery
	if err := p.identifyDevices(ctx); err != nil {
		return fmt.Errorf("device discovery failed: %w", err)
	}

	// Core radio config
	if err := p.configureAD9361(ctx); err != nil {
		return fmt.Errorf("radio init failed: %w", err)
	}

	p.dbg("PlutoSDR initialized successfully")
	return nil
}
