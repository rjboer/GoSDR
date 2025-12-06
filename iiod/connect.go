package iiod

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
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

	mu              sync.Mutex
	metrics         ClientMetrics
	reconnectCfg    *ReconnectConfig
	addr            string
	isConnected     atomic.Bool
	ProtocolVersion ProtocolVersion
	xmlContext      string // Cached XML context from server
	deviceIndexMap  map[string]uint16
	attributeCodes  map[attrKey]uint16
	stateMu         sync.Mutex
	openBuffers     map[string]int
	timeout         time.Duration
	healthWindow    time.Duration
}

// ErrWriteNotSupported indicates that the connected IIOD server does not allow attribute writes (e.g., protocol v0.25).
var ErrWriteNotSupported = errors.New("iiod protocol does not support attribute writes")

// ProtocolVersion captures the IIOD protocol version reported by the server.
type ProtocolVersion struct {
	Major int
	Minor int
}

type attrKey struct {
	device  string
	channel string
	attr    string
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

// AttributeInfo captures metadata for a device or channel attribute parsed from XML.
type AttributeInfo struct {
	Name     string
	Filename string
	Type     string
	Unit     string
	Value    string
}

// ChannelInfo captures metadata for a device channel parsed from XML.
type ChannelInfo struct {
	ID         string
	Type       string
	Attributes []AttributeInfo
}

// DeviceInfo captures metadata for a device parsed from XML.
type DeviceInfo struct {
	ID         string
	Name       string
	Attributes []AttributeInfo
	Channels   []ChannelInfo
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

	if _, err := client.GetXMLContextWithContext(ctx); err != nil {
		log.Printf("Connected to %s but failed to fetch IIOD XML context: %v", addr, err)
	} else {
		client.logProtocolVersion()
	}

	return client, nil
}

// IsLegacy reports whether the remote server is using a legacy IIOD protocol (v0.25).
func (c *Client) IsLegacy() bool {
	return c.ProtocolVersion.Major == 0 && c.ProtocolVersion.Minor > 0 && c.ProtocolVersion.Minor < 26
}

// SupportsWrite reports whether the server is expected to support attribute write operations.
func (c *Client) SupportsWrite() bool {
	return !c.IsLegacy()
}

func (c *Client) logProtocolVersion() {
	if c.ProtocolVersion.Major == 0 && c.ProtocolVersion.Minor == 0 {
		log.Printf("Connected to %s (IIOD protocol version unknown)", c.addr)
		return
	}

	log.Printf("Connected to %s using IIOD protocol v%d.%d", c.addr, c.ProtocolVersion.Major, c.ProtocolVersion.Minor)
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

// GetDeviceInfo retrieves detailed device metadata via the XML command.
func (c *Client) GetDeviceInfo() ([]DeviceInfo, error) {
	return c.GetDeviceInfoWithContext(context.Background())
}

// GetDeviceInfoWithContext retrieves device metadata via the XML command with context support.
func (c *Client) GetDeviceInfoWithContext(ctx context.Context) ([]DeviceInfo, error) {
	resp, err := c.SendWithContext(ctx, "XML")
	if err != nil {
		return nil, err
	}

	if resp == "" {
		return nil, fmt.Errorf("empty XML response")
	}

	c.cacheXMLMetadata(resp)
	return parseDeviceInfoFromXML(resp)
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
	if c.xmlContext != "" {
		if c.deviceIndexMap == nil || c.attributeCodes == nil {
			if err := c.refreshMetadataMaps(c.xmlContext); err != nil {
				log.Printf("Failed to parse IIOD metadata maps from cached XML: %v", err)
			}
		}
		return c.xmlContext, nil
	}

	resp, err := c.SendWithContext(ctx, "PRINT")
	if err != nil {
		return "", err
	}

	if c.xmlContext != "" {
		return c.xmlContext, nil
	}

	if resp == "" {
		return "", fmt.Errorf("no XML context received")
	}

	c.cacheXMLMetadata(resp)
	return c.xmlContext, nil
}

// ListDevicesFromXML parses device names from the XML context.
// This is a fallback for older IIOD versions that don't support LIST_DEVICES.
func (c *Client) ListDevicesFromXML(ctx context.Context) ([]string, error) {
	xmlContent, err := c.GetXMLContextWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("get XML context: %w", err)
	}

	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	devices := []string{}

	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			if tokenErr == io.EOF {
				break
			}
			return nil, fmt.Errorf("parse XML context: %w", tokenErr)
		}

		switch element := token.(type) {
		case xml.StartElement:
			if element.Name.Local != "device" {
				continue
			}

			for _, attr := range element.Attr {
				if attr.Name.Local == "id" {
					devices = append(devices, attr.Value)
					break
				}
			}
		}
	}

	return devices, nil
}

