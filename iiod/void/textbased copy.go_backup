package iiod

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
)

// Text protocol implementations used by connect.go when c.mode == ProtocolText.

// VERSION (text)
func (c *Client) getContextInfoWithContextText(ctx context.Context) (ContextInfo, error) {
	payload, err := c.sendCommandString(ctx, "VERSION")
	if err != nil {
		return ContextInfo{}, err
	}
	return parseContextInfo(payload)
}

// PRINT → XML (text)
func (c *Client) getXMLContextWithContextText(ctx context.Context) (string, error) {
	// Use cached XML if already available
	if c.xmlContext != "" {
		if c.deviceIndexMap == nil || c.attributeCodes == nil {
			if err := c.refreshMetadataMaps(c.xmlContext); err != nil {
				log.Printf("Failed to parse IIOD metadata maps from cached XML: %v", err)
			}
		}
		return c.xmlContext, nil
	}

	log.Printf("[IIOD DEBUG] getXMLContextWithContextText: sending PRINT (text)...")
	// We still use sendBinaryCommand for the length-prefixed reply, but the command is text.
	resp, err := c.sendBinaryCommand(ctx, "PRINT", nil)
	if err != nil {
		return "", err
	}

	// If sendBinaryCommand saw a legacy “raw XML” stream and cached it itself,
	// it may have set c.xmlContext and returned (nil, nil).
	if c.xmlContext == "" {
		if len(resp) == 0 {
			return "", fmt.Errorf("no XML context received")
		}
		c.cacheXMLMetadata(string(resp))
	}

	return c.xmlContext, nil
}

// LIST_DEVICES → use XML context for real IDs
func (c *Client) listDevicesWithContextText(ctx context.Context) ([]string, error) {
	// Pluto's text LIST_DEVICES returns "iio:device0 ..." which isn't useful for AD9361 discovery.
	// Instead, always parse device IDs from the XML context.
	return c.ListDevicesFromXML(ctx)
}

// LIST_CHANNELS <device> (text)
func (c *Client) getChannelsWithContextText(ctx context.Context, device string) ([]string, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}

	payload, err := c.sendCommandString(ctx, fmt.Sprintf("LIST_CHANNELS %s", device))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}

	return strings.Fields(payload), nil
}

// OPEN <device> <samples> (text)
func (c *Client) openBufferWithContextText(ctx context.Context, device string, samples int) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return fmt.Errorf("sample count must be positive")
	}

	_, err := c.sendCommandString(ctx, fmt.Sprintf("OPEN %s %d", device, samples))
	return err
}

// READBUF <device> <samples> (text)
func (c *Client) readBufferWithContextText(ctx context.Context, device string, samples int) ([]byte, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}

	return c.sendBinaryCommand(ctx, fmt.Sprintf("READBUF %s %d", device, samples), nil)
}

// WRITEBUF <device> <len> (text) + binary payload
func (c *Client) writeBufferWithContextText(ctx context.Context, device string, data []byte) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data provided for buffer write")
	}

	cmd := fmt.Sprintf("WRITEBUF %s %d", device, len(data))
	_, err := c.sendBinaryCommand(ctx, cmd, data)
	return err
}

// CLOSE <device> (text)
func (c *Client) closeBufferWithContextText(ctx context.Context, device string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}

	_, err := c.sendCommandString(ctx, fmt.Sprintf("CLOSE %s", device))
	return err
}

// readAttrText issues a text-based "READ" command.
func (c *Client) readAttrText(ctx context.Context, device, channel, attr string) (string, error) {
	cmd := "READ"
	target := device
	if channel != "" {
		target = fmt.Sprintf("%s %s", target, channel)
	}
	target = fmt.Sprintf("%s %s", target, attr)

	val, err := c.sendCommandString(ctx, fmt.Sprintf("%s %s", cmd, target))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(val), nil
}

// writeAttrText issues a text-based "WRITE" command.
func (c *Client) writeAttrText(ctx context.Context, device, channel, attr, value string) error {
	cmd := "WRITE"
	target := device
	if channel != "" {
		target = fmt.Sprintf("%s %s", target, channel)
	}
	target = fmt.Sprintf("%s %s", target, attr)

	_, err := c.sendCommandString(ctx, fmt.Sprintf("%s %s %s", cmd, target, value))
	return err

}

