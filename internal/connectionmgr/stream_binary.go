package connectionmgr

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// IIOD binary opcodes (from responder headers)
const (
	opCreateBuffer  = 0x10
	opEnableBuffer  = 0x11
	opCreateBlock   = 0x12
	opTransferBlock = 0x13
)

// iiodCommand mirrors struct iiod_command exactly
type iiodCommand struct {
	ClientID uint16
	Op       uint8
	Dev      uint8
	Code     int32
}

func (m *Manager) writeCommand(cmd iiodCommand) error {
	var buf [8]byte
	binary.BigEndian.PutUint16(buf[0:2], cmd.ClientID)
	buf[2] = cmd.Op
	buf[3] = cmd.Dev
	binary.BigEndian.PutUint32(buf[4:8], uint32(cmd.Code))

	_, err := m.conn.Write(buf[:])
	return err
}

func (m *Manager) readResponseHeader() (iiodCommand, error) {
	var buf [8]byte
	_, err := io.ReadFull(m.conn, buf[:])
	if err != nil {
		return iiodCommand{}, err
	}

	return iiodCommand{
		ClientID: binary.BigEndian.Uint16(buf[0:2]),
		Op:       buf[2],
		Dev:      buf[3],
		Code:     int32(binary.BigEndian.Uint32(buf[4:8])),
	}, nil
}

// CreateBinaryRXBuffer issues CREATE_BUFFER
func (m *Manager) CreateBinaryRXBuffer(dev uint8, samples int) error {
	cmd := iiodCommand{
		ClientID: m.clientID,
		Op:       opCreateBuffer,
		Dev:      dev,
		Code:     int32(samples),
	}

	if err := m.writeCommand(cmd); err != nil {
		return err
	}

	resp, err := m.readResponseHeader()
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
	cmd := iiodCommand{
		ClientID: m.clientID,
		Op:       opEnableBuffer,
		Dev:      dev,
		Code:     1,
	}

	if err := m.writeCommand(cmd); err != nil {
		return err
	}

	resp, err := m.readResponseHeader()
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
	cmd := iiodCommand{
		ClientID: m.clientID,
		Op:       opCreateBlock,
		Dev:      dev,
		Code:     int32(blockSize),
	}

	if err := m.writeCommand(cmd); err != nil {
		return err
	}

	resp, err := m.readResponseHeader()
	if err != nil {
		return err
	}
	if resp.Code < 0 {
		return fmt.Errorf("CREATE_BLOCK failed: %d", resp.Code)
	}
	return nil
}

// TransferBinaryRXBlock performs one TRANSFER_BLOCK and returns raw samples
func (m *Manager) TransferBinaryRXBlock(dev uint8, blockSize int, buf []byte) ([]byte, error) {
	if cap(buf) < blockSize {
		buf = make([]byte, blockSize)
	}
	buf = buf[:blockSize]

	cmd := iiodCommand{
		ClientID: m.clientID,
		Op:       opTransferBlock,
		Dev:      dev,
		Code:     blockSize,
	}

	if err := m.writeCommand(cmd); err != nil {
		return nil, err
	}

	resp, err := m.readResponseHeader()
	if err != nil {
		return nil, err
	}
	if resp.Code < 0 {
		return nil, fmt.Errorf("TRANSFER_BLOCK failed: %d", resp.Code)
	}

	_, err = io.ReadFull(m.conn, buf)
	if err != nil {
		return nil, err
	}

	return buf, nil
}

// Ensure net.Conn is exposed
func (m *Manager) SetConn(conn net.Conn) {
	m.conn = conn
}
