package iiod

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

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

// IIODCommand represents the 8-byte binary header used by the IIOD protocol.
type IIODCommand struct {
	Opcode  uint8
	Flags   uint8
	Address uint16
	Length  uint32
}

// Marshal encodes the command into its 8-byte network representation.
func (cmd IIODCommand) Marshal() ([]byte, error) {
	header := make([]byte, 8)
	header[0] = cmd.Opcode
	header[1] = cmd.Flags
	binary.BigEndian.PutUint16(header[2:], cmd.Address)
	binary.BigEndian.PutUint32(header[4:], cmd.Length)
	return header, nil
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
	resp, err := c.sendBinaryCommand(ctx, cmd, nil)
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

	return ContextInfo{Major: major, Minor: minor, Description: description}, nil
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
	return c.SendWithContext(ctx, "PRINT")
}

// ListDevicesFromXML parses device names from the XML context.
// This is a fallback for older IIOD versions that don't support LIST_DEVICES.
func (c *Client) ListDevicesFromXML(ctx context.Context) ([]string, error) {
	xml, err := c.GetXMLContextWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get XML context: %w", err)
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

	return c.sendBinaryCommand(ctx, fmt.Sprintf("READBUF %s %d", device, samples), nil)
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
	_, err := c.sendBinaryCommand(ctx, cmd, data)
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

func (c *Client) sendCommand(ctx context.Context, cmd IIODCommand, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.reader == nil {
		return fmt.Errorf("client is not connected")
	}

	header, err := cmd.Marshal()
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}

	c.metrics.CommandsSent.Add(1)

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetDeadline(deadline); err != nil {
			c.metrics.CommandsFailed.Add(1)
			return err
		}
		defer c.conn.SetDeadline(time.Time{})
	}

	n, err := c.conn.Write(header)
	if err != nil {
		c.metrics.CommandsFailed.Add(1)
		c.isConnected.Store(false)
		return err
	}
	c.metrics.BytesSent.Add(uint64(n))

	if len(payload) > 0 {
		n, err = c.conn.Write(payload)
		if err != nil {
			c.metrics.CommandsFailed.Add(1)
			return err
		}
		c.metrics.BytesSent.Add(uint64(n))
	}

	return nil
}

func (c *Client) readResponse(ctx context.Context) (int32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || c.reader == nil {
		return 0, fmt.Errorf("client is not connected")
	}

	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetDeadline(deadline); err != nil {
			c.metrics.CommandsFailed.Add(1)
			return 0, err
		}
		defer c.conn.SetDeadline(time.Time{})
	}

	var status int32
	if err := binary.Read(c.reader, binary.BigEndian, &status); err != nil {
		c.metrics.CommandsFailed.Add(1)
		c.isConnected.Store(false)
		return 0, err
	}
	c.metrics.BytesReceived.Add(4)

	if status < 0 {
		c.metrics.CommandsFailed.Add(1)
		return status, fmt.Errorf("iiod error %d", status)
	}

	c.metrics.LastCommandTime.Store(time.Now())
	return status, nil
}

func (c *Client) sendBinaryCommand(ctx context.Context, cmd string, payload []byte) ([]byte, error) {
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
				return c.sendBinaryCommand(ctx, cmd, payload)
			}
		}
		return nil, err
	}
	c.metrics.BytesSent.Add(uint64(n))

	// Send payload if present
	if len(payload) > 0 {
		n, err := c.conn.Write(payload)
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
		c.xmlContext = xmlBuilder.String()
		c.metrics.LastCommandTime.Store(time.Now())
		return nil, nil // Treat as success with no data
	}

	parts := strings.Fields(line)

	// Handle error-only response (e.g., "-22" without length field)
	if len(parts) == 1 {
		status, err := strconv.Atoi(parts[0])
		if err != nil {
			// Other non-numeric single-field responses - treat as data
			c.metrics.LastCommandTime.Store(time.Now())
			return []byte(line), nil
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
