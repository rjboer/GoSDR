package iiod

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// -----------------------------------------------------------------------------
// TEXT-BASED TRANSPORT (PlutoSDR Compatible)
// -----------------------------------------------------------------------------

// textClient wraps a line-based ASCII IIOD connection.
type textClient struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

func newTextClient(conn net.Conn) *textClient {
	return &textClient{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

func (tc *textClient) Close() error {
	return tc.conn.Close()
}

// -----------------------------------------------------------------------------

// Client is the main IIOD client object in your system.
// Fields added for hybrid-mode + XML knowledge.
type Client struct {
	conn net.Conn

	// Text protocol transport (Pluto uses this)
	text *textClient

	// XML context + fast index (optional, but used by Pluto)
	xmlCtx *IIODcontext
	xmlIdx *IIODIndex

	// Hybrid feature flags
	supportsBinary bool
	supportsText   bool

	// Debug printing flag
	Debug bool
}

// -----------------------------------------------------------------------------
// TEXT COMMAND SENDER
// -----------------------------------------------------------------------------

// sendTextCommand writes a raw text command and flushes it.
func (c *Client) sendTextCommand(cmd string) error {
	if c.Debug {
		fmt.Printf("[IIOD TEXT] >> %s\n", strings.TrimSpace(cmd))
	}
	_, err := c.text.writer.WriteString(cmd)
	if err != nil {
		return err
	}
	return c.text.writer.Flush()
}

// readTextLine reads a single line terminated with \n from IIOD.
func (c *Client) readTextLine() (string, error) {
	line, err := c.text.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")

	if c.Debug {
		fmt.Printf("[IIOD TEXT] << %s\n", line)
	}
	return line, nil
}

// readTextUntilEOF reads all remaining data until socket read returns 0/EOF timeout.
func (c *Client) readTextUntilEOF(timeout time.Duration) ([]byte, error) {
	var buf bytes.Buffer
	c.text.conn.SetReadDeadline(time.Now().Add(timeout))

	tmp := make([]byte, 4096)
	for {
		n, err := c.text.reader.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if err == io.EOF || isNetTimeout(err) {
				break
			}
			return nil, err
		}
	}

	raw := buf.Bytes()
	if c.Debug {
		fmt.Printf("[IIOD TEXT] << RAW(%d bytes)\n", len(raw))
	}
	return raw, nil
}

func isNetTimeout(err error) bool {
	nerr, ok := err.(net.Error)
	return ok && nerr.Timeout()
}

// -----------------------------------------------------------------------------
// PRINT Command — Retrieve IIOD XML Context
// -----------------------------------------------------------------------------

// getXMLContextWithContextText issues a PRINT command and loads XML.
func (c *Client) getXMLContextWithContextText(ctx context.Context) error {
	if !c.supportsText {
		return fmt.Errorf("text mode disabled")
	}

	// IIOD text protocol → "PRINT\n"
	if err := c.sendTextCommand("PRINT\n"); err != nil {
		return fmt.Errorf("failed to send PRINT: %w", err)
	}

	// Read all until EOF pause.
	raw, err := c.readTextUntilEOF(300 * time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to read PRINT response: %w", err)
	}

	if len(raw) == 0 {
		return fmt.Errorf("empty PRINT response")
	}

	if c.Debug {
		fmt.Printf("[IIOD DEBUG] Raw PRINT XML (%d bytes):\n%s\n\n",
			len(raw), NormalizeXMLForDebug(raw))
	}

	// Parse and index XML
	xmlCtx, xmlIdx, err := ParseIIODXML(raw)
	if err != nil {
		return fmt.Errorf("XML parse failed: %w", err)
	}

	c.xmlCtx = xmlCtx
	c.xmlIdx = xmlIdx

	return nil
}

// -----------------------------------------------------------------------------
// PUBLIC: ReadAttrText / WriteAttrText (sysfs-style), with XML mapping
// -----------------------------------------------------------------------------

// ReadAttrText implements attribute read using the text IIOD backend.
// device="ad9361-phy", channel="voltage0", attr="hardwaregain"
func (c *Client) ReadAttrText(
	ctx context.Context,
	device, channel, attr string,
) (string, error) {

	if c.xmlIdx == nil {
		return "", fmt.Errorf("XML index not loaded; cannot resolve filenames")
	}

	filename, err := TryResolveAttribute(c.xmlIdx, device, channel, attr)
	if err != nil {
		return "", err
	}

	// Text protocol → "READ device channel filename\n"
	cmd := ""
	if channel == "" {
		cmd = fmt.Sprintf("READ %s %s\n", device, filename)
	} else {
		cmd = fmt.Sprintf("READ %s %s %s\n", device, channel, filename)
	}

	if err := c.sendTextCommand(cmd); err != nil {
		return "", fmt.Errorf("send READ failed: %w", err)
	}

	// Expect a single line response
	line, err := c.readTextLine()
	if err != nil {
		return "", fmt.Errorf("READ response error: %w", err)
	}

	// IIOD text protocol returns either:
	//   OK <value>
	//   ERROR <msg>
	if strings.HasPrefix(line, "OK ") {
		return strings.TrimSpace(strings.TrimPrefix(line, "OK ")), nil
	}

	if strings.HasPrefix(line, "ERROR ") {
		return "", fmt.Errorf("IIOD TEXT error: %s", strings.TrimPrefix(line, "ERROR "))
	}

	return "", fmt.Errorf("malformed TEXT READ response: %q", line)
}

// WriteAttrText implements attribute write using the text IIOD backend.
func (c *Client) WriteAttrText(
	ctx context.Context,
	device, channel, attr, value string,
) error {

	if c.xmlIdx == nil {
		return fmt.Errorf("XML index not loaded; cannot resolve filenames")
	}

	filename, err := TryResolveAttribute(c.xmlIdx, device, channel, attr)
	if err != nil {
		return err
	}

	cmd := ""
	if channel == "" {
		cmd = fmt.Sprintf("WRITE %s %s %s\n", device, filename, value)
	} else {
		cmd = fmt.Sprintf("WRITE %s %s %s %s\n", device, channel, filename, value)
	}

	if err := c.sendTextCommand(cmd); err != nil {
		return fmt.Errorf("send WRITE failed: %w", err)
	}

	line, err := c.readTextLine()
	if err != nil {
		return fmt.Errorf("WRITE response error: %w", err)
	}

	if strings.HasPrefix(line, "OK") {
		return nil
	}

	if strings.HasPrefix(line, "ERROR ") {
		return fmt.Errorf("IIOD TEXT error: %s", strings.TrimPrefix(line, "ERROR "))
	}

	return fmt.Errorf("malformed TEXT WRITE response: %q", line)
}

// -----------------------------------------------------------------------------
// HYBRID-COMPAT WRAPPERS (used by pluto.go)
// -----------------------------------------------------------------------------

func (c *Client) ReadAttrTextCompat(
	ctx context.Context, device, channel, attr string,
) (string, error) {
	// for Pluto: always text
	return c.ReadAttrText(ctx, device, channel, attr)
}

func (c *Client) WriteAttrTextCompat(
	ctx context.Context, device, channel, attr, value string,
) error {
	// for Pluto: always text
	return c.WriteAttrText(ctx, device, channel, attr, value)
}

// -----------------------------------------------------------------------------
// PUBLIC INITIALIZATION ENTRY POINT (used by Dial/hybrid)
// -----------------------------------------------------------------------------

// EnableTextMode initializes the text transport after TCP dial.
func (c *Client) EnableTextMode(conn net.Conn) error {
	c.text = newTextClient(conn)
	c.supportsText = true
	return nil
}
