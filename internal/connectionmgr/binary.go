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
// IIOD binary opcodes (see iiod-responder.h)
// =======================
const (
	opResponse           = 0x00
	opCreateBuffer       = 0x0d
	opFreeBuffer         = 0x0e
	opEnableBuffer       = 0x0f
	opDisableBuffer      = 0x10
	opCreateBlock        = 0x11
	opFreeBlock          = 0x12
	opTransferBlock      = 0x13
	opEnqueueBlockCyclic = 0x14
	opRetryDequeueBlock  = 0x15
)

//
// =======================
// Low-level primitives
// =======================
//

// sendBinaryCommand writes exactly one binary command header followed by any payload slices.
func (m *Manager) sendBinaryCommand(op, dev uint8, code int32, payloads ...[]byte) error {
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

	if err := m.writeAll(hdr[:]); err != nil {
		return err
	}

	for _, payload := range payloads {
		if len(payload) == 0 {
			continue
		}
		if err := m.writeAll(payload); err != nil {
			return err
		}
	}

	return nil
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

// roundTripBinary sends a command (with optional payload slices), receives a response header,
// and reads up to len(respPayload) bytes from the response payload, discarding any overflow.
func (m *Manager) roundTripBinary(
	op, dev uint8,
	code int32,
	cmdPayload [][]byte,
	respPayload []byte,
) (iiodCommand, int, error) {

	if err := m.sendBinaryCommand(op, dev, code, cmdPayload...); err != nil {
		return iiodCommand{}, 0, err
	}

	resp, err := m.recvBinaryResponseHeader()
	if err != nil {
		return iiodCommand{}, 0, err
	}

	if resp.Code < 0 {
		return resp, 0, fmt.Errorf(
			"iiod binary error: op=0x%02x dev=%d code=%d",
			op, dev, resp.Code,
		)
	}

	payloadLen := int(resp.Code)
	copied := 0
	if payloadLen > 0 {
		copyLen := payloadLen
		if copyLen > len(respPayload) {
			copyLen = len(respPayload)
		}
		if copyLen > 0 {
			if err := m.readAll(respPayload[:copyLen]); err != nil {
				return resp, copied, err
			}
			copied = copyLen
		}

		remaining := payloadLen - copyLen
		if remaining > 0 {
			if err := m.discardN(remaining); err != nil {
				return resp, copied, err
			}
		}
	}

	return resp, copied, nil
}

//
// =======================
// High-level RX helpers
// =======================
//

// CreateBinaryRXBuffer issues CREATE_BUFFER
func (m *Manager) CreateBinaryRXBuffer(dev uint8, samples int) error {
	resp, _, err := m.roundTripBinary(
		opCreateBuffer,
		dev,
		int32(samples),
		nil,
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
	resp, _, err := m.roundTripBinary(
		opEnableBuffer,
		dev,
		1,
		nil,
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
	resp, _, err := m.roundTripBinary(
		opCreateBlock,
		dev,
		int32(blockSize),
		nil,
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

	resp, _, err := m.roundTripBinary(
		opTransferBlock,
		dev,
		int32(blockSize),
		nil,
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