func (c *Client) updateProtocolVersionFromXML(xmlContent string) {
	version, ok := parseProtocolVersionFromXML(xmlContent)
	if !ok {
		return
	}

	c.ProtocolVersion = version
}

func (c *Client) cacheXMLMetadata(xmlContent string) {
	c.xmlContext = xmlContent
	c.updateProtocolVersionFromXML(xmlContent)

	if err := c.refreshMetadataMaps(xmlContent); err != nil {
		log.Printf("Failed to parse IIOD metadata maps from XML: %v", err)
	}
}

func (c *Client) refreshMetadataMaps(xmlContent string) error {
	deviceIdx, attrCodes, err := parseDeviceIndexAndAttrCodes(xmlContent)
	if err != nil {
		return err
	}

	c.deviceIndexMap = deviceIdx
	c.attributeCodes = attrCodes
	return nil
}

func parseProtocolVersionFromXML(xmlContent string) (ProtocolVersion, bool) {
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))

	for {
		token, err := decoder.Token()
		if err != nil {
			return ProtocolVersion{}, false
		}

		startElement, ok := token.(xml.StartElement)
		if !ok {
			continue
		}

		if startElement.Name.Local != "context" {
			continue
		}

		version := ProtocolVersion{}
		for _, attr := range startElement.Attr {
			switch attr.Name.Local {
			case "version-major":
				if major, convErr := strconv.Atoi(attr.Value); convErr == nil {
					version.Major = major
				}
			case "version-minor":
				if minor, convErr := strconv.Atoi(attr.Value); convErr == nil {
					version.Minor = minor
				}
			}
		}

		if version.Major != 0 || version.Minor != 0 {
			return version, true
		}

		return version, false
	}
}

func parseDeviceIndexAndAttrCodes(xmlContent string) (map[string]uint16, map[attrKey]uint16, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))
	deviceIndexes := make(map[string]uint16)
	attrCodes := make(map[attrKey]uint16)

	var currentDevice string
	var currentChannel string
	var nextDeviceIndex uint16

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, err
		}

		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "device":
				currentChannel = ""
				currentDevice = attrValue(element.Attr, "id")
				if currentDevice == "" {
					continue
				}

				idxStr := attrValue(element.Attr, "index")
				parsedIdx, err := parseUintWithFallback(idxStr, nextDeviceIndex)
				if err != nil {
					log.Printf("iiod: failed to parse device index for %q: %v", currentDevice, err)
				}

				deviceIndexes[currentDevice] = parsedIdx
				if parsedIdx >= nextDeviceIndex {
					nextDeviceIndex = parsedIdx + 1
				}

			case "channel":
				currentChannel = attrValue(element.Attr, "id")
			case "attribute":
				name := attrValue(element.Attr, "name")
				codeStr := attrValue(element.Attr, "code")

				if codeStr == "" || name == "" || currentDevice == "" {
					continue
				}

				code, err := strconv.ParseUint(codeStr, 0, 16)
				if err != nil {
					log.Printf("iiod: failed to parse attribute code %q for %s/%s/%s: %v", codeStr, currentDevice, currentChannel, name, err)
					continue
				}

				attrCodes[attrKey{device: currentDevice, channel: currentChannel, attr: name}] = uint16(code)
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "device":
				currentDevice = ""
				currentChannel = ""
			case "channel":
				currentChannel = ""
			}
		}
	}

	return deviceIndexes, attrCodes, nil
}

