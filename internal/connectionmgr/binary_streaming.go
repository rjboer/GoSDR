package connectionmgr

import (
	"encoding/binary"
	"fmt"
	"sort"
)

// Buffer models a binary streaming buffer on the IIOD server.
type Buffer struct {
	ID       uint16
	Dev      uint8
	Channels []uint8
	Cyclic   bool

	nextBlockID uint16
}

// Block models a fixed-size transfer block associated with a Buffer.
type Block struct {
	ID     uint16
	Size   int
	buffer *Buffer
}

// EnterBinaryMode must be called before CreateBuffer or any binary streaming operations.
// CreateBuffer sends CREATE_BUFFER with the channel bitmask payload and returns the buffer metadata.
func (m *Manager) CreateBuffer(dev uint8, channels []uint8, cyclic bool) (*Buffer, error) {
	if m == nil {
		return nil, fmt.Errorf("nil Manager")
	}
	if m.conn == nil {
		return nil, fmt.Errorf("CreateBuffer: not connected")
	}
	if m.Mode != ModeBinary {
		return nil, fmt.Errorf("CreateBuffer: not in binary mode")
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("CreateBuffer: at least one channel is required")
	}

	maskPayload := encodeChannelMask(channels)
	bufID := m.nextBufferID
	m.nextBufferID++

	respBuf := make([]byte, len(maskPayload))
	if _, _, err := m.roundTripBinary(
		opCreateBuffer,
		dev,
		int32(bufID),
		[][]byte{maskPayload},
		respBuf,
	); err != nil {
		return nil, err
	}

	sortedCh := append([]uint8(nil), channels...)
	sort.Slice(sortedCh, func(i, j int) bool { return sortedCh[i] < sortedCh[j] })

	return &Buffer{
		ID:       bufID,
		Dev:      dev,
		Channels: sortedCh,
		Cyclic:   cyclic,
	}, nil
}

// EnableBuffer sends ENABLE_BUFFER for the given Buffer.
func (m *Manager) EnableBuffer(buf *Buffer) error {
	if m == nil {
		return fmt.Errorf("nil Manager")
	}
	if buf == nil {
		return fmt.Errorf("EnableBuffer: buffer is nil")
	}
	if m.Mode != ModeBinary {
		return fmt.Errorf("EnableBuffer: not in binary mode")
	}

	_, _, err := m.roundTripBinary(
		opEnableBuffer,
		buf.Dev,
		int32(buf.ID),
		nil,
		nil,
	)
	return err
}

// DisableBuffer sends DISABLE_BUFFER for the given Buffer.
func (m *Manager) DisableBuffer(buf *Buffer) error {
	if m == nil {
		return fmt.Errorf("nil Manager")
	}
	if buf == nil {
		return fmt.Errorf("DisableBuffer: buffer is nil")
	}
	if m.Mode != ModeBinary {
		return fmt.Errorf("DisableBuffer: not in binary mode")
	}

	_, _, err := m.roundTripBinary(
		opDisableBuffer,
		buf.Dev,
		int32(buf.ID),
		nil,
		nil,
	)
	return err
}

// FreeBuffer sends FREE_BUFFER for the given Buffer.
func (m *Manager) FreeBuffer(buf *Buffer) error {
	if m == nil {
		return fmt.Errorf("nil Manager")
	}
	if buf == nil {
		return fmt.Errorf("FreeBuffer: buffer is nil")
	}
	if m.Mode != ModeBinary {
		return fmt.Errorf("FreeBuffer: not in binary mode")
	}

	_, _, err := m.roundTripBinary(
		opFreeBuffer,
		buf.Dev,
		int32(buf.ID),
		nil,
		nil,
	)
	return err
}

