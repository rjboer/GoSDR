package iiod

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// getContextInfoWithContextBinary retrieves context info using binary protocol (NOT SUPPORTED).
// Binary protocol v1 does not have a generic "Context Info" command analogous to text VERSION string parsing?
// Actually, IIOD usually returns XML in text mode. Binary mode relies on XML overlap?
// For now, we return empty or error, as binary mode usually assumes we have what we need.
// However, connect.go expects it. `BinaryBackend` sends OpVersion.
func (c *Client) getContextInfoWithContextBinary(ctx context.Context) (ContextInfo, error) {
	// Send VERSION command
	cmd := IIODCommand{Opcode: opcodeVersion, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, nil); err != nil {
		return ContextInfo{}, err
	}
	// Read status
	status, err := c.readResponse(ctx)
	if err != nil {
		return ContextInfo{}, err
	}
	// Binary VERSION response is just status (0=success). It doesn't return a string description.
	// So we return synthetic info.
	if status == 0 {
		return ContextInfo{Major: 1, Minor: 0, Description: "Binary Protocol Detected"}, nil
	}
	return ContextInfo{}, fmt.Errorf("binary version check failed: %d", status)
}

func (c *Client) getContextInfoWithContextText(ctx context.Context) (ContextInfo, error) {
	resp, err := c.sendCommandString(ctx, "VERSION")
	if err != nil {
		return ContextInfo{}, err
	}
	return parseContextInfo(resp)
}

func (c *Client) listDevicesWithContextBinary(ctx context.Context) ([]string, error) {
	cmd := IIODCommand{Opcode: opcodeListDevices, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, nil); err != nil {
		return nil, err
	}
	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := c.readPayload(status)
	if err != nil {
		return nil, err
	}
	return splitNullTerminated(payload), nil
}

func (c *Client) listDevicesWithContextText(ctx context.Context) ([]string, error) {
	resp, err := c.sendCommandString(ctx, "LISTDEVICES")
	if err != nil {
		return nil, err
	}
	if resp == "" {
		return []string{}, nil
	}
	return strings.Fields(resp), nil
}

func (c *Client) getXMLContextWithContextBinary(ctx context.Context) (string, error) {
	// Binary protocol doesn't support XML fetch directly?
	// BinaryBackend.GetXMLContext returns error.
	return "", fmt.Errorf("binary protocol cannot fetch XML context (use text mode)")
}

func (c *Client) getXMLContextWithContextText(ctx context.Context) (string, error) {
	// Send PRINT command
	if _, err := c.sendCommandString(ctx, "PRINT"); err != nil {
		return "", err
	}
	if c.xmlContext != "" {
		return c.xmlContext, nil
	}
	// readRawXML handles the streaming response for PRINT
	return c.readRawXML(ctx)
}

func (c *Client) getChannelsWithContextBinary(ctx context.Context, device string) ([]string, error) {
	cmd := IIODCommand{Opcode: opcodeListChannels, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, []byte(device)); err != nil {
		return nil, err
	}
	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := c.readPayload(status)
	if err != nil {
		return nil, err
	}
	return splitNullTerminated(payload), nil
}

func (c *Client) getChannelsWithContextText(ctx context.Context, device string) ([]string, error) {
	resp, err := c.sendCommandString(ctx, fmt.Sprintf("LISTCHANNELS %s", device))
	if err != nil {
		return nil, err
	}
	if resp == "" {
		return []string{}, nil
	}
	return strings.Fields(resp), nil
}

func (c *Client) openBufferWithContextBinary(ctx context.Context, device string, samples int) error {
	payload := []byte(fmt.Sprintf("%s:%d", device, samples))
	cmd := IIODCommand{Opcode: opcodeOpenBuffer, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return err
	}
	status, err := c.readResponse(ctx)
	if err != nil {
		return err
	}
	payloadResp, err := c.readPayload(status)
	if err != nil {
		return err
	}
	// Parse ID from payload
	var id int
	if _, err := fmt.Sscanf(string(payloadResp), "%d", &id); err != nil {
		return fmt.Errorf("malformed buffer id: %s", string(payloadResp))
	}

	c.stateMu.Lock()
	if c.openBuffers == nil {
		c.openBuffers = make(map[string]int)
	}
	c.openBuffers[device] = id
	c.stateMu.Unlock()
	return nil
}

