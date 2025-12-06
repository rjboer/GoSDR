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
	"sync/atomic"
	"time"
)

// ProtocolVersion describes the IIOD protocol major/minor version pair.
// It provides comparison helpers for feature detection.
type ProtocolVersion struct {
	Major int
	Minor int
}

// Compare returns -1 if v < other, 0 if equal, and 1 if v > other.
func (v ProtocolVersion) Compare(other ProtocolVersion) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}
		return 1
	}
	if v.Minor < other.Minor {
		return -1
	}
	if v.Minor > other.Minor {
		return 1
	}
	return 0
}

// AtLeast reports whether the version is greater than or equal to the provided version.
func (v ProtocolVersion) AtLeast(other ProtocolVersion) bool {
	return v.Compare(other) >= 0
}

// IsZero reports whether the version is unset.
func (v ProtocolVersion) IsZero() bool {
	return v.Major == 0 && v.Minor == 0
}

func (v ProtocolVersion) String() string {
	if v.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// Client implements the IIOD TCP protocol with enhanced reliability and performance features.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader

	mu           sync.Mutex
	metrics      ClientMetrics
	reconnectCfg *ReconnectConfig
	addr         string
	isConnected  atomic.Bool
	xmlContext   string // Cached XML context from server
	protoVersion ProtocolVersion
}

// ClientMetrics tracks IIO client performance and health.
type ClientMetrics struct {
	BytesSent       atomic.Uint64
	BytesReceived   atomic.Uint64
	CommandsSent    atomic.Uint64
	CommandsFailed  atomic.Uint64
	LastCommandTime atomic.Value // time.Time
	ConnectedAt     time.Time
	ReconnectCount  atomic.Uint32
}

// ReconnectConfig configures automatic reconnection behavior.
type ReconnectConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	OnReconnect  func(*Client) error // Called after successful reconnect to restore state
}

// Send issues a raw IIOD command and returns the response payload.
func (c *Client) Send(cmd string) (string, error) {
	return c.SendWithContext(context.Background(), cmd)
}

// SendWithContext issues a raw IIOD command with context support.
func (c *Client) SendWithContext(ctx context.Context, cmd string) (string, error) {
	resp, err := c.sendBinaryWithContext(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(resp)), nil
}

// Close terminates the underlying network connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return fmt.Errorf("client is not connected")
	}

	err := c.conn.Close()
	c.conn = nil
	c.reader = nil
	c.isConnected.Store(false)
	return err
}

// GetMetrics returns a snapshot of client metrics.
func (c *Client) GetMetrics() ClientMetrics {
	return c.metrics
}

// ContextInfo describes the remote IIOD context reported by the server.
type ContextInfo struct {
	Major       int
	Minor       int
	Description string
}

// Dial opens a TCP connection to an IIOD server.
func Dial(addr string) (*Client, error) {
	return DialWithContext(context.Background(), addr, nil)
}

// DialWithContext opens a TCP connection with context and optional reconnect config.
func DialWithContext(ctx context.Context, addr string, reconnectCfg *ReconnectConfig) (*Client, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	client := &Client{
		conn:         conn,
		reader:       bufio.NewReader(conn),
		addr:         addr,
		reconnectCfg: reconnectCfg,
	}
	client.isConnected.Store(true)
	client.metrics.ConnectedAt = time.Now()

	return client, nil
}

// reconnect attempts to re-establish connection with exponential backoff.
func (c *Client) reconnect(ctx context.Context) error {
	if c.reconnectCfg == nil {
		return fmt.Errorf("reconnect not configured")
	}

	delay := c.reconnectCfg.InitialDelay
	if delay == 0 {
		delay = 100 * time.Millisecond
	}

	maxRetries := c.reconnectCfg.MaxRetries
	if maxRetries == 0 {
		maxRetries = 5
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		dialer := net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", c.addr)
		if err == nil {
			c.mu.Lock()
			c.conn = conn
			c.reader = bufio.NewReader(conn)
			c.isConnected.Store(true)
			c.metrics.ReconnectCount.Add(1)
			c.mu.Unlock()

			// Call user callback to restore hardware state
			if c.reconnectCfg.OnReconnect != nil {
				if err := c.reconnectCfg.OnReconnect(c); err != nil {
					_ = c.Close()
					return fmt.Errorf("reconnect callback failed: %w", err)
				}
			}

			return nil
		}

		// Exponential backoff with jitter
		delay *= 2
		if c.reconnectCfg.MaxDelay > 0 && delay > c.reconnectCfg.MaxDelay {
			delay = c.reconnectCfg.MaxDelay
		}
	}

	return fmt.Errorf("reconnect failed after %d attempts", maxRetries)
}

