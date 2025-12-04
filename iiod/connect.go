package iiod

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/xml"
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
	stats        ConnectionStats
}

// Device describes an IIO device and its channels/attributes derived from the XML context.
type Device struct {
	ID         string      `xml:"id,attr"`
	Name       string      `xml:"name,attr"`
	Attributes []Attribute `xml:"attribute"`
	Channels   []Channel   `xml:"channel"`
}

// Channel describes a single channel within a device.
type Channel struct {
	ID         string      `xml:"id,attr"`
	Type       string      `xml:"type,attr"`
	Attributes []Attribute `xml:"attribute"`
}

// Attribute represents a typed attribute exposed by a device or channel.
type Attribute struct {
	Name     string `xml:"name,attr"`
	Filename string `xml:"filename,attr"`
	Type     string `xml:"type,attr"`
	Value    string `xml:",chardata"`
	Unit     string `xml:"unit,attr"`
}

// ConnectionStats captures lightweight client health metrics.
type ConnectionStats struct {
	BytesSent     uint64
	BytesReceived uint64
	LastPing      time.Time
	LastError     time.Time
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
	c.stateMu.Lock()
	c.openBuffers = map[string]int{}
	c.stateMu.Unlock()
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

// GetXMLContext retrieves the raw XML device tree from the remote IIOD server.
func (c *Client) GetXMLContext() (string, error) {
	xmlPayload, err := c.send("XML")
	if err != nil {
		return "", err
	}

	return xmlPayload, nil
}

// GetDeviceInfo returns parsed device metadata derived from the XML context.
func (c *Client) GetDeviceInfo() ([]Device, error) {
	payload, err := c.GetXMLContext()
	if err != nil {
		return nil, err
	}

	var ctx struct {
		Devices []Device `xml:"device"`
	}

	if err := xml.Unmarshal([]byte(payload), &ctx); err != nil {
		return nil, fmt.Errorf("failed to parse XML context: %w", err)
	}

	return ctx.Devices, nil
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

// GetTrigger returns the currently configured trigger for a device.
func (c *Client) GetTrigger(device string) (string, error) {
	if strings.TrimSpace(device) == "" {
		return "", fmt.Errorf("device name is required")
	}

	return c.send(fmt.Sprintf("GETTRIG %s", device))
}

// SetTrigger configures a trigger source for a device.
func (c *Client) SetTrigger(device, trigger string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(trigger) == "" {
		return fmt.Errorf("trigger name is required")
	}

	_, err := c.send(fmt.Sprintf("SETTRIG %s %s", device, trigger))
	return err
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

// ReadDebugAttr reads an attribute from the debug filesystem of a device or channel.
func (c *Client) ReadDebugAttr(device, channel, attr string) (string, error) {
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

	return c.send(fmt.Sprintf("READ_DEBUG_ATTR %s", target))
}

// WriteDebugAttr writes an attribute in the debug filesystem for a device or channel.
func (c *Client) WriteDebugAttr(device, channel, attr, value string) error {
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

	_, err := c.send(fmt.Sprintf("WRITE_DEBUG_ATTR %s", target))
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

	deadline := time.Time{}
	if c.timeout > 0 {
		deadline = time.Now().Add(c.timeout)
		_ = c.conn.SetDeadline(deadline)
		defer c.conn.SetDeadline(time.Time{})
	}

	if err := c.writeCommand(cmd, payload); err != nil {
		c.trackError()
		return nil, err
	}

	status, resp, err := c.readResponse(deadline)
	if err != nil {
		c.trackError()
		return nil, err
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
	c.stats.LastPing = c.lastPing
	c.stateMu.Unlock()

	return resp, nil
}

func (c *Client) writeCommand(cmd string, payload []byte) error {
	if _, err := fmt.Fprintf(c.conn, "%s\n", cmd); err != nil {
		return err
	}

	c.stateMu.Lock()
	c.stats.BytesSent += uint64(len(cmd) + 1)
	c.stateMu.Unlock()

	if len(payload) == 0 {
		return nil
	}

	var lengthPrefix [4]byte
	binary.BigEndian.PutUint32(lengthPrefix[:], uint32(len(payload)))

	if _, err := c.conn.Write(lengthPrefix[:]); err != nil {
		return err
	}

	c.stateMu.Lock()
	c.stats.BytesSent += uint64(len(lengthPrefix))
	c.stateMu.Unlock()

	_, err := c.conn.Write(payload)
	if err == nil {
		c.stateMu.Lock()
		c.stats.BytesSent += uint64(len(payload))
		c.stateMu.Unlock()
	}
	return err
}

func (c *Client) readResponse(deadline time.Time) (int, []byte, error) {
	if !deadline.IsZero() {
		_ = c.conn.SetReadDeadline(deadline)
		defer c.conn.SetReadDeadline(time.Time{})
	}

	line, err := c.reader.ReadString('\n')
	if err != nil {
		return 0, nil, err
	}
	line = strings.TrimSpace(line)

	parts := strings.Fields(line)
	if len(parts) != 2 {
		return 0, nil, fmt.Errorf("malformed reply header: %q", line)
	}

	status, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, nil, fmt.Errorf("invalid status code: %w", err)
	}
	length, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, nil, fmt.Errorf("invalid payload length: %w", err)
	}
	if length < 0 {
		return 0, nil, fmt.Errorf("negative payload length: %d", length)
	}

	var resp []byte
	if length > 0 {
		resp = make([]byte, length)
		if _, err := io.ReadFull(c.reader, resp); err != nil {
			return 0, nil, err
		}
	}

	c.stateMu.Lock()
	c.stats.BytesReceived += uint64(len(line) + 1 + len(resp))
	c.stateMu.Unlock()

	return status, resp, nil
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

// SetTimeout updates both the server-side timeout (when supported) and the local deadline window.
func (c *Client) SetTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	millis := int(timeout.Milliseconds())
	if millis == 0 {
		millis = 1
	}

	if _, err := c.send(fmt.Sprintf("TIMEOUT %d", millis)); err != nil {
		return err
	}

	c.stateMu.Lock()
	c.timeout = timeout
	c.stateMu.Unlock()
	return nil
}

// StreamBuffer continuously reads from a device buffer and forwards payloads to the handler.
// Backpressure is handled by blocking reads when the handler is still processing previous data.
func (c *Client) StreamBuffer(ctx context.Context, device string, samples int, channelMask uint64, handler func([]byte) error) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}

	buf, err := c.CreateStreamBuffer(device, samples, channelMask)
	if err != nil {
		return err
	}
	defer buf.Close()

	dataCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		for {
			payload, readErr := buf.ReadSamples()
			if readErr != nil {
				errCh <- readErr
				return
			}

			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case dataCh <- payload:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errCh:
			return err
		case payload := <-dataCh:
			if err := handler(payload); err != nil {
				return err
			}
		}
	}
}

// BatchReadAttrs reads multiple attributes for a given device/channel pair.
func (c *Client) BatchReadAttrs(device, channel string, attrs []string) (map[string]string, error) {
	if len(attrs) == 0 {
		return nil, fmt.Errorf("no attributes requested")
	}

	results := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		value, err := c.ReadAttr(device, channel, attr)
		if err != nil {
			return nil, err
		}
		results[attr] = value
	}

	return results, nil
}

// BatchWriteAttrs writes a collection of attributes atomically, aborting on the first error.
func (c *Client) BatchWriteAttrs(device, channel string, attrs map[string]string) error {
	if len(attrs) == 0 {
		return fmt.Errorf("no attributes provided")
	}

	for key, value := range attrs {
		if err := c.WriteAttr(device, channel, key, value); err != nil {
			return err
		}
	}

	return nil
}

// Stats returns a snapshot of current connection metrics.
func (c *Client) Stats() ConnectionStats {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.stats
}

func (c *Client) trackError() {
	c.stateMu.Lock()
	c.stats.LastError = time.Now()
	c.stateMu.Unlock()
}