// -----------------------------------------------------------------------------
//  ATTRIBUTE COMPAT LAYER (Binary → Text Fallback)
// -----------------------------------------------------------------------------
//
// These functions provide safe compatibility for legacy IIOD servers like
// Analog Devices PlutoSDR, which return XML on unsupported binary opcodes.
//
// The logic is:
//    1. Try binary ReadAttr/WriteAttr if binary mode is active
//    2. If binary mode fails or returns malformed header → fallback to text
//    3. Perform "READ <dev> <chan> <attr>" or "WRITE <dev> <chan> <attr> <value>"
// -----------------------------------------------------------------------------

// ReadAttrCompat tries binary mode first, then falls back to text mode.
func (c *Client) ReadAttrCompat(ctx context.Context, device, channel, attr string) (string, error) {
	if c.mode == ProtocolBinary {
		val, err := c.ReadAttr(device, channel, attr)
		if err == nil {
			return val, nil
		}

		// If we get XML or any unexpected data, binary mode is unsupported.
		if isLikelyXML(err) {
			log.Printf("[IIOD DEBUG] binary ReadAttr returned XML-like data, falling back to text")
		} else {
			log.Printf("[IIOD DEBUG] binary ReadAttr failed (%v), falling back to text", err)
		}
	}

	// -- TEXT MODE FALLBACK --
	cmd := ""
	if channel == "" {
		cmd = fmt.Sprintf("READ %s %s", device, attr)
	} else {
		cmd = fmt.Sprintf("READ %s %s %s", device, channel, attr)
	}

	resp, err := c.sendCommandString(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("text READ failed: %w", err)
	}
	return resp, nil
}

// WriteAttrCompat tries binary mode first, then falls back to text mode.
func (c *Client) WriteAttrCompat(ctx context.Context, device, channel, attr, value string) error {
	if c.mode == ProtocolBinary {
		err := c.WriteAttr(device, channel, attr, value)
		if err == nil {
			return nil
		}

		// If binary produced XML, switch to text immediately.
		if isLikelyXML(err) {
			log.Printf("[IIOD DEBUG] binary WriteAttr returned XML-like reply, falling back to text")
		} else {
			log.Printf("[IIOD DEBUG] binary WriteAttr failed (%v), falling back to text", err)
		}
	}

	// -- TEXT MODE FALLBACK --
	var cmd string
	if channel == "" {
		cmd = fmt.Sprintf("WRITE %s %s %s", device, attr, value)
	} else {
		cmd = fmt.Sprintf("WRITE %s %s %s %s", device, channel, attr, value)
	}

	_, err := c.sendCommandString(ctx, cmd)
	if err != nil {
		return fmt.Errorf("text WRITE failed: %w", err)
	}
	return nil
}

// Helper that detects XML returned instead of binary header
func isLikelyXML(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "<?xml") ||
		strings.Contains(msg, "<context") ||
		strings.Contains(msg, "DOCTYPE") ||
		strings.Contains(msg, "<device")
}

// / DumpRawXML retrieves the full XML context using the IIOD text protocol.
// This works on PlutoSDR (IIOD v0.25–v0.38).
func (c *Client) DumpRawXML() (string, error) {
	if c.conn == nil {
		return "", fmt.Errorf("text-based IIOD connection is not initialized")
	}

	// Send PRINT\n to request the XML dump
	if _, err := c.conn.Write([]byte("PRINT\n")); err != nil {
		return "", fmt.Errorf("failed to send PRINT command: %w", err)
	}

	// Read until we have the closing </context>
	var buf bytes.Buffer
	tmp := make([]byte, 4096)

	for {
		n, err := c.conn.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])

			// Stop once closing tag is seen
			if bytes.Contains(buf.Bytes(), []byte("</context>")) {
				break
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error reading XML: %w", err)
		}
	}

	return buf.String(), nil
}