// CreateBlock allocates a block on the server with the requested byte size.
func (m *Manager) CreateBlock(buf *Buffer, blockSize int) (*Block, error) {
	if m == nil {
		return nil, fmt.Errorf("nil Manager")
	}
	if buf == nil {
		return nil, fmt.Errorf("CreateBlock: buffer is nil")
	}
	if blockSize <= 0 {
		return nil, fmt.Errorf("CreateBlock: blockSize must be positive")
	}
	if m.Mode != ModeBinary {
		return nil, fmt.Errorf("CreateBlock: not in binary mode")
	}

	blockID := buf.nextBlockID
	buf.nextBlockID++

	sizeLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeLE, uint64(blockSize))

	if _, _, err := m.roundTripBinary(
		opCreateBlock,
		buf.Dev,
		composeBlockCode(buf.ID, blockID),
		[][]byte{sizeLE},
		nil,
	); err != nil {
		return nil, err
	}

	return &Block{
		ID:     blockID,
		Size:   blockSize,
		buffer: buf,
	}, nil
}

// FreeBlock releases a block previously created on the given Buffer.
func (m *Manager) FreeBlock(blk *Block) error {
	if m == nil {
		return fmt.Errorf("nil Manager")
	}
	if blk == nil || blk.buffer == nil {
		return fmt.Errorf("FreeBlock: block or parent buffer is nil")
	}
	if m.Mode != ModeBinary {
		return fmt.Errorf("FreeBlock: not in binary mode")
	}

	_, _, err := m.roundTripBinary(
		opFreeBlock,
		blk.buffer.Dev,
		composeBlockCode(blk.buffer.ID, blk.ID),
		nil,
		nil,
	)
	return err
}

// TransferBlock performs one TRANSFER_BLOCK transaction and reads the payload into dst.
// It returns the byte count reported by the server (resp.Code) after draining any overflow.
func (m *Manager) TransferBlock(blk *Block, dst []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("nil Manager")
	}
	if blk == nil || blk.buffer == nil {
		return 0, fmt.Errorf("TransferBlock: block or parent buffer is nil")
	}
	if m.Mode != ModeBinary {
		return 0, fmt.Errorf("TransferBlock: not in binary mode")
	}

	bytesUsed := blk.Size
	if bytesUsed <= 0 {
		return 0, fmt.Errorf("TransferBlock: block size must be > 0")
	}

	sizeLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeLE, uint64(bytesUsed))

	resp, copied, err := m.roundTripBinary(
		opTransferBlock,
		blk.buffer.Dev,
		composeBlockCode(blk.buffer.ID, blk.ID),
		[][]byte{sizeLE},
		dst,
	)
	if err != nil {
		return copied, err
	}

	return int(resp.Code), nil
}

// StartRXStream continuously issues TRANSFER_BLOCK and delivers payload copies to out until stop is signaled.
func (m *Manager) StartRXStream(buf *Buffer, blk *Block, out chan<- []byte, stop <-chan struct{}) error {
	if buf == nil {
		return fmt.Errorf("StartRXStream: buffer is nil")
	}
	if blk == nil {
		return fmt.Errorf("StartRXStream: block is nil")
	}
	if blk.buffer != buf {
		return fmt.Errorf("StartRXStream: block does not belong to buffer")
	}
	if out == nil {
		return fmt.Errorf("StartRXStream: out channel is nil")
	}

	payload := make([]byte, blk.Size)

	for {
		select {
		case <-stop:
			return nil
		default:
		}

		n, err := m.TransferBlock(blk, payload)
		if err != nil {
			return err
		}
		if n <= 0 {
			continue
		}

		copyLen := n
		if copyLen > len(payload) {
			copyLen = len(payload)
		}

		frame := make([]byte, copyLen)
		copy(frame, payload[:copyLen])

		select {
		case out <- frame:
		case <-stop:
			return nil
		}
	}
}

func encodeChannelMask(channels []uint8) []byte {
	var maxIdx uint8
	for _, ch := range channels {
		if ch > maxIdx {
			maxIdx = ch
		}
	}

	words := int(maxIdx/32) + 1
	mask := make([]uint32, words)
	for _, ch := range channels {
		word := ch / 32
		bit := ch % 32
		mask[word] |= 1 << bit
	}

	payload := make([]byte, words*4)
	for i, word := range mask {
		binary.LittleEndian.PutUint32(payload[i*4:], word)
	}
	return payload
}

func composeBlockCode(bufID, blockID uint16) int32 {
	return int32(bufID) | int32(blockID)<<16
}