func parseDeviceInfoFromXML(xmlContent string) ([]DeviceInfo, error) {
	decoder := xml.NewDecoder(strings.NewReader(xmlContent))

	var devices []DeviceInfo
	var currentDevice *DeviceInfo
	var currentChannel *ChannelInfo

	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		switch element := token.(type) {
		case xml.StartElement:
			switch element.Name.Local {
			case "device":
				devices = append(devices, DeviceInfo{
					ID:   attrValue(element.Attr, "id"),
					Name: attrValue(element.Attr, "name"),
				})
				currentDevice = &devices[len(devices)-1]
				currentChannel = nil
			case "channel":
				if currentDevice == nil {
					continue
				}

				currentDevice.Channels = append(currentDevice.Channels, ChannelInfo{
					ID:   attrValue(element.Attr, "id"),
					Type: attrValue(element.Attr, "type"),
				})
				currentChannel = &currentDevice.Channels[len(currentDevice.Channels)-1]
			case "attribute":
				if currentDevice == nil {
					continue
				}

				attrInfo := AttributeInfo{
					Name:     attrValue(element.Attr, "name"),
					Filename: attrValue(element.Attr, "filename"),
					Type:     attrValue(element.Attr, "type"),
					Unit:     attrValue(element.Attr, "unit"),
				}

				var value string
				if err := decoder.DecodeElement(&value, &element); err == nil {
					attrInfo.Value = strings.TrimSpace(value)
				} else {
					log.Printf("iiod: failed to decode attribute %q content: %v", attrInfo.Name, err)
				}

				if currentChannel != nil {
					currentChannel.Attributes = append(currentChannel.Attributes, attrInfo)
				} else {
					currentDevice.Attributes = append(currentDevice.Attributes, attrInfo)
				}
			}
		case xml.EndElement:
			switch element.Name.Local {
			case "channel":
				currentChannel = nil
			case "device":
				currentDevice = nil
				currentChannel = nil
			}
		}
	}

	return devices, nil
}

func parseUintWithFallback(value string, fallback uint16) (uint16, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return fallback, err
	}

	return uint16(parsed), nil
}

func attrValue(attrs []xml.Attr, local string) string {
	for _, attr := range attrs {
		if attr.Name.Local == local {
			return attr.Value
		}
	}

	return ""
}

func (c *Client) ensureMetadataMaps(ctx context.Context) error {
	if c.deviceIndexMap != nil && c.attributeCodes != nil {
		return nil
	}

	xmlContent := c.xmlContext
	if xmlContent == "" {
		var err error
		xmlContent, err = c.GetXMLContextWithContext(ctx)
		if err != nil {
			return err
		}
	}

	return c.refreshMetadataMaps(xmlContent)
}

