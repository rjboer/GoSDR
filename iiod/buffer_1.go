package iiod

import (
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
)

// ============================================================
// Debug Infrastructure
// ============================================================

func (c *Client) debug(level int, format string, args ...interface{}) {
	if c.debugLevel >= level {
		c.logf(format, args...)
	}
}

// hex dump helper for level ≥ 3
func (c *Client) debugHex(level int, label string, data []byte) {
	if c.debugLevel < level {
		return
	}
	if len(data) == 0 {
		c.logf("[%s] <empty>", label)
		return
	}
	dump := hex.Dump(data)
	c.logf("[%s RAW]:\n%s", label, dump)
}

// ============================================================
// Streaming Types
// ============================================================

type RXStream struct {
	C           chan []complex64
	stop        chan struct{}
	running     atomic.Bool
	buf         *BufferHandle
	dev         string
	channels    []string
	sampleBytes int
	client      *Client
}

type TXWriter struct {
	dev         string
	channels    []string
	sampleBytes int
	client      *Client
}

// ============================================================
// BufferHandle Wrapper
// (Abstracts over binary / text modes via connect.go router)
// ============================================================

type BufferHandle struct {
	ID     int
	Device string
	Chans  []string
	client *Client
}

// ============================================================
// Device & Channel Detector (XML-driven)
// ============================================================

// DetectStreamingDevices dynamically determines:
// - RX device
// - TX device
// - RX/TX channels
// - sample byte width (from scan-element.format)
func (c *Client) DetectStreamingDevices() (rxDev string, txDev string, rxCh []string, txCh []string, sampleBytes int, err error) {

	ctx := c.lastXML // loaded during Dial()

	if ctx == nil {
		return "", "", nil, nil, 0, errors.New("DetectStreamingDevices: no XML context loaded")
	}

	c.debug(1, "DetectStreamingDevices: scanning XML for devices...")

	type candidate struct {
		dev   string
		chans []string
		fmt   string
	}

	var rxOpts []candidate
	var txOpts []candidate

	// scan devices
	for _, d := range ctx.Device {
		devName := d.Name
		devID := d.ID

		var rxC []string
		var txC []string
		var fmtCandidate string

		for _, ch := range d.Channel {
			// input channel → RX
			if ch.Type == "input" {
				rxC = append(rxC, ch.ID)
				if ch.ScanElement.Format != "" {
					fmtCandidate = ch.ScanElement.Format
				}
			}
			// output channel → TX
			if ch.Type == "output" {
				txC = append(txC, ch.ID)
				if ch.ScanElement.Format != "" {
					fmtCandidate = ch.ScanElement.Format
				}
			}
		}

		// RX device candidate
		if len(rxC) > 0 {
			rxOpts = append(rxOpts, candidate{
				dev:   devName,
				chans: rxC,
				fmt:   fmtCandidate,
			})
		}

		// TX device candidate
		if len(txC) > 0 {
			txOpts = append(txOpts, candidate{
				dev:   devName,
				chans: txC,
				fmt:   fmtCandidate,
			})
		}
	}

	if len(rxOpts) == 0 {
		return "", "", nil, nil, 0, errors.New("no RX-capable device found")
	}
	if len(txOpts) == 0 {
		return "", "", nil, nil, 0, errors.New("no TX-capable device found")
	}

	// choose first candidates (simple baseline, optimizations later)
	rxSel := rxOpts[0]
	txSel := txOpts[0]

	// determine sample byte width
	sb, err := c.parseSampleWidth(rxSel.fmt)
	if err != nil || sb == 0 {
		c.debug(1, "DetectStreamingDevices: sample width parse failed, defaulting to 2 bytes")
		sb = 2 // fallback
	}

	c.debug(1, "DetectStreamingDevices: RX dev=%s chans=%v sampleBytes=%d",
		rxSel.dev, rxSel.chans, sb)
	c.debug(1, "DetectStreamingDevices: TX dev=%s chans=%v", txSel.dev, txSel.chans)

	return rxSel.dev, txSel.dev, rxSel.chans, txSel.chans, sb, nil
}

// ============================================================
// Sample Width Parser (extract S12/S16/etc. from scan-element)
// ============================================================

func (c *Client) parseSampleWidth(format string) (int, error) {
	if format == "" {
		return 0, errors.New("empty format string")
	}

	// Examples:
	// "le:S16/16>>0"
	// "be:S12/0>>0"
	// "S16"
	var bits int
	_, err := fmt.Sscanf(format, "%*[^S]S%d", &bits)
	if err != nil {
		return 0, fmt.Errorf("parseSampleWidth failed: %w", err)
	}

	return bits / 8, nil
}
