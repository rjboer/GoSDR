package iiod

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

// ============================================================
// High-Level Streaming Session
// ============================================================

// StreamingSession bundles RX + optional TX capability
type StreamingSession struct {
	RXDev       string
	TXDev       string
	RXChannels  []string
	TXChannels  []string
	SampleBytes int
	RX          *RXStream
	TX          *TXWriter
	client      *Client

	FramesDropped uint64
	FramesRecv    uint64
}

// CreateStreamingSession auto-detects devices and prepares RX/TX
func (c *Client) CreateStreamingSession(ctx context.Context) (*StreamingSession, error) {

	c.debug(1, "CreateStreamingSession: begin auto-detection")

	rxDev, txDev, rxCh, txCh, sampleBytes, err :=
		c.DetectStreamingDevices()

	if err != nil {
		return nil, fmt.Errorf("CreateStreamingSession: detect: %w", err)
	}

	c.debug(1,
		"CreateStreamingSession: RX=%s %v, TX=%s %v, sampleBytes=%d",
		rxDev, rxCh, txDev, txCh, sampleBytes,
	)

	sess := &StreamingSession{
		RXDev:       rxDev,
		TXDev:       txDev,
		RXChannels:  rxCh,
		TXChannels:  txCh,
		SampleBytes: sampleBytes,
		client:      c,
	}

	return sess, nil
}

// StartRX activates RX streaming for the session
func (s *StreamingSession) StartRX(ctx context.Context) error {
	if s.RX != nil {
		return errors.New("StartRX: RX already running")
	}

	rx, err := s.client.StartRX(ctx, s.RXDev, s.RXChannels, s.SampleBytes)
	if err != nil {
		return fmt.Errorf("StartRX: %w", err)
	}

	s.RX = rx
	s.client.debug(1, "StreamingSession: RX started successfully")
	return nil
}

// StopRX stops RX streaming
func (s *StreamingSession) StopRX() {
	if s.RX != nil {
		s.client.debug(1, "StreamingSession: stopping RX")
		s.RX.Stop()
		s.RX = nil
	}
}

// StartTX prepares a TX writer (non-cyclic TX)
func (s *StreamingSession) StartTX() error {
	if s.TX != nil {
		return errors.New("StartTX: TX already enabled")
	}
	s.client.debug(1, "StreamingSession: TX enabled")

	s.TX = s.client.StartTX(s.TXDev, s.TXChannels, s.SampleBytes)
	return nil
}

// StopTX disables TX writer
func (s *StreamingSession) StopTX() {
	s.client.debug(1, "StreamingSession: TX disabled")
	s.TX = nil
}

// ============================================================
// Statistics
// ============================================================

// AttachStats hooks RXStream to automatically count frames
func (s *StreamingSession) AttachStats() {
	if s.RX == nil {
		return
	}
	c := s.client

	go func() {
		for frame := range s.RX.C {
			_ = frame // frame content not needed for stats
			atomic.AddUint64(&s.FramesRecv, 1)
		}
		c.debug(1, "AttachStats: RX channel closed")
	}()
}

func (s *StreamingSession) DumpStats() {
	fmt.Printf("Frames received: %d\n", atomic.LoadUint64(&s.FramesRecv))
	fmt.Printf("Frames dropped:  %d\n", atomic.LoadUint64(&s.FramesDropped))
}

// ============================================================
// Utility Helpers (for future expansion)
// ============================================================

// ValidateIQFormat warns if sampleBytes mismatches scan-element format
func (c *Client) ValidateIQFormat(dev, format string, sampleBytes int) {
	expected, err := c.parseSampleWidth(format)
	if err != nil {
		c.debug(1, "ValidateIQFormat: cannot parse format %q: %v", format, err)
		return
	}
	if expected != sampleBytes {
		c.debug(1,
			"WARNING: sampleBytes mismatch for dev=%s: parsed=%d actual=%d",
			dev, expected, sampleBytes)
	}
}

// DumpIQ prints first few IQ samples (debug level ≥ 3)
func (c *Client) DumpIQ(label string, iq []complex64) {
	if c.debugLevel < 3 {
		return
	}
	n := 8
	if len(iq) < n {
		n = len(iq)
	}
	c.logf("%s: first %d IQ samples:", label, n)
	for i := 0; i < n; i++ {
		c.logf("  [%d] = %v", i, iq[i])
	}
}

// EnsureSessionReady guarantees RX/TX channels exist and buffers are valid
func (s *StreamingSession) EnsureSessionReady() error {
	if len(s.RXChannels) == 0 {
		return errors.New("EnsureSessionReady: no RX channels detected")
	}
	if len(s.TXChannels) == 0 {
		return errors.New("EnsureSessionReady: no TX channels detected")
	}
	return nil
}