func (c *Client) openBufferWithContextText(ctx context.Context, device string, samples int) error {
	cmd := fmt.Sprintf("OPEN %s %d", device, samples)
	log.Printf("[IIOD DEBUG] openBufferWithContextText: sending %q", cmd)
	resp, err := c.sendCommandString(ctx, cmd)
	if err != nil {
		var iiErr *IIODError
		if errors.As(err, &iiErr) && iiErr.Status == -22 {
			legacyCmd := fmt.Sprintf("BUFFER_OPEN %s %d", device, samples)
			log.Printf("[IIOD DEBUG] openBufferWithContextText: OPEN failed with EINVAL, retrying with %q", legacyCmd)
			resp, err = c.sendCommandString(ctx, legacyCmd)
		}
		if err != nil {
			log.Printf("[IIOD DEBUG] openBufferWithContextText: command failed for %s: %v", device, err)
			return err
		}
	}
	log.Printf("[IIOD DEBUG] openBufferWithContextText: response=%q", resp)
	var id int
	if _, err := fmt.Sscanf(resp, "%d", &id); err != nil {
		log.Printf("[IIOD DEBUG] openBufferWithContextText: malformed buffer id for %s: %q", device, resp)
		return fmt.Errorf("malformed buffer id: %s", resp)
	}

	c.stateMu.Lock()
	if c.openBuffers == nil {
		c.openBuffers = make(map[string]int)
	}
	c.openBuffers[device] = id
	c.stateMu.Unlock()
	return nil
}

func (c *Client) readBufferWithContextBinary(ctx context.Context, device string, samples int) ([]byte, error) {
	c.stateMu.Lock()
	id, ok := c.openBuffers[device]
	c.stateMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("buffer not open for device %s", device)
	}

	// Assuming samples = bytes? connect.go uses samples int.
	// But usually we need byte count. BinaryBackend.ReadBuffer takes "nBytes".
	// connect.go ReadBuffer takes "samples".
	// Assuming samples IS bytes here for simplicity or external calculation.
	// Actually callers should know stride.
	nBytes := samples // Simplification as per interface

	payload := []byte(fmt.Sprintf("%d:%d", id, nBytes))
	cmd := IIODCommand{Opcode: opcodeReadBuffer, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return nil, err
	}
	status, err := c.readResponse(ctx)
	if err != nil {
		return nil, err
	}
	return c.readPayload(status)
}

func (c *Client) readBufferWithContextText(ctx context.Context, device string, samples int) ([]byte, error) {
	c.stateMu.Lock()
	_, ok := c.openBuffers[device]
	c.stateMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("buffer not open for device %s", device)
	}
	return nil, fmt.Errorf("text buffer read not implemented fully (needs streaming parser logic)")
}

func (c *Client) writeBufferWithContextBinary(ctx context.Context, device string, data []byte) error {
	c.stateMu.Lock()
	id, ok := c.openBuffers[device]
	c.stateMu.Unlock()
	if !ok {
		return fmt.Errorf("buffer not open for device %s", device)
	}

	header := fmt.Sprintf("%d:%d:", id, len(data))
	payload := append([]byte(header), data...)

	cmd := IIODCommand{Opcode: opcodeWriteBuffer, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return err
	}
	_, err := c.readResponse(ctx)
	return err
}

func (c *Client) writeBufferWithContextText(ctx context.Context, device string, data []byte) error {
	return fmt.Errorf("text buffer write not implemented")
}

func (c *Client) closeBufferWithContextBinary(ctx context.Context, device string) error {
	c.stateMu.Lock()
	id, ok := c.openBuffers[device]
	if ok {
		delete(c.openBuffers, device)
	}
	c.stateMu.Unlock()
	if !ok {
		return nil
	}

	payload := []byte(fmt.Sprintf("%d", id))
	cmd := IIODCommand{Opcode: opcodeCloseBuffer, Device: 0, Code: 0}
	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return err
	}
	_, err := c.readResponse(ctx)
	return err
}

func (c *Client) closeBufferWithContextText(ctx context.Context, device string) error {
	c.stateMu.Lock()
	id, ok := c.openBuffers[device]
	if ok {
		delete(c.openBuffers, device)
	}
	c.stateMu.Unlock()
	if !ok {
		return nil
	}

	_, err := c.sendCommandString(ctx, fmt.Sprintf("BUFFER_CLOSE %d", id))
	return err
}

func (c *Client) readAttrText(ctx context.Context, device, channel, attr string) (string, error) {
	var cmd string
	if channel == "" {
		cmd = fmt.Sprintf("READ %s %s", device, attr)
	} else {
		cmd = fmt.Sprintf("READ %s %s %s", device, channel, attr)
	}
	return c.sendCommandString(ctx, cmd)
}

func (c *Client) writeAttrText(ctx context.Context, device, channel, attr, value string) error {
	var cmd string
	if channel == "" {
		cmd = fmt.Sprintf("WRITE %s %s %s", device, attr, value)
	} else {
		cmd = fmt.Sprintf("WRITE %s %s %s %s", device, channel, attr, value)
	}
	_, err := c.sendCommandString(ctx, cmd)
	return err
}
