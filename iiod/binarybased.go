package iiod

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// ----------------------------------------------------------------------
// IIOD Binary Protocol Constants
// ----------------------------------------------------------------------

const (
	opVersion     = 0 // VERSION
	opPrint       = 1 // PRINT (XML)
	opListDevices = 2 // LIST_DEVICES
	opOpenBuffer  = 3
	opCloseBuffer = 4
	opReadBuf     = 5
	opWriteBuf    = 6
	opAttr        = 7 // READ/WRITE ATTR
	opDevAttr     = 8
)

// Response structure:
//   uint32 status
//   uint32 length
//   <payload bytes>

// Status 0 = OK
// All others = error codes

// ----------------------------------------------------------------------
// BinaryBackend
// ----------------------------------------------------------------------

type BinaryBackend struct {
	conn net.Conn
}

func NewBinaryBackend(conn net.Conn) *BinaryBackend {
	return &BinaryBackend{conn: conn}
}

// ----------------------------------------------------------------------
// Helper send/recv framing
// ----------------------------------------------------------------------

func (bb *BinaryBackend) send(cmd []byte) error {
	_, err := bb.conn.Write(cmd)
	return err
}

func (bb *BinaryBackend) recvResponse() (status uint32, payload []byte, err error) {
	header := make([]byte, 8)
	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err = io.ReadFull(bb.conn, header); err != nil {
		return 0, nil, fmt.Errorf("read header: %w", err)
	}

	status = binary.LittleEndian.Uint32(header[0:4])
	length := binary.LittleEndian.Uint32(header[4:8])

	if length > 32*1024*1024 {
		return 0, nil, fmt.Errorf("payload too large: %d", length)
	}

	payload = make([]byte, length)
	if _, err = io.ReadFull(bb.conn, payload); err != nil {
		return 0, nil, fmt.Errorf("read payload: %w", err)
	}

	return status, payload, nil
}

// ----------------------------------------------------------------------
// Probe – binary-first detection
// ----------------------------------------------------------------------

func (bb *BinaryBackend) Probe(ctx context.Context, conn net.Conn) error {
	// Build “PRINT” command
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opPrint)) // op
	binary.Write(&buf, binary.LittleEndian, uint32(0))       // reserved
	binary.Write(&buf, binary.LittleEndian, uint32(0))       // dev
	binary.Write(&buf, binary.LittleEndian, uint32(0))       // code
	binary.Write(&buf, binary.LittleEndian, uint32(0))       // payload length

	if err := bb.send(buf.Bytes()); err != nil {
		return fmt.Errorf("binary probe send: %w", err)
	}

	status, payload, err := bb.recvResponse()
	if err != nil {
		return fmt.Errorf("binary probe recv: %w", err)
	}

	if status != 0 {
		return fmt.Errorf("binary probe status=%d", status)
	}

	if !bytes.Contains(payload, []byte("<context")) {
		return fmt.Errorf("binary PRINT did not return XML header")
	}

	return nil
}

// ----------------------------------------------------------------------
// Get XML Context
// ----------------------------------------------------------------------

func (bb *BinaryBackend) GetXMLContext(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opPrint))
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // dev
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // code
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // no payload

	if err := bb.send(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("send PRINT: %w", err)
	}

	status, payload, err := bb.recvResponse()
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("PRINT error: %d", status)
	}

	return payload, nil
}

// ----------------------------------------------------------------------
// List Devices
// ----------------------------------------------------------------------

func (bb *BinaryBackend) ListDevices(ctx context.Context) ([]string, error) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opListDevices))
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // dev
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // code
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // no payload

	if err := bb.send(buf.Bytes()); err != nil {
		return nil, err
	}

	status, payload, err := bb.recvResponse()
	if err != nil {
		return nil, err
	}
	if status != 0 {
		return nil, fmt.Errorf("LIST_DEVICES failed: %d", status)
	}

	// Null-terminated names
	parts := bytes.Split(payload, []byte{0})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := string(bytes.TrimSpace(p))
		if s != "" {
			out = append(out, s)
		}
	}

	return out, nil
}

// ----------------------------------------------------------------------
// GetChannels – binary LIST_DEVICES does NOT include channels.
// Must parse XML.
//
// Binary protocol *never* exposes channel lists.
// ----------------------------------------------------------------------

func (bb *BinaryBackend) GetChannels(ctx context.Context, dev string) ([]string, error) {
	xmlBytes, err := bb.GetXMLContext(ctx)
	if err != nil {
		return nil, err
	}
	parsed, err := ParseIIODXML(xmlBytes)
	if err != nil {
		return nil, err
	}

	for _, d := range parsed.Device {
		if d.ID == dev {
			channels := make([]string, 0, len(d.Channel))
			for _, ch := range d.Channel {
				channels = append(channels, ch.ID)
			}
			return channels, nil
		}
	}
	return nil, fmt.Errorf("device %q not found in XML", dev)
}

