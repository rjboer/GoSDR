package connectionmgr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// iiodCommand is the 8-byte header used by libiio's binary protocol.
// It mirrors (layout-wise) the internal struct used by the iiod responder:
//
//	uint8_t  op;        // operation code (IIOD_OP_*)
//	uint8_t  dev;       // device index (or 0 for "no device")
//	uint16_t client_id; // logical buffer/client index
//	int32_t  code;      // length, timeout, or status/error (op-specific)
//
// The actual opcode values and code semantics are defined by the daemon
// (see ops.c / iiod-responder.c on the C side).
type iiodCommand struct {
	Op       uint8
	Dev      uint8
	ClientID uint16
	Code     int32
}

// maxBinaryPayload is a safety limit for response payload sizes.
const maxBinaryPayload = 16 * 1024 * 1024 // 16 MiB

var (
	errNotConnected = errors.New("iiod: not connected")
	errNotBinary    = errors.New("iiod: connection not in binary mode")
)

// Detect host endianness once.
var hostIsLittleEndian = func() bool {
	var x uint16 = 0x1
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], x)
	return b[0] == 0x1
}()

// encode serialises the command into the 8-byte on-wire representation.
func (c iiodCommand) encode() [8]byte {
	var h [8]byte
	h[0] = c.Op
	h[1] = c.Dev

	if hostIsLittleEndian {
		binary.LittleEndian.PutUint16(h[2:4], c.ClientID)
		binary.LittleEndian.PutUint32(h[4:8], uint32(c.Code))
	} else {
		binary.BigEndian.PutUint16(h[2:4], c.ClientID)
		binary.BigEndian.PutUint32(h[4:8], uint32(c.Code))
	}
	return h
}

// decodeCommand parses an 8-byte header into a command struct.
func decodeCommand(hdr []byte) (iiodCommand, error) {
	if len(hdr) != 8 {
		return iiodCommand{}, fmt.Errorf("decodeCommand: invalid header length %d", len(hdr))
	}

	c := iiodCommand{
		Op:  hdr[0],
		Dev: hdr[1],
	}

	if hostIsLittleEndian {
		c.ClientID = binary.LittleEndian.Uint16(hdr[2:4])
		c.Code = int32(binary.LittleEndian.Uint32(hdr[4:8]))
	} else {
		c.ClientID = binary.BigEndian.Uint16(hdr[2:4])
		c.Code = int32(binary.BigEndian.Uint32(hdr[4:8]))
	}

	return c, nil
}

// sendBinaryCommand writes a command header plus optional payload.
// For most "write" operations, Code should be the payload length.
func (m *Manager) sendBinaryCommand(c iiodCommand, payload []byte) error {
	if m == nil || m.conn == nil {
		return errNotConnected
	}
	if m.Mode != ModeBinary {
		return errNotBinary
	}

	hdr := c.encode()
	if err := m.writeAll(hdr[:]); err != nil {
		return fmt.Errorf("sendBinaryCommand: write header: %w", err)
	}

	if len(payload) > 0 {
		if err := m.writeAll(payload); err != nil {
			return fmt.Errorf("sendBinaryCommand: write payload: %w", err)
		}
	}
	return nil
}

// recvBinaryResponse reads one response header and, optionally, a payload.
//
// Semantics:
//
//	Code < 0  → error from daemon; no payload is read.
//	Code == 0 → success, no payload.
//	Code > 0  → success, Code is payload length in bytes.
//
// If expectPayload=false and Code>0, the payload is read and discarded; only
// the header (and thus status) is returned.
func (m *Manager) recvBinaryResponse(expectPayload bool) (iiodCommand, []byte, error) {
	if m == nil || m.conn == nil {
		return iiodCommand{}, nil, errNotConnected
	}
	if m.Mode != ModeBinary {
		return iiodCommand{}, nil, errNotBinary
	}

	var hdr [8]byte
	m.applyReadDeadline()
	if _, err := io.ReadFull(m.reader, hdr[:]); err != nil {
		return iiodCommand{}, nil, fmt.Errorf("recvBinaryResponse: read header: %w", err)
	}

	cmd, err := decodeCommand(hdr[:])
	if err != nil {
		return iiodCommand{}, nil, err
	}

	if cmd.Code < 0 {
		// Negative values are errno-style error codes from the daemon.
		return cmd, nil, fmt.Errorf("iiod binary error: code=%d", cmd.Code)
	}

	if cmd.Code == 0 {
		// No payload
		return cmd, nil, nil
	}

	length := int(cmd.Code)
	if length < 0 || length > maxBinaryPayload {
		return cmd, nil, fmt.Errorf("recvBinaryResponse: suspicious payload length %d", length)
	}

	if !expectPayload {
		// Caller isn't interested in the payload bytes; drain them.
		if err := m.discardN(length); err != nil {
			return cmd, nil, fmt.Errorf("recvBinaryResponse: discard payload: %w", err)
		}
		return cmd, nil, nil
	}

	buf := make([]byte, length)
	if err := m.readAll(buf); err != nil {
		return cmd, nil, fmt.Errorf("recvBinaryResponse: read payload: %w", err)
	}

	return cmd, buf, nil
}

