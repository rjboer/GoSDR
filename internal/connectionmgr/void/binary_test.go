package connectionmgr

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestTransferBinaryRXBlockStatusOnly(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		// TRANSFER_BLOCK request
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read header: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = 1
		binary.BigEndian.PutUint32(resp[4:8], 0)
		_, _ = server.Write(resp[:])
	}()

	res, err := m.TransferBinaryRXBlock(1, 4, nil)
	if err != nil {
		t.Fatalf("TransferBinaryRXBlock: %v", err)
	}
	if !res.StatusOnly || res.StatusCode != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	<-done
}

func TestTransferBinaryRXBlockNegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read header: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = 2
		code := int32(-5)
		binary.BigEndian.PutUint32(resp[4:8], uint32(code))
		_, _ = server.Write(resp[:])
	}()

	_, err := m.TransferBinaryRXBlock(2, 4, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, ok := err.(ErrIiodStatus); !ok {
		t.Fatalf("expected ErrIiodStatus, got %T", err)
	}

	<-done
}

// new code

func (m *Manager) SendReadAttr(dev uint8, code int32, name string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opReadAttr, dev, code, lpString(name))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendReadDbgAttr(dev uint8, code int32, name string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opReadDbgAttr, dev, code, lpString(name))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendReadBufAttr(dev uint8, code int32, name string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opReadBufAttr, dev, code, lpString(name))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendReadChnAttr(dev uint8, code int32, name string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opReadChnAttr, dev, code, lpString(name))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendWriteAttr(dev uint8, code int32, name, value string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opWriteAttr, dev, code, nameValue(name, value))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendWriteDbgAttr(dev uint8, code int32, name, value string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opWriteDbgAttr, dev, code, nameValue(name, value))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendWriteBufAttr(dev uint8, code int32, name, value string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opWriteBufAttr, dev, code, nameValue(name, value))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendWriteChnAttr(dev uint8, code int32, name, value string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opWriteChnAttr, dev, code, nameValue(name, value))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// GETTRIG has no payload.
func (m *Manager) SendGetTrig(dev uint8, code int32) error {
	_, _, err := m.sendBinaryCommand(opGetTrig, dev, code)
	if err != nil {
		return err
	}
	return nil
}

// SETTRIG: payload is trigger name (4+N)
func (m *Manager) SendSetTrig(dev uint8, code int32, trigName string) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opSetTrig, dev, code, lpString(trigName))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// CREATE_BUFFER payload: (count + ids...) + samples_per_block
// Bytes: 4 + 4*C + 4
func (m *Manager) SendCreateBuffer(dev uint8, code int32, channelIDs []uint32, samplesPerBlock uint32) (*BinaryHeader, ResponsePlan, error) {
	part1 := u32SliceWithCount(channelIDs) // 4 + 4*C
	part2 := u32(samplesPerBlock)          // 4
	return m.sendBinaryCommand(opCreateBuffer, dev, code, part1, part2)
}

func (m *Manager) SendFreeBuffer(dev uint8, code int32, bufferID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opFreeBuffer, dev, code, u32(bufferID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendEnableBuffer(dev uint8, code int32, bufferID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opEnableBuffer, dev, code, u32(bufferID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
func (m *Manager) SendDisableBuffer(dev uint8, code int32, bufferID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opDisableBuffer, dev, code, u32(bufferID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// CREATE_BLOCK payload: buffer_id + block_size (8 bytes)
func (m *Manager) SendCreateBlock(dev uint8, code int32, bufferID uint32, blockSize uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opCreateBlock, dev, code, u32(bufferID), u32(blockSize))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendFreeBlock(dev uint8, code int32, blockID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opFreeBlock, dev, code, u32(blockID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// TRANSFER_BLOCK RX (read): payload is block_id (4 bytes)
func (m *Manager) SendTransferBlockRX(dev uint8, code int32, blockID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opTransferBlock, dev, code, u32(blockID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// TRANSFER_BLOCK TX (write): payload is block_id + data_len + data
// Bytes: 4 + 4 + N
func (m *Manager) SendTransferBlockTX(dev uint8, code int32, blockID uint32, data []byte) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opTransferBlock, dev, code, u32(blockID), u32(uint32(len(data))), data)
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendEnqueueBlockCyclic(dev uint8, code int32, blockID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opEnqueueBlockCyclic, dev, code, u32(blockID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendRetryDequeueBlock(dev uint8, code int32, blockID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opRetryDequeueBlock, dev, code, u32(blockID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

// CREATE_EVSTREAM: no payload
func (m *Manager) SendCreateEvStream(dev uint8, code int32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opCreateEvStream, dev, code)
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendFreeEvStream(dev uint8, code int32, streamID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opFreeEvStream, dev, code, u32(streamID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}

func (m *Manager) SendReadEvent(dev uint8, code int32, streamID uint32) (*BinaryHeader, ResponsePlan, error) {
	resp, plan, err := m.sendBinaryCommand(opReadEvent, dev, code, u32(streamID))
	if err != nil {
		return nil, plan, err
	}
	return resp, plan, nil
}
