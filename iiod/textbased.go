package iiod

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// ----------------------------------------------------------------------
// TextBackend implements Backend for legacy Pluto-style IIOD
// ----------------------------------------------------------------------

type TextBackend struct {
	conn   net.Conn
	reader *bufio.Reader
}

func NewTextBackend(conn net.Conn) *TextBackend {
	return &TextBackend{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// ----------------------------------------------------------------------
// Probe for Pluto IIOD text mode
// ----------------------------------------------------------------------

func (tb *TextBackend) Probe(ctx context.Context, conn net.Conn) error {
	// Pluto accepts "PRINT\n" and replies with XML
	if err := tb.sendLine("PRINT"); err != nil {
		return fmt.Errorf("text probe PRINT failed: %w", err)
	}

	// Pluto responds with XML header "<context"
	line, err := tb.readUntilStartXML()
	if err != nil {
		return fmt.Errorf("text probe read failed: %w", err)
	}

	if !bytes.Contains(line, []byte("<context")) {
		return fmt.Errorf("text probe invalid response (expected <context>)")
	}

	return nil
}

// ----------------------------------------------------------------------
// Helper: send a single line
// ----------------------------------------------------------------------

func (tb *TextBackend) sendLine(s string) error {
	_, err := io.WriteString(tb.conn, s+"\n")
	return err
}

// ----------------------------------------------------------------------
// Helper: read until the start of XML
// ----------------------------------------------------------------------

func (tb *TextBackend) readUntilStartXML() ([]byte, error) {
	tb.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var buf bytes.Buffer
	for {
		line, err := tb.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		buf.Write(line)

		if bytes.Contains(line, []byte("<context")) {
			return buf.Bytes(), nil
		}
	}
}

// ----------------------------------------------------------------------
// Get XML Context
// ----------------------------------------------------------------------

func (tb *TextBackend) GetXMLContext(ctx context.Context) ([]byte, error) {
	if err := tb.sendLine("PRINT"); err != nil {
		return nil, fmt.Errorf("text PRINT failed: %w", err)
	}

	// Capture until "</context>"
	var out bytes.Buffer

	for {
		tb.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, err := tb.reader.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("reading XML: %w", err)
		}

		out.Write(line)

		if bytes.Contains(line, []byte("</context>")) {
			break
		}
	}

	return out.Bytes(), nil
}

// ----------------------------------------------------------------------
// List Devices (text mode does not provide this directly)
// Use XML context as the only valid source
// ----------------------------------------------------------------------

func (tb *TextBackend) ListDevices(ctx context.Context) ([]string, error) {
	xmlBytes, err := tb.GetXMLContext(ctx)
	if err != nil {
		return nil, err
	}

	ctxParsed, err := ParseIIODXML(xmlBytes)
	if err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	var devs []string
	for _, d := range ctxParsed.Device {
		devs = append(devs, d.ID)
	}
	return devs, nil
}

// ----------------------------------------------------------------------
// Get Channels (text mode => parse from XML)
// ----------------------------------------------------------------------

func (tb *TextBackend) GetChannels(ctx context.Context, dev string) ([]string, error) {
	xmlBytes, err := tb.GetXMLContext(ctx)
	if err != nil {
		return nil, err
	}

	ctxParsed, err := ParseIIODXML(xmlBytes)
	if err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	for _, d := range ctxParsed.Device {
		if d.ID == dev {
			channels := make([]string, 0, len(d.Channel))
			for _, ch := range d.Channel {
				channels = append(channels, ch.ID)
			}
			return channels, nil
		}
	}

	return nil, fmt.Errorf("device %q not found", dev)
}

// ----------------------------------------------------------------------
// Attribute Access: READ
// Syntax per Pluto:
//   GET <device> <channel|-> <attr>
// Response = single line containing value
// ----------------------------------------------------------------------

func (tb *TextBackend) ReadAttr(ctx context.Context, dev, ch, attr string) (string, error) {
	target := ch
	if target == "" {
		target = "-"
	}

	cmd := fmt.Sprintf("GET %s %s %s", dev, target, attr)
	if err := tb.sendLine(cmd); err != nil {
		return "", err
	}

	tb.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	line, err := tb.reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read GET: %w", err)
	}

	return strings.TrimSpace(line), nil
}

// ----------------------------------------------------------------------
// Attribute Access: WRITE
// Syntax per Pluto:
//   SET <device> <channel|-> <attr> <value>
// Response: "OK"
// ----------------------------------------------------------------------

func (tb *TextBackend) WriteAttr(ctx context.Context, dev, ch, attr, value string) error {
	target := ch
	if target == "" {
		target = "-"
	}

	cmd := fmt.Sprintf("SET %s %s %s %s", dev, target, attr, value)
	if err := tb.sendLine(cmd); err != nil {
		return err
	}

	tb.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp, err := tb.reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("write SET: %w", err)
	}

	if !strings.HasPrefix(resp, "OK") {
		return fmt.Errorf("SET returned error: %s", strings.TrimSpace(resp))
	}

	return nil
}

// ----------------------------------------------------------------------
// Buffers (Pluto text protocol uses sysfs buffer commands)
// Minimal implementation using XML names
// ----------------------------------------------------------------------

func (tb *TextBackend) OpenBuffer(ctx context.Context, dev string, samples int, cyclic bool) (int, error) {
	// Pluto has no real “buffer IDs” in text API.
	// We treat text buffers as ID = 0 always.
	return 0, nil
}

func (tb *TextBackend) ReadBuffer(ctx context.Context, bufID int, p []byte) (int, error) {
	return 0, errors.New("Pluto text mode does not support remote buffer read")
}

func (tb *TextBackend) WriteBuffer(ctx context.Context, bufID int, p []byte) (int, error) {
	return 0, errors.New("Pluto text mode does not support remote buffer write")
}

func (tb *TextBackend) CloseBuffer(ctx context.Context, bufID int) error {
	return nil
}

// ----------------------------------------------------------------------
// Close backend
// ----------------------------------------------------------------------

func (tb *TextBackend) Close() error {
	return nil // no special cleanup needed
}
