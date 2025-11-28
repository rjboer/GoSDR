package iiod

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Client implements a small subset of the IIOD TCP protocol used by libiio.
//
// Typical usage:
//
//	client, err := iiod.Dial("192.168.2.1:30431")
//	if err != nil {
//	        // handle error
//	}
//	info, _ := client.GetContextInfo()
//	devices, _ := client.ListDevices()
//	channels, _ := client.GetChannels(devices[0])
//	_, _ = client.CreateBuffer(devices[0], 1024)
//	_ = client.WriteAttr(devices[0], "", "sampling_frequency", "1000000")
//	_, _ = client.ReadAttr(devices[0], "", "sampling_frequency")
//
// The methods build protocol-compliant command strings and rely on the shared
// send helper to validate responses.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

// Send issues a raw IIOD command and returns the response payload.
//
// This is a thin wrapper around the internal send helper for callers that need
// direct access to lower-level protocol commands.
func (c *Client) Send(cmd string) (string, error) {
	return c.send(cmd)
}

// Close terminates the underlying network connection.
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("client is not connected")
	}

	return c.conn.Close()
}

// ContextInfo describes the remote IIOD context reported by the server.
type ContextInfo struct {
	Major       int
	Minor       int
	Description string
}

// Dial opens a TCP connection to an IIOD server.
func Dial(addr string) (*Client, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	c, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   c,
		reader: bufio.NewReader(c),
	}, nil
}

// GetContextInfo queries the remote IIOD context version and description.
func (c *Client) GetContextInfo() (ContextInfo, error) {
	payload, err := c.send("VERSION")
	if err != nil {
		return ContextInfo{}, err
	}

	parts := strings.Fields(payload)
	if len(parts) < 2 {
		return ContextInfo{}, fmt.Errorf("unexpected context info: %q", payload)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ContextInfo{}, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return ContextInfo{}, fmt.Errorf("invalid minor version: %w", err)
	}

	description := ""
	if len(parts) > 2 {
		description = strings.Join(parts[2:], " ")
	}

	return ContextInfo{Major: major, Minor: minor, Description: description}, nil
}

// ListDevices returns the set of device identifiers known by the server.
func (c *Client) ListDevices() ([]string, error) {
	payload, err := c.send("LIST_DEVICES")
	if err != nil {
		return nil, err
	}
	if payload == "" {
		return nil, nil
	}
	return strings.Fields(payload), nil
}

// GetChannels returns channel names for a given device.
func (c *Client) GetChannels(device string) ([]string, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}

	payload, err := c.send(fmt.Sprintf("LIST_CHANNELS %s", device))
	if err != nil {
		return nil, err
	}
	if payload == "" {
		return nil, nil
	}
	return strings.Fields(payload), nil
}

// CreateBuffer allocates a remote buffer for the given device and sample count.
func (c *Client) CreateBuffer(device string, samples int) (string, error) {
	if strings.TrimSpace(device) == "" {
		return "", fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return "", fmt.Errorf("sample count must be positive")
	}

	return c.send(fmt.Sprintf("CREATE_BUFFER %s %d", device, samples))
}

// ReadAttr reads a device or channel attribute. An empty channel targets a
// device attribute; otherwise the attribute is read from the named channel.
func (c *Client) ReadAttr(device, channel, attr string) (string, error) {
	if strings.TrimSpace(device) == "" {
		return "", fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(attr) == "" {
		return "", fmt.Errorf("attribute name is required")
	}

	target := fmt.Sprintf("%s %s", device, attr)
	if channel != "" {
		target = fmt.Sprintf("%s %s %s", device, channel, attr)
	}

	return c.send(fmt.Sprintf("READ_ATTR %s", target))
}

// WriteAttr writes a device or channel attribute value. An empty channel targets
// a device attribute; otherwise the attribute is written to the named channel.
func (c *Client) WriteAttr(device, channel, attr, value string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(attr) == "" {
		return fmt.Errorf("attribute name is required")
	}

	target := fmt.Sprintf("%s %s %s", device, attr, value)
	if channel != "" {
		target = fmt.Sprintf("%s %s %s %s", device, channel, attr, value)
	}

	_, err := c.send(fmt.Sprintf("WRITE_ATTR %s", target))
	return err
}

func (c *Client) send(cmd string) (string, error) {
	if c == nil || c.conn == nil || c.reader == nil {
		return "", fmt.Errorf("client is not connected")
	}
	if strings.TrimSpace(cmd) == "" {
		return "", fmt.Errorf("command is required")
	}

	if _, err := fmt.Fprintf(c.conn, "%s\n", cmd); err != nil {
		return "", err
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed reply header: %q", line)
	}

	status, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid status code: %w", err)
	}
	length, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid payload length: %w", err)
	}
	if length < 0 {
		return "", fmt.Errorf("negative payload length: %d", length)
	}

	var payload string
	if length > 0 {
		buf := make([]byte, length)
		if _, err := io.ReadFull(c.reader, buf); err != nil {
			return "", err
		}
		payload = string(buf)
	}

	if status != 0 {
		payload = strings.TrimSpace(payload)
		if payload != "" {
			return "", fmt.Errorf("iiod error %d: %s", status, payload)
		}
		return "", fmt.Errorf("iiod error %d", status)
	}

	return strings.TrimSpace(payload), nil
}
