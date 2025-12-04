package iiod

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
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

	mu           sync.Mutex
	stateMu      sync.Mutex
	openBuffers  map[string]int
	timeout      time.Duration
	healthWindow time.Duration
	lastPing     time.Time
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

	err := c.conn.Close()
	c.conn = nil
	c.reader = nil
	if err != nil {
		return err
	}

	return nil
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
		conn:         c,
		reader:       bufio.NewReader(c),
		openBuffers:  make(map[string]int),
		timeout:      5 * time.Second,
		healthWindow: 10 * time.Second,
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

// OpenBuffer issues the OPEN command to allocate a streaming buffer for the
// given device and sample count.
func (c *Client) OpenBuffer(device string, samples int) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return fmt.Errorf("sample count must be positive")
	}
	if err := c.ensureHealthy(); err != nil {
		return err
	}

	c.stateMu.Lock()
	if _, exists := c.openBuffers[device]; exists {
		c.stateMu.Unlock()
		return fmt.Errorf("buffer already open for device %s", device)
	}
	c.stateMu.Unlock()

	// OPEN replies use the standard binary header. We only care about the
	// status code; any payload is ignored.
	if _, err := c.sendBinary(fmt.Sprintf("OPEN %s %d", device, samples), nil); err != nil {
		return err
	}

	c.stateMu.Lock()
	c.openBuffers[device] = samples
	c.stateMu.Unlock()
	return nil
}

// ReadBuffer requests binary sample data from the remote buffer.
func (c *Client) ReadBuffer(device string, samples int) ([]byte, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}
	if err := c.ensureHealthy(); err != nil {
		return nil, err
	}

	c.stateMu.Lock()
	bufSamples, ok := c.openBuffers[device]
	c.stateMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("buffer for device %s is not open", device)
	}
	if bufSamples != samples {
		return nil, fmt.Errorf("requested samples %d do not match open buffer size %d", samples, bufSamples)
	}

	// Parse the binary reply to ensure the status code is checked and the
	// exact payload length is read.
	data, err := c.sendBinary(fmt.Sprintf("READBUF %s %d", device, samples), nil)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// WriteBuffer writes binary IQ data to the remote buffer.
func (c *Client) WriteBuffer(device string, data []byte) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data provided for buffer write")
	}
	if err := c.ensureHealthy(); err != nil {
		return err
	}

	c.stateMu.Lock()
	_, ok := c.openBuffers[device]
	c.stateMu.Unlock()
	if !ok {
		return fmt.Errorf("buffer for device %s is not open", device)
	}

	cmd := fmt.Sprintf("WRITEBUF %s %d", device, len(data))
	// The response uses the same binary header format as other commands;
	// we ignore any payload and rely on the status code for success.
	if _, err := c.sendBinary(cmd, data); err != nil {
		return err
	}

	return nil
}

// CloseBuffer tears down the remote buffer.
func (c *Client) CloseBuffer(device string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if err := c.ensureHealthy(); err != nil {
		return err
	}

	c.stateMu.Lock()
	if _, ok := c.openBuffers[device]; !ok {
		c.stateMu.Unlock()
		return fmt.Errorf("buffer for device %s is not open", device)
	}
	c.stateMu.Unlock()

	if _, err := c.sendBinary(fmt.Sprintf("CLOSE %s", device), nil); err != nil {
		return err
	}

	c.stateMu.Lock()
	delete(c.openBuffers, device)
	c.stateMu.Unlock()
	return nil
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
	resp, err := c.sendBinary(cmd, nil)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}

func (c *Client) sendBinary(cmd string, payload []byte) ([]byte, error) {
	if c == nil || c.conn == nil || c.reader == nil {
		return nil, fmt.Errorf("client is not connected")
	}
	if strings.TrimSpace(cmd) == "" {
		return nil, fmt.Errorf("command is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.timeout > 0 {
		_ = c.conn.SetDeadline(time.Now().Add(c.timeout))
		defer c.conn.SetDeadline(time.Time{})
	}

	if _, err := fmt.Fprintf(c.conn, "%s\n", cmd); err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return nil, err
		}
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed reply header: %q", line)
	}

	status, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid status code: %w", err)
	}
	length, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid payload length: %w", err)
	}
	if length < 0 {
		return nil, fmt.Errorf("negative payload length: %d", length)
	}

	var resp []byte
	if length > 0 {
		resp = make([]byte, length)
		if _, err := io.ReadFull(c.reader, resp); err != nil {
			return nil, err
		}
	}

	if status != 0 {
		msg := strings.TrimSpace(string(resp))
		if msg != "" {
			return nil, fmt.Errorf("iiod error %d: %s", status, msg)
		}
		return nil, fmt.Errorf("iiod error %d", status)
	}

	c.stateMu.Lock()
	c.lastPing = time.Now()
	c.stateMu.Unlock()

	return resp, nil
}

// ensureHealthy pings the server when the connection has been idle for longer
// than the configured health window.
func (c *Client) ensureHealthy() error {
	if c == nil || c.conn == nil || c.reader == nil {
		return fmt.Errorf("client is not connected")
	}

	c.stateMu.Lock()
	needPing := c.lastPing.IsZero() || time.Since(c.lastPing) > c.healthWindow
	c.stateMu.Unlock()

	if !needPing {
		return nil
	}

	return c.Ping()
}

// Ping issues a lightweight request to verify that the IIOD session is still
// responsive.
func (c *Client) Ping() error {
	if c == nil || c.conn == nil || c.reader == nil {
		return fmt.Errorf("client is not connected")
	}

	if _, err := c.GetContextInfo(); err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}

	c.stateMu.Lock()
	c.lastPing = time.Now()
	c.stateMu.Unlock()

	return nil
}