// GetContextInfo queries the remote IIOD context version and description.
func (c *Client) GetContextInfo() (ContextInfo, error) {
	return c.GetContextInfoWithContext(context.Background())
}

// GetContextInfoWithContext queries context info with context support.
func (c *Client) GetContextInfoWithContext(ctx context.Context) (ContextInfo, error) {
	payload, err := c.SendWithContext(ctx, "VERSION")
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

	info := ContextInfo{Major: major, Minor: minor, Description: description}
	c.cacheProtocolVersion(ProtocolVersion{Major: major, Minor: minor})
	return info, nil
}

func (c *Client) cacheProtocolVersion(version ProtocolVersion) {
	if version.IsZero() {
		return
	}

	c.mu.Lock()
	if c.protoVersion.IsZero() {
		c.protoVersion = version
	}
	c.mu.Unlock()
}

// DetectProtocolVersion attempts to populate and return the negotiated protocol version.
// It prefers extracting attributes from the XML context and falls back to the VERSION command.
func (c *Client) DetectProtocolVersion(ctx context.Context) (ProtocolVersion, error) {
	c.mu.Lock()
	if !c.protoVersion.IsZero() {
		v := c.protoVersion
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	if xml, err := c.GetXMLContextWithContext(ctx); err == nil {
		if version := parseProtocolVersionFromXML(xml); !version.IsZero() {
			c.cacheProtocolVersion(version)
			return version, nil
		}
	}

	info, err := c.GetContextInfoWithContext(ctx)
	if err != nil {
		return ProtocolVersion{}, err
	}

	version := ProtocolVersion{Major: info.Major, Minor: info.Minor}
	c.cacheProtocolVersion(version)
	return version, nil
}

// SupportsWriteCommand reports whether the IIOD server exposes the WRITE command.
// WRITEBUF support arrived alongside protocol refinements; require at least v1.1.
func (c *Client) SupportsWriteCommand(ctx context.Context) (bool, error) {
	version, err := c.DetectProtocolVersion(ctx)
	if err != nil {
		return false, err
	}
	return version.AtLeast(ProtocolVersion{Major: 1, Minor: 1}), nil
}

// SupportsStreamingBuffers reports whether the server can open persistent streaming buffers.
// This is available on protocol versions 1.3+.
func (c *Client) SupportsStreamingBuffers(ctx context.Context) (bool, error) {
	version, err := c.DetectProtocolVersion(ctx)
	if err != nil {
		return false, err
	}
	return version.AtLeast(ProtocolVersion{Major: 1, Minor: 3}), nil
}

// ListDevices returns the set of device identifiers known by the server.
func (c *Client) ListDevices() ([]string, error) {
	return c.ListDevicesWithContext(context.Background())
}

// ListDevicesWithContext lists devices with context support.
func (c *Client) ListDevicesWithContext(ctx context.Context) ([]string, error) {
	payload, err := c.SendWithContext(ctx, "LIST_DEVICES")
	if err != nil {
		// Fallback to XML parsing for older IIOD versions
		return c.ListDevicesFromXML(ctx)
	}
	if payload == "" {
		return nil, nil
	}
	return strings.Fields(payload), nil
}

// GetXMLContext retrieves the full XML context description from the IIOD server.
func (c *Client) GetXMLContext() (string, error) {
	return c.GetXMLContextWithContext(context.Background())
}

// GetXMLContextWithContext retrieves XML context with context support.
func (c *Client) GetXMLContextWithContext(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.xmlContext != "" {
		xml := c.xmlContext
		c.mu.Unlock()
		return xml, nil
	}
	c.mu.Unlock()

	payload, err := c.SendWithContext(ctx, "PRINT")
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	xml := c.xmlContext
	if xml == "" {
		xml = payload
		c.xmlContext = payload
	}
	if xmlVersion := parseProtocolVersionFromXML(xml); !xmlVersion.IsZero() {
		c.protoVersion = xmlVersion
	}
	c.mu.Unlock()

	if strings.TrimSpace(xml) == "" {
		return "", fmt.Errorf("empty XML context")
	}

	return xml, nil
}

// ListDevicesFromXML parses device names from the XML context.
// This is a fallback for older IIOD versions that don't support LIST_DEVICES.
func (c *Client) ListDevicesFromXML(ctx context.Context) ([]string, error) {
	xml, err := c.GetXMLContextWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get XML context: %w", err)
	}

	if xmlVersion := parseProtocolVersionFromXML(xml); !xmlVersion.IsZero() {
		c.cacheProtocolVersion(xmlVersion)
	}

	// Simple XML parsing to extract device IDs
	devices := []string{}
	lines := strings.Split(xml, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for <device id="..." name="...">
		if strings.HasPrefix(line, "<device ") && strings.Contains(line, "id=") {
			// Extract id attribute
			idStart := strings.Index(line, "id=\"")
			if idStart == -1 {
				continue
			}
			idStart += 4 // Skip 'id="'
			idEnd := strings.Index(line[idStart:], "\"")
			if idEnd == -1 {
				continue
			}
			deviceID := line[idStart : idStart+idEnd]
			devices = append(devices, deviceID)
		}
	}

	return devices, nil
}