// discardN drains exactly n bytes from the underlying stream.
func (m *Manager) discardN(n int) error {
	if n <= 0 {
		return nil
	}
	if m == nil || m.conn == nil {
		return errNotConnected
	}

	remaining := n
	tmp := make([]byte, 4096)

	for remaining > 0 {
		chunk := remaining
		if chunk > len(tmp) {
			chunk = len(tmp)
		}

		m.applyReadDeadline()
		read, err := io.ReadFull(m.reader, tmp[:chunk])
		if err != nil {
			return err
		}
		remaining -= read
	}
	return nil
}

// BinaryExecSimple sends a command without payload and expects only a status.
// Example use cases: TIMEOUT, ENABLE_BUFFER, DISABLE_BUFFER, etc.
//
// It returns cmd.Code from the response (normally 0 on success).
func (m *Manager) BinaryExecSimple(op, dev uint8, code int32) (int32, error) {
	cmd := iiodCommand{
		Op:       op,
		Dev:      dev,
		ClientID: 0,
		Code:     code,
	}
	if err := m.sendBinaryCommand(cmd, nil); err != nil {
		return 0, err
	}

	resp, _, err := m.recvBinaryResponse(false)
	if err != nil {
		return 0, err
	}
	return resp.Code, nil
}

// BinaryExecWrite sends a command with a payload and expects a status-only
// response. Typical example: attribute write, enqueue buffer, etc.
func (m *Manager) BinaryExecWrite(op, dev uint8, payload []byte) (int32, error) {
	cmd := iiodCommand{
		Op:       op,
		Dev:      dev,
		ClientID: 0,
		Code:     int32(len(payload)),
	}
	if err := m.sendBinaryCommand(cmd, payload); err != nil {
		return 0, err
	}

	resp, _, err := m.recvBinaryResponse(false)
	if err != nil {
		return 0, err
	}
	return resp.Code, nil
}

// BinaryExecRead sends a command without payload and returns the response
// payload. Examples: PRINT, READ_ATTR, READ_DBG_ATTR, READ_BUF, etc.
func (m *Manager) BinaryExecRead(op, dev uint8) ([]byte, error) {
	cmd := iiodCommand{
		Op:       op,
		Dev:      dev,
		ClientID: 0,
		Code:     0,
	}
	if err := m.sendBinaryCommand(cmd, nil); err != nil {
		return nil, err
	}

	_, payload, err := m.recvBinaryResponse(true)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

// -----------------------------------------------------------------------------
// Legacy-style helpers retained for experimentation
// -----------------------------------------------------------------------------

// SendBinary is a generic "write style" helper. It sets Code=len(payload) and
// sends header + payload. Prefer the BinaryExec* helpers for real use cases.
func (m *Manager) SendBinary(op uint8, payload []byte) error {
	cmd := iiodCommand{
		Op:       op,
		Dev:      0,
		ClientID: 0,
		Code:     int32(len(payload)),
	}
	return m.sendBinaryCommand(cmd, payload)
}

// ReadBinary reads the next binary response on the wire and returns its
// status (Code) and payload (if any). Any negative error code from the
// daemon is converted to a Go error.
func (m *Manager) ReadBinary() (status int32, data []byte, err error) {
	resp, payload, err := m.recvBinaryResponse(true)
	if err != nil {
		return 0, nil, err
	}
	return resp.Code, payload, nil
}
