package iiod

import (
	"context"
	"fmt"
	"time"
)

// ============================================================
// Buffer Open / Close / Read / Write (router-level wrappers)
// ============================================================

// 1 MiB RX buffer (H4)
const defaultRXBufferLen = 1 * 1024 * 1024

// OpenBuffer opens a device buffer for RX or TX depending on usage.
// Uses router functions implemented in connect.go.
func (c *Client) OpenBuffer(ctx context.Context, dev string, channels []string, length int) (*BufferHandle, error) {

	if length <= 0 {
		length = defaultRXBufferLen
	}

	c.debug(1, "OpenBuffer(dev=%s, channels=%v, length=%d)", dev, channels, length)

	id, err := c.OpenBufferWithContext(ctx, dev, channels, length)
	if err != nil {
		return nil, fmt.Errorf("OpenBuffer: %w", err)
	}

	h := &BufferHandle{
		ID:     id,
		Device: dev,
		Chans:  channels,
		client: c,
	}

	c.debug(1, "OpenBuffer created handle ID=%d", id)

	return h, nil
}

// Close closes a buffer handle.
func (b *BufferHandle) Close(ctx context.Context) error {
	c := b.client
	c.debug(1, "CloseBuffer(handle=%d, dev=%s)", b.ID, b.Device)
	return c.CloseBufferWithContext(ctx, b.Device, b.ID)
}

// Read reads from an RX buffer.
func (b *BufferHandle) Read(ctx context.Context) ([]byte, error) {
	c := b.client

	c.debug(2, "ReadBuffer(handle=%d, dev=%s)", b.ID, b.Device)

	data, err := c.ReadBufferWithContext(ctx, b.Device, b.ID)
	if err != nil {
		return nil, fmt.Errorf("ReadBuffer: %w", err)
	}

	c.debug(2, "ReadBuffer: got %d bytes", len(data))
	c.debugHex(3, "ReadBuffer", data)

	return data, nil
}

// Write writes to a TX buffer.
func (b *BufferHandle) Write(ctx context.Context, payload []byte) error {
	c := b.client

	c.debug(2, "WriteBuffer(handle=%d, len=%d)", b.ID, len(payload))
	c.debugHex(3, "WriteBuffer payload", payload)

	if err := c.WriteBufferWithContext(ctx, b.Device, b.ID, payload); err != nil {
		return fmt.Errorf("WriteBuffer: %w", err)
	}

	return nil
}

// ============================================================
// RX STREAMING API (F1)
// ============================================================

// StartRX opens an RX buffer and launches a background worker
func (c *Client) StartRX(ctx context.Context, dev string, channels []string, sampleBytes int) (*RXStream, error) {

	c.debug(1, "StartRX(dev=%s, channels=%v, sampleBytes=%d)", dev, channels, sampleBytes)

	buf, err := c.OpenBuffer(ctx, dev, channels, defaultRXBufferLen)
	if err != nil {
		return nil, fmt.Errorf("StartRX: open buffer: %w", err)
	}

	s := &RXStream{
		C:           make(chan []complex64, 16),
		stop:        make(chan struct{}),
		buf:         buf,
		dev:         dev,
		channels:    channels,
		sampleBytes: sampleBytes,
		client:      c,
	}

	s.running.Store(true)

	go s.worker(ctx)

	return s, nil
}

// Worker goroutine that continuously reads from the RX buffer
func (s *RXStream) worker(ctx context.Context) {
	c := s.client

	c.debug(1, "RX worker started for dev=%s", s.dev)

	defer func() {
		s.running.Store(false)
		close(s.C)
		c.debug(1, "RX worker stopped for dev=%s", s.dev)
	}()

	for {
		select {
		case <-ctx.Done():
			c.debug(1, "RX worker ctx canceled")
			return
		case <-s.stop:
			c.debug(1, "RX worker stop requested")
			return
		default:
		}

		raw, err := s.buf.Read(ctx)
		if err != nil {
			c.debug(1, "RX worker read error: %v", err)
			time.Sleep(time.Millisecond * 20)
			continue
		}

		// Convert bytes → complex IQ
		iq, err := c.iqHelper.DeinterleaveIQ(raw, s.sampleBytes)
		if err != nil {
			c.debug(1, "RX worker IQ parse error: %v", err)
			continue
		}

		// Send IQ frame to consumer
		select {
		case s.C <- iq:
		default:
			// drop frame if consumer is slow
			c.debug(2, "RX worker: dropping frame (backpressure)")
		}
	}
}

// Stop gracefully stops RX streaming.
func (s *RXStream) Stop() {
	if s.running.Load() {
		close(s.stop)
	}
}

// ============================================================
// TX WRITER API (G2 — non-cyclic TX)
// ============================================================

// StartTX creates a TX writer for a device and set of channels.
func (c *Client) StartTX(dev string, channels []string, sampleBytes int) *TXWriter {
	c.debug(1, "StartTX(dev=%s, channels=%v, sampleBytes=%d)", dev, channels, sampleBytes)

	return &TXWriter{
		dev:         dev,
		channels:    channels,
		sampleBytes: sampleBytes,
		client:      c,
	}
}

// WriteIQ performs a single-shot TX write.
func (tx *TXWriter) WriteIQ(ctx context.Context, iq []complex64) error {
	c := tx.client

	c.debug(1, "WriteIQ(dev=%s, samples=%d)", tx.dev, len(iq))

	// Convert to interleaved raw bytes
	raw, err := c.iqHelper.InterleaveIQ(iq, tx.sampleBytes)
	if err != nil {
		return fmt.Errorf("WriteIQ: interleave: %w", err)
	}

	// Open TX buffer
	buf, err := c.OpenBuffer(ctx, tx.dev, tx.channels, len(raw))
	if err != nil {
		return fmt.Errorf("WriteIQ: open buffer: %w", err)
	}
	defer buf.Close(ctx)

	// Write payload
	if err := buf.Write(ctx, raw); err != nil {
		return fmt.Errorf("WriteIQ: write: %w", err)
	}

	c.debug(1, "WriteIQ complete")
	return nil
}
