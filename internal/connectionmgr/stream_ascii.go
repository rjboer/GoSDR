package connectionmgr

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// StreamASCIIConfig controls continuous streaming via legacy ASCII READBUF.
type StreamASCIIConfig struct {
	DeviceID string

	// BytesPerRead is the READBUF length argument. Example: 65536.
	// Keep this reasonably sized (64KiB..1MiB) to avoid latency spikes.
	BytesPerRead int

	// Out is where raw payload chunks are delivered.
	// Backpressure: if the channel is full, streaming blocks unless DropIfFull is true.
	Out chan<- []byte

	// DropIfFull: if true, drop a frame when Out is full (instead of blocking).
	DropIfFull bool

	// CopyOut: if true, each delivered chunk is copied to a fresh slice
	// so the caller can retain it. If false, delivered slices may be reused.
	// In practice, keep this true unless you implement a pool in the consumer.
	CopyOut bool

	// ReadTimeoutPerChunk overrides socket read deadline per READBUF transaction (optional).
	// If zero, Manager.Timeout is used.
	ReadTimeoutPerChunk time.Duration

	// LogPrefix is included in logs for correlation.
	LogPrefix string
}

// StreamASCIIHandle controls a running stream.
type StreamASCIIHandle struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	errMu  sync.Mutex
	err    error
}

func (h *StreamASCIIHandle) Stop() {
	if h == nil {
		return
	}
	h.cancel()
	h.wg.Wait()
}

func (h *StreamASCIIHandle) Err() error {
	if h == nil {
		return nil
	}
	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.err
}

func (h *StreamASCIIHandle) setErr(err error) {
	h.errMu.Lock()
	defer h.errMu.Unlock()
	if h.err == nil {
		h.err = err
	}
}

// StartStreamASCII runs a continuous READBUF loop on an already-open Manager connection.
// It assumes you already did: Connect -> TIMEOUT -> PRINT/XML -> OPEN buffer.
func (m *Manager) StartStreamASCII(parent context.Context, cfg StreamASCIIConfig) (*StreamASCIIHandle, error) {
	if m == nil {
		return nil, errors.New("nil Manager")
	}
	if m.conn == nil {
		return nil, errors.New("manager not connected")
	}
	if cfg.DeviceID == "" {
		return nil, errors.New("DeviceID is required")
	}
	if cfg.BytesPerRead <= 0 {
		return nil, errors.New("BytesPerRead must be > 0")
	}
	if cfg.Out == nil {
		return nil, errors.New("Out channel is required")
	}

	ctx, cancel := context.WithCancel(parent)
	h := &StreamASCIIHandle{cancel: cancel}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		pfx := cfg.LogPrefix
		if pfx == "" {
			pfx = "stream"
		}

		log.Printf("[%s] start: device=%s bytesPerRead=%d dropIfFull=%v copyOut=%v",
			pfx, cfg.DeviceID, cfg.BytesPerRead, cfg.DropIfFull, cfg.CopyOut)

		// Single reusable buffer for the read transaction.
		// We still *may* copy before sending (cfg.CopyOut).
		buf := make([]byte, cfg.BytesPerRead)

		for {
			select {
			case <-ctx.Done():
				log.Printf("[%s] stop requested", pfx)
				return
			default:
			}

			// Optional per-transaction deadline tightening.
			if cfg.ReadTimeoutPerChunk > 0 {
				_ = m.conn.SetReadDeadline(time.Now().Add(cfg.ReadTimeoutPerChunk))
			} else if m.Timeout > 0 {
				_ = m.conn.SetReadDeadline(time.Now().Add(m.Timeout))
			}

			// Perform one READBUF transaction.
			// IMPORTANT: ReadBufferASCII must stop when it has read the requested length
			// (do NOT wait for a trailing "0" chunk, because servers may keep streaming).
			n, err := m.ReadBufferASCII(cfg.DeviceID, buf[:cfg.BytesPerRead])
			if err != nil {
				h.setErr(fmt.Errorf("ReadBufferASCII: %w", err))
				log.Printf("[%s] error: %v", pfx, err)
				return
			}
			if n <= 0 {
				// Avoid tight loop if device is momentarily not producing.
				time.Sleep(10 * time.Millisecond)
				continue
			}

			payload := buf[:n]
			if cfg.CopyOut {
				tmp := make([]byte, len(payload))
				copy(tmp, payload)
				payload = tmp
			}

			if cfg.DropIfFull {
				select {
				case cfg.Out <- payload:
				default:
					// Drop frame.
					log.Printf("[%s] drop: out channel full (len=%d)", pfx, len(payload))
				}
			} else {
				// Backpressure blocks here.
				select {
				case cfg.Out <- payload:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return h, nil
}
