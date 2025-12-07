package iiod

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// GetContextInfoWithContext queries context info with context support (text protocol only).
func (c *Client) GetContextInfoWithContextText(ctx context.Context) (ContextInfo, error) {
	payload, err := c.sendCommandString(ctx, "VERSION")
	if err != nil {
		return ContextInfo{}, err
	}
	return parseContextInfo(payload)
}

// GetXMLContextWithContext retrieves XML context using the text protocol (PRINT).
func (c *Client) GetXMLContextWithContextText(ctx context.Context) (string, error) {
	// Use cached XML if already available
	if c.xmlContext != "" {
		if c.deviceIndexMap == nil || c.attributeCodes == nil {
			if err := c.refreshMetadataMaps(c.xmlContext); err != nil {
				log.Printf("Failed to parse IIOD metadata maps from cached XML: %v", err)
			}
		}
		return c.xmlContext, nil
	}

	log.Printf("[IIOD DEBUG] GetXMLContext: Sending PRINT command (text)...")
	resp, err := c.sendBinaryCommand(ctx, "PRINT", nil)
	if err != nil {
		return "", err
	}

	// Case 1: sendBinaryCommand saw '<?xml' on the first line and cached XML itself.
	// In that path it returns (nil, nil) and c.xmlContext is already filled.
	if c.xmlContext == "" {
		// Case 2: normal "0 <len>\n<payload>" reply: resp is the XML bytes.
		if len(resp) == 0 {
			return "", fmt.Errorf("no XML context received")
		}
		c.cacheXMLMetadata(string(resp))
	}

	return c.xmlContext, nil
}

// ListDevicesWithContext lists devices using the text protocol.
func (c *Client) ListDevicesWithContextText(ctx context.Context) ([]string, error) {
	payload, err := c.sendCommandString(ctx, "LIST_DEVICES")
	if err != nil || strings.TrimSpace(payload) == "" {
		// Some older servers don't support LIST_DEVICES; fall back to parsing XML.
		return c.ListDevicesFromXML(ctx)
	}

	return strings.Fields(payload), nil
}

// GetChannelsWithContext gets channels using the text protocol (LIST_CHANNELS).
func (c *Client) GetChannelsWithContextText(ctx context.Context, device string) ([]string, error) {
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

// OpenBufferWithContext opens a buffer using the text protocol (OPEN).
func (c *Client) OpenBufferWithContextText(ctx context.Context, device string, samples int) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return fmt.Errorf("sample count must be positive")
	}

	_, err := c.sendCommandString(ctx, fmt.Sprintf("OPEN %s %d", device, samples))
	return err
}

// ReadBufferWithContext reads samples using the text protocol (READBUF).
func (c *Client) ReadBufferWithContextText(ctx context.Context, device string, samples int) ([]byte, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}

	return c.sendBinaryCommand(ctx, fmt.Sprintf("READBUF %s %d", device, samples), nil)
}

// WriteBufferWithContext writes buffer data using the text protocol (WRITEBUF).
func (c *Client) WriteBufferWithContextText(ctx context.Context, device string, data []byte) error {
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

// CloseBufferWithContext closes the buffer using the text protocol (CLOSE).
func (c *Client) CloseBufferWithContextText(ctx context.Context, device string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}

	_, err := c.sendCommandString(ctx, fmt.Sprintf("CLOSE %s", device))
	return err
}
