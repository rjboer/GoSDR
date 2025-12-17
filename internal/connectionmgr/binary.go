package connectionmgr

import (
	"encoding/binary"
	"fmt"
)

// =======================
// Binary protocol header
// =======================
//
// Wire format (network / big-endian):
//
//	uint16 client_id
//	uint8  op
//	uint8  dev
//	int32  code
type iiodCommand struct {
	ClientID uint16
	Op       uint8
	Dev      uint8
	Code     int32
}

// =======================
// IIOD binary opcodes
// =======================
const (
	opCreateBuffer  = 0x10
	opEnableBuffer  = 0x11
	opCreateBlock   = 0x12
	opTransferBlock = 0x13
)

//
// =======================
// Low-level primitives
// =======================
//

// sendBinaryCommand writes exactly one binary command header.
func (m *Manager) sendBinaryCommand(op, dev uint8, code int32) error {
	if m == nil || m.conn == nil {
		return fmt.Errorf("sendBinaryCommand: not connected")
	}
	if m.Mode != ModeBinary {
		return fmt.Errorf("sendBinaryCommand: not in binary mode")
	}

	var hdr [8]byte
	binary.BigEndian.PutUint16(hdr[0:2], m.clientID)
	hdr[2] = op
	hdr[3] = dev
	binary.BigEndian.PutUint32(hdr[4:8], uint32(code))

	return m.writeAll(hdr[:])
}

// recvBinaryResponseHeader reads exactly one binary response header.
func (m *Manager) recvBinaryResponseHeader() (iiodCommand, error) {
	var hdr [8]byte
	if err := m.readAll(hdr[:]); err != nil {
		return iiodCommand{}, err
	}

	return iiodCommand{
		ClientID: binary.BigEndian.Uint16(hdr[0:2]),
		Op:       hdr[2],
		Dev:      hdr[3],
		Code:     int32(binary.BigEndian.Uint32(hdr[4:8])),
	}, nil
}

// discardN drains exactly n bytes from the connection.
func (m *Manager) discardN(n int) error {
	if n <= 0 {
		return nil
	}

	tmp := make([]byte, 4096)
	for n > 0 {
		chunk := n
		if chunk > len(tmp) {
			chunk = len(tmp)
		}
		if err := m.readAll(tmp[:chunk]); err != nil {
			return err
		}
		n -= chunk
	}
	return nil
}

// roundTripBinary sends a command, receives a response header,
// and optionally reads a fixed-size payload.
func (m *Manager) roundTripBinary(
	op, dev uint8,
	code int32,
	payloadDst []byte,
) (iiodCommand, error) {

	if err := m.sendBinaryCommand(op, dev, code); err != nil {
		return iiodCommand{}, err
	}

	resp, err := m.recvBinaryResponseHeader()
	if err != nil {
		return iiodCommand{}, err
	}

	if resp.Code < 0 {
		return resp, fmt.Errorf(
			"iiod binary error: op=0x%02x dev=%d code=%d",
			op, dev, resp.Code,
		)
	}

	if len(payloadDst) > 0 {
		if err := m.readAll(payloadDst); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

//
// =======================
// High-level RX helpers
// =======================
//

// CreateBinaryRXBuffer issues CREATE_BUFFER
func (m *Manager) CreateBinaryRXBuffer(dev uint8, samples int) error {
	resp, err := m.roundTripBinary(
		opCreateBuffer,
		dev,
		int32(samples),
		nil,
	)
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("CREATE_BUFFER failed: %d", resp.Code)
	}
	return nil
}

// EnableBinaryRXBuffer issues ENABLE_BUFFER
func (m *Manager) EnableBinaryRXBuffer(dev uint8) error {
	resp, err := m.roundTripBinary(
		opEnableBuffer,
		dev,
		1,
		nil,
	)
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("ENABLE_BUFFER failed: %d", resp.Code)
	}
	return nil
}

// CreateBinaryRXBlock issues CREATE_BLOCK
func (m *Manager) CreateBinaryRXBlock(dev uint8, blockSize int) error {
	resp, err := m.roundTripBinary(
		opCreateBlock,
		dev,
		int32(blockSize),
		nil,
	)
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("CREATE_BLOCK failed: %d", resp.Code)
	}
	return nil
}

// TransferBinaryRXBlock performs one TRANSFER_BLOCK and returns raw samples.
func (m *Manager) TransferBinaryRXBlock(
	dev uint8,
	blockSize int,
	buf []byte,
) ([]byte, error) {

	if cap(buf) < blockSize {
		buf = make([]byte, blockSize)
	}
	buf = buf[:blockSize]

	resp, err := m.roundTripBinary(
		opTransferBlock,
		dev,
		int32(blockSize),
		buf,
	)
	if err != nil {
		return nil, err
	}
	if resp.Code < 0 {
		return nil, fmt.Errorf("TRANSFER_BLOCK failed: %d", resp.Code)
	}

	return buf, nil
}