func parseProtocolVersionFromXML(xmlPayload string) ProtocolVersion {
	decoder := xml.NewDecoder(strings.NewReader(xmlPayload))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ProtocolVersion{}
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "context" {
			continue
		}

		var version ProtocolVersion
		for _, attr := range start.Attr {
			switch attr.Name.Local {
			case "version-major":
				if val, err := strconv.Atoi(attr.Value); err == nil {
					version.Major = val
				}
			case "version-minor":
				if val, err := strconv.Atoi(attr.Value); err == nil {
					version.Minor = val
				}
			}
		}
		return version
	}
}

// GetChannels returns the list of channel IDs for a given device.
func (c *Client) GetChannels(device string) ([]string, error) {
	return c.GetChannelsWithContext(context.Background(), device)
}

// GetChannelsWithContext gets channels with context support.
func (c *Client) GetChannelsWithContext(ctx context.Context, device string) ([]string, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}

	payload, err := c.SendWithContext(ctx, fmt.Sprintf("LIST_CHANNELS %s", device))
	if err != nil {
		return nil, err
	}
	if payload == "" {
		return nil, nil
	}
	return strings.Fields(payload), nil
}

// CreateBuffer is deprecated. Use CreateStreamBuffer instead.
func (c *Client) CreateBuffer(device string, samples int) (string, error) {
	return c.SendWithContext(context.Background(), fmt.Sprintf("CREATE_BUFFER %s %d", device, samples))
}

// OpenBuffer issues the OPEN command to allocate a streaming buffer.
func (c *Client) OpenBuffer(device string, samples int) error {
	return c.OpenBufferWithContext(context.Background(), device, samples)
}

// OpenBufferWithContext opens a buffer with context support.
func (c *Client) OpenBufferWithContext(ctx context.Context, device string, samples int) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return fmt.Errorf("sample count must be positive")
	}

	_, err := c.SendWithContext(ctx, fmt.Sprintf("OPEN %s %d", device, samples))
	return err
}

// ReadBuffer requests binary sample data from the remote buffer.
func (c *Client) ReadBuffer(device string, samples int) ([]byte, error) {
	return c.ReadBufferWithContext(context.Background(), device, samples)
}

// ReadBufferWithContext reads buffer with context support.
func (c *Client) ReadBufferWithContext(ctx context.Context, device string, samples int) ([]byte, error) {
	if strings.TrimSpace(device) == "" {
		return nil, fmt.Errorf("device name is required")
	}
	if samples <= 0 {
		return nil, fmt.Errorf("sample count must be positive")
	}

	return c.sendBinaryWithContext(ctx, fmt.Sprintf("READBUF %s %d", device, samples), nil)
}

// WriteBuffer writes binary IQ data to the remote buffer.
func (c *Client) WriteBuffer(device string, data []byte) error {
	return c.WriteBufferWithContext(context.Background(), device, data)
}

// WriteBufferWithContext writes buffer with context support.
func (c *Client) WriteBufferWithContext(ctx context.Context, device string, data []byte) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data provided for buffer write")
	}

	cmd := fmt.Sprintf("WRITEBUF %s %d", device, len(data))
	_, err := c.sendBinaryWithContext(ctx, cmd, data)
	return err
}

// CloseBuffer tears down the remote buffer.
func (c *Client) CloseBuffer(device string) error {
	return c.CloseBufferWithContext(context.Background(), device)
}

// CloseBufferWithContext closes buffer with context support.
func (c *Client) CloseBufferWithContext(ctx context.Context, device string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}

	_, err := c.SendWithContext(ctx, fmt.Sprintf("CLOSE %s", device))
	return err
}

// ReadAttr reads a device or channel attribute.
func (c *Client) ReadAttr(device, channel, attr string) (string, error) {
	return c.ReadAttrWithContext(context.Background(), device, channel, attr)
}

