package connectionmgr

import (
	"encoding/binary"
	"fmt"
	"time"
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

// ErrIiodStatus surfaces status codes carried in IIOD responses (including negative errno values).
type ErrIiodStatus struct {
	Op   uint8
	Dev  uint8
	Code int32
}

func (e ErrIiodStatus) Error() string {
	return fmt.Sprintf("iiod status op=0x%02x dev=%d code=%d", e.Op, e.Dev, e.Code)
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

// DisableBinaryRXBuffer issues DISABLE_BUFFER
func (m *Manager) DisableBinaryRXBuffer(dev uint8) error {
	resp, _, err := m.roundTripBinary(
		opDisableBuffer,
		dev,
		1,
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("DISABLE_BUFFER failed: %d", resp.Code)
	}
	return nil
}

// FreeBinaryRXBuffer issues FREE_BUFFER
func (m *Manager) FreeBinaryRXBuffer(dev uint8) error {
	resp, _, err := m.roundTripBinary(
		opFreeBuffer,
		dev,
		1,
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("FREE_BUFFER failed: %d", resp.Code)
	}
	return nil
}

// CreateBinaryTXBuffer mirrors CreateBinaryRXBuffer for transmit paths.
func (m *Manager) CreateBinaryTXBuffer(dev uint8, samples int) error {
	return m.CreateBinaryRXBuffer(dev, samples)
}

// EnableBinaryTXBuffer mirrors EnableBinaryRXBuffer for transmit paths.
func (m *Manager) EnableBinaryTXBuffer(dev uint8) error {
	return m.EnableBinaryRXBuffer(dev)
}

// DisableBinaryTXBuffer mirrors DisableBinaryRXBuffer for transmit paths.
func (m *Manager) DisableBinaryTXBuffer(dev uint8) error {
	return m.DisableBinaryRXBuffer(dev)
}

// FreeBinaryTXBuffer mirrors FreeBinaryRXBuffer for transmit paths.
func (m *Manager) FreeBinaryTXBuffer(dev uint8) error {
	return m.FreeBinaryRXBuffer(dev)
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

// CreateBinaryTXBlock mirrors CreateBinaryRXBlock for transmit paths.
func (m *Manager) CreateBinaryTXBlock(dev uint8, blockSize int) error {
	return m.CreateBinaryRXBlock(dev, blockSize)
}

// BinaryTransferResult describes a parsed TRANSFER_BLOCK response.
type BinaryTransferResult struct {
	Payload    []byte
	StatusCode int32
	StatusOnly bool
}

// TransferBinaryRXBlock performs one TRANSFER_BLOCK and returns structured results with retry on short reads.
func (m *Manager) TransferBinaryRXBlock(
	dev uint8,
	blockSize int,
	buf []byte,
) (*BinaryTransferResult, error) {

	if cap(buf) < blockSize {
		buf = make([]byte, blockSize)
	}
	buf = buf[:blockSize]

	var (
		resp   iiodCommand
		copied int
		err    error
	)

	retries := 3
	backoff := 5 * time.Millisecond

	for attempt := 0; attempt <= retries; attempt++ {
		resp, copied, err = m.roundTripBinary(
			opTransferBlock,
			dev,
			int32(blockSize),
			nil,
			buf,
		)
		if err == nil && (int(resp.Code) == copied || resp.Code <= 0) {
			break
		}

		if attempt == retries {
			_ = m.DisableBinaryRXBuffer(dev)
			return nil, fmt.Errorf("TRANSFER_BLOCK short read after retries: %w", err)
		}
		time.Sleep(time.Duration(attempt+1) * backoff)
	}

	if resp.Code < 0 {
		return nil, ErrIiodStatus{Op: resp.Op, Dev: resp.Dev, Code: resp.Code}
	}

	result := &BinaryTransferResult{StatusCode: resp.Code, StatusOnly: resp.Code == 0}
	if resp.Code > 0 {
		used := copied
		if used > len(buf) {
			used = len(buf)
		}
		result.Payload = append([]byte(nil), buf[:used]...)
	}

	return result, nil
}

// TransferBinaryTXBlock performs one TRANSFER_BLOCK for transmit and returns status feedback.
func (m *Manager) TransferBinaryTXBlock(
	dev uint8,
	payload []byte,
) (*BinaryTransferResult, error) {
	size := len(payload)
	resp, copied, err := m.roundTripBinary(
		opTransferBlock,
		dev,
		int32(size),
		[][]byte{payload},
		nil,
	)
	if err != nil {
		return nil, err
	}
	if resp.Code < 0 {
		return nil, ErrIiodStatus{Op: resp.Op, Dev: resp.Dev, Code: resp.Code}
	}

	return &BinaryTransferResult{
		Payload:    payload[:copied],
		StatusCode: resp.Code,
		StatusOnly: resp.Code == 0,
	}, nil
}