func (c *Client) logMetadataLookup(ctx context.Context, device, channel, attr string) {
	if err := c.ensureMetadataMaps(ctx); err != nil {
		log.Printf("iiod: could not load XML metadata for %s/%s/%s: %v", device, channel, attr, err)
		return
	}

	if _, ok := c.deviceIndexMap[device]; !ok {
		log.Printf("iiod: device %q not found in IIOD XML metadata; binary attribute access may fail", device)
	}

	if _, ok := c.attributeCodes[attrKey{device: device, channel: channel, attr: attr}]; !ok {
		log.Printf("iiod: attribute code missing for %q (channel=%q device=%q)", attr, channel, device)
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
	return c.ReadAttrBinary(ctx, device, channel, attr)
}

// WriteAttr writes a device or channel attribute value.
func (c *Client) WriteAttr(device, channel, attr, value string) error {
	return c.WriteAttrWithContext(context.Background(), device, channel, attr, value)
}

// WriteAttrWithContext writes attribute with context support.
func (c *Client) WriteAttrWithContext(ctx context.Context, device, channel, attr, value string) error {
	return c.WriteAttrBinary(ctx, device, channel, attr, value)
}

// WriteAttrCompat writes an attribute while handling legacy servers that do not support write operations.
func (c *Client) WriteAttrCompat(device, channel, attr, value string) error {
	return c.WriteAttrCompatWithContext(context.Background(), device, channel, attr, value)
}

// WriteAttrCompatWithContext writes an attribute and returns a descriptive error when the server reports no write support.
func (c *Client) WriteAttrCompatWithContext(ctx context.Context, device, channel, attr, value string) error {
	if c.IsLegacy() {
		log.Printf("IIOD protocol v0.%d does not support attribute writes; skipping %s/%s/%s", c.ProtocolVersion.Minor, device, channel, attr)
		return fmt.Errorf("%w: protocol v0.%d", ErrWriteNotSupported, c.ProtocolVersion.Minor)
	}

	return c.WriteAttrBinary(ctx, device, channel, attr, value)
}

// ReadAttrBinary reads a device or channel attribute using the binary protocol.
// It automatically adapts to legacy (v0.25) response formats.
func (c *Client) ReadAttrBinary(ctx context.Context, device, channel, attr string) (string, error) {
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

	// Modern servers still expect the text-based READ_ATTR command.
	if !c.IsLegacy() {
		return c.SendWithContext(ctx, fmt.Sprintf("READ_ATTR %s", target))
	}

	c.logMetadataLookup(ctx, device, channel, attr)

	// Legacy v0.25 servers respond with a 32-bit status followed by payload bytes.
	payload := []byte(target + "\n")
	cmd := IIODCommand{Opcode: 6, Flags: 0, Address: 0, Length: uint32(len(payload))}
	if err := c.sendCommand(ctx, cmd, payload); err != nil {
		return "", err
	}
	status, err := c.readResponse(ctx)
	if err != nil {
		return "", err
	}

	// In legacy responses, a positive status is the payload length.
	if status == 0 {
		return "", nil
	}

	buf := make([]byte, status)
	if _, err := io.ReadFull(c.reader, buf); err != nil {
		return "", err
	}
	c.metrics.BytesReceived.Add(uint64(status))
	c.metrics.LastCommandTime.Store(time.Now())

	return strings.TrimSpace(string(buf)), nil
}

// WriteAttrBinary writes a device or channel attribute using the binary protocol (opcode 7).
// The payload includes the attribute target and the length-prefixed data, matching v0.25 expectations.
func (c *Client) WriteAttrBinary(ctx context.Context, device, channel, attr, value string) error {
	if strings.TrimSpace(device) == "" {
		return fmt.Errorf("device name is required")
	}
	if strings.TrimSpace(attr) == "" {
		return fmt.Errorf("attribute name is required")
	}

	target := fmt.Sprintf("%s %s", device, attr)
	if channel != "" {
		target = fmt.Sprintf("%s %s %s", device, channel, attr)
	}

	// Modern servers: keep using the text-based command path.
	if !c.IsLegacy() {
		targetWithValue := fmt.Sprintf("%s %s", target, value)
		_, err := c.SendWithContext(ctx, fmt.Sprintf("WRITE_ATTR %s", targetWithValue))
		return err
	}

	c.logMetadataLookup(ctx, device, channel, attr)

	// Legacy binary write: opcode 7 with length-prefixed data.
	valueBytes := []byte(value)
	buf := bytes.NewBufferString(target + "\n")
	if err := binary.Write(buf, binary.BigEndian, uint64(len(valueBytes))); err != nil {
		return fmt.Errorf("encode value length: %w", err)
	}
	buf.Write(valueBytes)

	cmd := IIODCommand{Opcode: 7, Flags: 0, Address: 0, Length: uint32(buf.Len())}
	if err := c.sendCommand(ctx, cmd, buf.Bytes()); err != nil {
		return err
	}
	_, err := c.readResponse(ctx)
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
		c.cacheXMLMetadata(xmlBuilder.String())
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
