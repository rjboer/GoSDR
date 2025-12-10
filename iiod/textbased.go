package iiod

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// TextBackend implements the IIOD text protocol.
// This backend is used when binary probing fails or when explicitly forced.
type TextBackend struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

// NewTextBackend attaches a TCP connection to a new TextBackend.
func NewTextBackend(conn net.Conn) *TextBackend {
	return &TextBackend{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

// ensureNewline ensures commands sent to IIOD always end with \n.
func ensureNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

// readLineStrict reads a full line and trims \r\n.
func (tb *TextBackend) readLineStrict(ctx context.Context) (string, error) {
	tb.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	line, err := tb.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// readUntilEOF reads all available data until the server closes the stream.
// Used mainly for PRINT output, which ends with EOF.
func (tb *TextBackend) readUntilEOF(ctx context.Context) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 4096)

	for {
		n, err := tb.reader.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// Backend interface implementation
///////////////////////////////////////////////////////////////////////////////////////////////////

func (tb *TextBackend) GetXMLContext(ctx context.Context) (string, error) {
	// PlutoSDR uses PRINT <device> <attribute> — but "PRINT" alone dumps full XML
	cmd := "PRINT"
	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return "", err
	}
	tb.writer.Flush()

	// PRINT ends with the server closing the stream for this response
	xmlStr, err := tb.readUntilEOF(ctx)
	if err != nil {
		return "", fmt.Errorf("PRINT read failed: %w", err)
	}

	// Some servers include leading garbage or BOM; trim until we hit '<'
	idx := strings.Index(xmlStr, "<")
	if idx > 0 {
		xmlStr = xmlStr[idx:]
	}
	return xmlStr, nil
}

func (tb *TextBackend) ReadAttr(ctx context.Context, device string, channel string, attr string) (string, error) {
	var cmd string

	if channel == "" {
		cmd = fmt.Sprintf("READ %s %s", device, attr)
	} else {
		cmd = fmt.Sprintf("READ %s %s %s", device, channel, attr)
	}

	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return "", err
	}
	tb.writer.Flush()

	// Reply is exactly 1 line containing the attribute value.
	line, err := tb.readLineStrict(ctx)
	if err != nil {
		return "", err
	}
	return line, nil
}

func (tb *TextBackend) WriteAttr(ctx context.Context, device string, channel string, attr string, value string) error {
	var cmd string

	if channel == "" {
		cmd = fmt.Sprintf("WRITE %s %s %s", device, attr, value)
	} else {
		cmd = fmt.Sprintf("WRITE %s %s %s %s", device, channel, attr, value)
	}

	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return err
	}
	tb.writer.Flush()

	// Expect "OK"
	reply, err := tb.readLineStrict(ctx)
	if err != nil {
		return err
	}

	if reply != "OK" {
		return fmt.Errorf("text WRITE failed: %s", reply)
	}
	return nil
}

func (tb *TextBackend) ListDevices(ctx context.Context) ([]string, error) {
	_, err := tb.writer.WriteString("LISTDEVICES\n")
	if err != nil {
		return nil, err
	}
	tb.writer.Flush()

	line, err := tb.readLineStrict(ctx)
	if err != nil {
		return nil, err
	}
	if line == "" {
		return []string{}, nil
	}

	return strings.Fields(line), nil
}

func (tb *TextBackend) GetChannels(ctx context.Context, device string) ([]string, error) {
	_, err := tb.writer.WriteString(fmt.Sprintf("LISTCHANNELS %s\n", device))
	if err != nil {
		return nil, err
	}
	tb.writer.Flush()

	line, err := tb.readLineStrict(ctx)
	if err != nil {
		return nil, err
	}

	if line == "" {
		return []string{}, nil
	}
	return strings.Fields(line), nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// Buffer operations (Pluto only supports limited text buffer features)
///////////////////////////////////////////////////////////////////////////////////////////////////

func (tb *TextBackend) OpenBuffer(ctx context.Context, device string, samples int) (int, error) {
	cmd := fmt.Sprintf("BUFFER_OPEN %s %d", device, samples)
	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return -1, err
	}
	tb.writer.Flush()

	reply, err := tb.readLineStrict(ctx)
	if err != nil {
		return -1, err
	}

	var id int
	_, err = fmt.Sscanf(reply, "%d", &id)
	if err != nil {
		return -1, fmt.Errorf("invalid buffer id: %s", reply)
	}

	return id, nil
}

func (tb *TextBackend) ReadBuffer(ctx context.Context, bufID int, nBytes int) ([]byte, error) {
	cmd := fmt.Sprintf("BUFFER_READ %d %d", bufID, nBytes)
	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return nil, err
	}
	tb.writer.Flush()

	// IIOD text streaming format = binary payload followed by newline
	tb.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	raw := make([]byte, nBytes)
	_, err = io.ReadFull(tb.reader, raw)
	if err != nil {
		return nil, err
	}

	// Consume trailing newline
	tb.reader.ReadString('\n')
	return raw, nil
}

func (tb *TextBackend) WriteBuffer(ctx context.Context, bufID int, data []byte) (int, error) {
	cmd := fmt.Sprintf("BUFFER_WRITE %d %d", bufID, len(data))
	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return 0, err
	}
	tb.writer.Flush()

	_, err = tb.writer.Write(data)
	if err != nil {
		return 0, err
	}
	tb.writer.WriteByte('\n')
	tb.writer.Flush()

	reply, err := tb.readLineStrict(ctx)
	if err != nil {
		return 0, err
	}

	var written int
	fmt.Sscanf(reply, "%d", &written)
	return written, nil
}

func (tb *TextBackend) CloseBuffer(ctx context.Context, bufID int) error {
	cmd := fmt.Sprintf("BUFFER_CLOSE %d", bufID)
	_, err := tb.writer.WriteString(ensureNewline(cmd))
	if err != nil {
		return err
	}
	tb.writer.Flush()

	reply, err := tb.readLineStrict(ctx)
	if err != nil {
		return err
	}
	if reply != "OK" {
		return fmt.Errorf("close buffer: %s", reply)
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////
// Shutdown
///////////////////////////////////////////////////////////////////////////////////////////////////

func (tb *TextBackend) Close() error {
	return tb.conn.Close()
}