// ----------------------------------------------------------------------
// Attribute Read/Write
//
// op=7 (ATTR)
// payload format:
//   <attr string>  (null terminated)
// ----------------------------------------------------------------------

func (bb *BinaryBackend) ReadAttr(ctx context.Context, dev, ch, attr string) (string, error) {
	payload := bb.buildAttrPayload(dev, ch, attr, "")

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opAttr))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(len(payload)))
	buf.Write(payload)

	if err := bb.send(buf.Bytes()); err != nil {
		return "", err
	}
	status, out, err := bb.recvResponse()
	if err != nil {
		return "", err
	}
	if status != 0 {
		return "", fmt.Errorf("ATTR read status=%d", status)
	}

	return string(out), nil
}

func (bb *BinaryBackend) WriteAttr(ctx context.Context, dev, ch, attr, value string) error {
	payload := bb.buildAttrPayload(dev, ch, attr, value)

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opAttr))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(len(payload)))
	buf.Write(payload)

	if err := bb.send(buf.Bytes()); err != nil {
		return err
	}

	status, _, err := bb.recvResponse()
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("ATTR write status=%d", status)
	}

	return nil
}

// ----------------------------------------------------------------------
// Attribute Payload Builder
// (device \0 channel \0 attr \0 [value\0])
// ----------------------------------------------------------------------

func (bb *BinaryBackend) buildAttrPayload(dev, ch, attr, value string) []byte {
	buf := bytes.NewBuffer(nil)
	buf.WriteString(dev)
	buf.WriteByte(0)
	if ch != "" {
		buf.WriteString(ch)
	} else {
		buf.WriteString("-")
	}
	buf.WriteByte(0)
	buf.WriteString(attr)
	buf.WriteByte(0)
	if value != "" {
		buf.WriteString(value)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// ----------------------------------------------------------------------
// Buffer Handling
// ----------------------------------------------------------------------

func (bb *BinaryBackend) OpenBuffer(ctx context.Context, dev string, samples int, cyclic bool) (int, error) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opOpenBuffer))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	// payload = sample count + cyclic flag + dev string
	payload := bytes.NewBuffer(nil)
	binary.Write(payload, binary.LittleEndian, uint32(samples))
	binary.Write(payload, binary.LittleEndian, uint32(boolToUint(cyclic)))
	payload.WriteString(dev)
	payload.WriteByte(0)

	binary.Write(&buf, binary.LittleEndian, uint32(payload.Len()))
	buf.Write(payload.Bytes())

	if err := bb.send(buf.Bytes()); err != nil {
		return 0, err
	}
	status, out, err := bb.recvResponse()
	if err != nil {
		return 0, err
	}
	if status != 0 {
		return 0, fmt.Errorf("OPEN_BUFFER status=%d", status)
	}

	if len(out) < 4 {
		return 0, fmt.Errorf("invalid buffer id response")
	}

	bufID := binary.LittleEndian.Uint32(out[0:4])
	return int(bufID), nil
}

func (bb *BinaryBackend) ReadBuffer(ctx context.Context, bufID int, p []byte) (int, error) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opReadBuf))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(bufID))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	if err := bb.send(buf.Bytes()); err != nil {
		return 0, err
	}
	status, payload, err := bb.recvResponse()
	if err != nil {
		return 0, err
	}
	if status != 0 {
		return 0, fmt.Errorf("READBUF status=%d", status)
	}

	n := copy(p, payload)
	return n, nil
}

func (bb *BinaryBackend) WriteBuffer(ctx context.Context, bufID int, data []byte) (int, error) {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opWriteBuf))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(bufID))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(len(data)))
	buf.Write(data)

	if err := bb.send(buf.Bytes()); err != nil {
		return 0, err
	}
	status, _, err := bb.recvResponse()
	if err != nil {
		return 0, err
	}
	if status != 0 {
		return 0, fmt.Errorf("WRITEBUF status=%d", status)
	}

	return len(data), nil
}

func (bb *BinaryBackend) CloseBuffer(ctx context.Context, bufID int) error {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint32(opCloseBuffer))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(bufID))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))

	if err := bb.send(buf.Bytes()); err != nil {
		return err
	}
	status, _, err := bb.recvResponse()
	if err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("CLOSE_BUFFER status=%d", status)
	}

	return nil
}

// ----------------------------------------------------------------------
// Close
// ----------------------------------------------------------------------

func (bb *BinaryBackend) Close() error {
	return nil
}

func boolToUint(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}