// ReadAttrWithContext reads attribute with context support.
func (c *Client) ReadAttrWithContext(ctx context.Context, device, channel, attr string) (string, error) {
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

	return c.SendWithContext(ctx, fmt.Sprintf("READ_ATTR %s", target))
}

// WriteAttr writes a device or channel attribute value.
func (c *Client) WriteAttr(device, channel, attr, value string) error {
	return c.WriteAttrWithContext(context.Background(), device, channel, attr, value)
}

// WriteAttrWithContext writes attribute with context support.
func (c *Client) WriteAttrWithContext(ctx context.Context, device, channel, attr, value string) error {
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

	_, err := c.SendWithContext(ctx, fmt.Sprintf("WRITE_ATTR %s", target))
	return err
}

// ReadDebugAttr reads a debug attribute (direct register access).
func (c *Client) ReadDebugAttr(device, attr string) (string, error) {
	return c.ReadDebugAttrWithContext(context.Background(), device, attr)
}

// ReadDebugAttrWithContext reads debug attribute with context support.
func (c *Client) ReadDebugAttrWithContext(ctx context.Context, device, attr string) (string, error) {
	if strings.TrimSpace(device) == "" {
		return "", fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(attr) == "" {
		return "", fmt.Errorf("attribute name is required")
	}

	return c.SendWithContext(ctx, fmt.Sprintf("READ %s DEBUG %s", device, attr))
}

// WriteDebugAttr writes a debug attribute (direct register access).
func (c *Client) WriteDebugAttr(device, attr, value string) error {
	return c.WriteDebugAttrWithContext(context.Background(), device, attr, value)
}

// WriteDebugAttrWithContext writes debug attribute with context support.
func (c *Client) WriteDebugAttrWithContext(ctx context.Context, device, attr, value string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(attr) == "" {
		return fmt.Errorf("attribute name is required")
	}

	_, err := c.SendWithContext(ctx, fmt.Sprintf("WRITE %s DEBUG %s %s", device, attr, value))
	return err
}

// AttrOperation represents a single attribute read or write operation.
type AttrOperation struct {
	Device  string
	Channel string
	Attr    string
	Value   string // Empty for reads
	IsWrite bool
}

// BatchReadAttrs reads multiple attributes in a single pipelined operation.
func (c *Client) BatchReadAttrs(ops []AttrOperation) ([]string, error) {
	return c.BatchReadAttrsWithContext(context.Background(), ops)
}

// BatchReadAttrsWithContext reads multiple attributes with context support.
func (c *Client) BatchReadAttrsWithContext(ctx context.Context, ops []AttrOperation) ([]string, error) {
	results := make([]string, len(ops))
	for i, op := range ops {
		if op.IsWrite {
			return nil, fmt.Errorf("operation %d is a write, use BatchWriteAttrs", i)
		}
		val, err := c.ReadAttrWithContext(ctx, op.Device, op.Channel, op.Attr)
		if err != nil {
			return nil, fmt.Errorf("read operation %d failed: %w", i, err)
		}
		results[i] = val
	}
	return results, nil
}

// BatchWriteAttrs writes multiple attributes in a single pipelined operation.
func (c *Client) BatchWriteAttrs(ops []AttrOperation) error {
	return c.BatchWriteAttrsWithContext(context.Background(), ops)
}

// BatchWriteAttrsWithContext writes multiple attributes with context support.
func (c *Client) BatchWriteAttrsWithContext(ctx context.Context, ops []AttrOperation) error {
	for i, op := range ops {
		if !op.IsWrite {
			return fmt.Errorf("operation %d is a read, use BatchReadAttrs", i)
		}
		if err := c.WriteAttrWithContext(ctx, op.Device, op.Channel, op.Attr, op.Value); err != nil {
			return fmt.Errorf("write operation %d failed: %w", i, err)
		}
	}
	return nil
}

func (c *Client) sendBinaryWithContext(ctx context.Context, cmd string, payload []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.reader == nil {
		return nil, fmt.Errorf("client is not connected")
	}
	if strings.TrimSpace(cmd) == "" {
		return nil, fmt.Errorf("command is required")
	}

	c.metrics.CommandsSent.Add(1)

	// Set deadline based on context
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetDeadline(deadline); err != nil {
			c.metrics.CommandsFailed.Add(1)
			return nil, err
		}
		defer c.conn.SetDeadline(time.Time{}) // Clear deadline
	}

	// Send command
	cmdBytes := []byte(cmd + "\n")
	n, err := c.conn.Write(cmdBytes)
	if err != nil {
		c.metrics.CommandsFailed.Add(1)
		c.isConnected.Store(false)

		// Attempt reconnect if configured
		if c.reconnectCfg != nil {
			if reconnectErr := c.reconnect(ctx); reconnectErr == nil {
				// Retry command after reconnect
				return c.sendBinaryWithContext(ctx, cmd, payload)
			}
		}
		return nil, err
	}
	c.metrics.BytesSent.Add(uint64(n))

	// Send payload if present
	if len(payload) > 0 {
		lenPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(lenPrefix, uint32(len(payload)))
		n, err := c.conn.Write(lenPrefix)
		if err != nil {
			c.metrics.CommandsFailed.Add(1)
			return nil, err
		}
		c.metrics.BytesSent.Add(uint64(n))

		n, err = c.conn.Write(payload)
		if err != nil {
			c.metrics.CommandsFailed.Add(1)
			return nil, err
		}
		c.metrics.BytesSent.Add(uint64(n))
	}

	// Read response header
	line, err := c.reader.ReadString('\n')
	if err != nil {
		c.metrics.CommandsFailed.Add(1)
		c.isConnected.Store(false)
		return nil, err
	}
	c.metrics.BytesReceived.Add(uint64(len(line)))
	line = strings.TrimSpace(line)

	// Check for XML response BEFORE splitting into fields (XML has many whitespace-separated tokens)
	if strings.HasPrefix(line, "<?xml") {
		// Consume and cache the entire XML document
		xmlBuilder := strings.Builder{}
		xmlBuilder.WriteString(line)
		xmlBuilder.WriteString("\n")
		if strings.Contains(line, "</context>") {
			xmlPayload := xmlBuilder.String()
			c.xmlContext = xmlPayload
			if version := parseProtocolVersionFromXML(xmlPayload); !version.IsZero() {
				c.protoVersion = version
			}
			c.metrics.LastCommandTime.Store(time.Now())
			return nil, nil
		}

		for {
			xmlLine, readErr := c.reader.ReadString('\n')
			if readErr != nil {
				break
			}
			c.metrics.BytesReceived.Add(uint64(len(xmlLine)))
			xmlBuilder.WriteString(xmlLine)
			if strings.Contains(xmlLine, "</context>") {
				break
			}
		}

		// Cache the XML context
		xmlPayload := xmlBuilder.String()
		c.xmlContext = xmlPayload
		if version := parseProtocolVersionFromXML(xmlPayload); !version.IsZero() {
			c.protoVersion = version
		}
		c.metrics.LastCommandTime.Store(time.Now())
		return nil, nil // Treat as success with no data
	}

	parts := strings.Fields(line)

	// Handle error-only response (e.g., "-22" without length field)
	if len(parts) == 1 {
		status, err := strconv.Atoi(parts[0])
		if err != nil {
			c.metrics.CommandsFailed.Add(1)
			return nil, fmt.Errorf("malformed reply header: %q", line)
		}
		if status < 0 {
			c.metrics.CommandsFailed.Add(1)
			return nil, fmt.Errorf("iiod error %d (EINVAL)", status)
		}
		// Positive single number - legacy format, treat as successful response
		c.metrics.LastCommandTime.Store(time.Now())
		return []byte(line), nil
	}

	if len(parts) != 2 {
		c.metrics.CommandsFailed.Add(1)
		return nil, fmt.Errorf("malformed reply header: %q", line)
	}

	status, err := strconv.Atoi(parts[0])
	if err != nil {
		c.metrics.CommandsFailed.Add(1)
		return nil, fmt.Errorf("invalid status code: %w", err)
	}
	length, err := strconv.Atoi(parts[1])
	if err != nil {
		c.metrics.CommandsFailed.Add(1)
		return nil, fmt.Errorf("invalid payload length: %w", err)
	}
	if length < 0 {
		c.metrics.CommandsFailed.Add(1)
		return nil, fmt.Errorf("negative payload length: %d", length)
	}

	// Read payload
	var resp []byte
	if length > 0 {
		resp = make([]byte, length)
		if _, err := io.ReadFull(c.reader, resp); err != nil {
			c.metrics.CommandsFailed.Add(1)
			return nil, err
		}
		c.metrics.BytesReceived.Add(uint64(length))
	}

	// Check status
	if status != 0 {
		c.metrics.CommandsFailed.Add(1)
		msg := strings.TrimSpace(string(resp))
		if msg != "" {
			return nil, fmt.Errorf("iiod error %d: %s", status, msg)
		}
		return nil, fmt.Errorf("iiod error %d", status)
	}

	// Update metrics
	c.metrics.LastCommandTime.Store(time.Now())

	return resp, nil
}
