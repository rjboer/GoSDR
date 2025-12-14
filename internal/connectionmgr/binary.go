package connectionmgr

import (
	"encoding/binary"
	"fmt"
	"io"
)

// NOTE: This file implements a *custom* binary framing:
//
//   Request:  [opcode:1][len:4][payload...]
//   Response: [status:4][len:4][payload...]
//
// This is NOT the same as libiio's official binary protocol, which is built on
// iiod_responder + iiod_io + IIOD_OP_* command structs in C. :contentReference[oaicite:10]{index=10}
// Use this only if you control both client and server; it will not talk to a
// stock IIOD daemon.

func (m *Manager) SendBinary(opcode uint8, payload []byte) error {
	if m.Mode != ModeBinary {
		return fmt.Errorf("SendBinary: not in binary mode (Mode=%v)", m.Mode)
	}
	if m.conn == nil {
		return fmt.Errorf("SendBinary: not connected")
	}

	header := make([]byte, 5)
	header[0] = opcode
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)))

	if err := m.writeAll(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if err := m.writeAll(payload); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) ReadBinary() (status int32, data []byte, err error) {
	if m.Mode != ModeBinary {
		return 0, nil, fmt.Errorf("ReadBinary: not in binary mode (Mode=%v)", m.Mode)
	}
	if m.conn == nil {
		return 0, nil, fmt.Errorf("ReadBinary: not connected")
	}

	header := make([]byte, 8)
	m.applyReadDeadline()
	if _, err = io.ReadFull(m.reader, header); err != nil {
		return 0, nil, err
	}
	status = int32(binary.BigEndian.Uint32(header[0:4]))
	length := binary.BigEndian.Uint32(header[4:8])

	if length == 0 {
		return status, nil, nil
	}
	data = make([]byte, length)
	if _, err = io.ReadFull(m.reader, data); err != nil {
		return 0, nil, err
	}
	return status, data, nil
}
