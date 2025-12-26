package connectionmgr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// StreamQueueConfig controls buffering between producers and consumers in binary streaming.
// Watermarks provide backpressure signaling for callers that want to react to queue pressure
// (e.g., slow down writers when the high watermark is crossed, resume when the low watermark
// is reached again).
type StreamQueueConfig struct {
	Depth           int
	HighWatermark   int
	LowWatermark    int
	HighWatermarkCh chan<- struct{}
	LowWatermarkCh  chan<- struct{}
}

type streamQueueConfig struct {
	depth         int
	highWatermark int
	lowWatermark  int
	highCh        chan<- struct{}
	lowCh         chan<- struct{}
}

func normalizeStreamQueueConfig(cfg StreamQueueConfig) streamQueueConfig {
	const defaultDepth = 8

	depth := cfg.Depth
	if depth <= 0 {
		depth = defaultDepth
	}
	high := cfg.HighWatermark
	if high <= 0 || high > depth {
		high = depth - 1
	}
	low := cfg.LowWatermark
	if low < 0 || low >= high {
		low = high / 2
	}

	highCh := cfg.HighWatermarkCh
	if highCh == nil {
		highCh = make(chan struct{}, 1)
	}
	lowCh := cfg.LowWatermarkCh
	if lowCh == nil {
		lowCh = make(chan struct{}, 1)
	}

	return streamQueueConfig{depth: depth, highWatermark: high, lowWatermark: low, highCh: highCh, lowCh: lowCh}
}

var (
	errQueueClosed  = errors.New("stream queue closed")
	errQueueStopped = errors.New("stream queue stopped")
)

type streamQueue struct {
	cfg      streamQueueConfig
	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	items    [][]byte
	closed   bool
	err      error
	belowLow bool
}

func newStreamQueue(cfg StreamQueueConfig) *streamQueue {
	nCfg := normalizeStreamQueueConfig(cfg)
	q := &streamQueue{
		cfg:      nCfg,
		belowLow: true,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

func (q *streamQueue) close(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	if err != nil {
		q.err = err
	}
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

func (q *streamQueue) enqueue(item []byte, stop <-chan struct{}) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for !q.closed && len(q.items) >= q.cfg.depth {
		if stop != nil {
			q.mu.Unlock()
			select {
			case <-stop:
				return errQueueStopped
			default:
			}
			q.mu.Lock()
		}
		q.notFull.Wait()
	}

	if q.closed {
		if q.err != nil {
			return q.err
		}
		return errQueueClosed
	}

	q.items = append(q.items, item)
	q.emitWatermarksLocked()
	q.notEmpty.Signal()
	return nil
}

func (q *streamQueue) dequeue(stop <-chan struct{}) ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.items) == 0 && !q.closed {
		if stop != nil {
			q.mu.Unlock()
			select {
			case <-stop:
				return nil, errQueueStopped
			default:
			}
			q.mu.Lock()
		}
		q.notEmpty.Wait()
	}

	if len(q.items) == 0 {
		if q.err != nil {
			return nil, q.err
		}
		return nil, errQueueClosed
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.emitWatermarksLocked()
	q.notFull.Signal()
	return item, nil
}

func (q *streamQueue) emitWatermarksLocked() {
	size := len(q.items)
	if size >= q.cfg.highWatermark && q.belowLow {
		q.belowLow = false
		select {
		case q.cfg.highCh <- struct{}{}:
		default:
		}
	}
	if size <= q.cfg.lowWatermark && !q.belowLow {
		q.belowLow = true
		select {
		case q.cfg.lowCh <- struct{}{}:
		default:
		}
	}
}

// Buffer models a binary streaming buffer on the IIOD server.
type Buffer struct {
	ID       uint16
	Dev      uint8
	Channels []uint8
	Cyclic   bool

	nextBlockID uint16
	inFlight    map[uint16]int
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
		inFlight: make(map[uint16]int),
	}, nil
}

func (m *Manager) CreateBuffer2(
	dev uint8,
	channels []uint8,
	cyclic bool,
) (*Buffer, error) {

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

	// Channel mask goes in CODE
	mask := encodeChannelMask2(channels)

	// 1. Send command
	fmt.Println("Send command")
	if err := m.sendBinaryCommand(opCreateBuffer, dev, mask); err != nil {
		return nil, err
	}

	// 2. Read response header
	fmt.Println("Read response header")
	var resp iiodCommand
	if err := m.readBinaryHeader(&resp); err != nil {
		return nil, err
	}
	fmt.Println("Print response header:", resp)

	// 3. Check return code
	fmt.Println("Check return code")
	if resp.Code < 0 {
		return nil, fmt.Errorf("CREATE_BUFFER failed: %d", resp.Code)
	}

	bufID := uint16(resp.Code)

	fmt.Println("CreateBuffer2: buffer ID", bufID)
	return &Buffer{
		ID:       bufID,
		Dev:      dev,
		Channels: append([]uint8(nil), channels...),
		Cyclic:   cyclic,
		inFlight: make(map[uint16]int),
	}, nil
}

func (m *Manager) CreateBuffer3(dev uint8, channels []uint8, cyclic bool) (*Buffer, error) {
	if m.Mode != ModeBinary {
		return nil, fmt.Errorf("CreateBuffer: not in binary mode")
	}
	fmt.Println("CreateBuffer3: device", dev, "channels", channels)
	maskPayload := encodeChannelMask3(channels)

	// IMPORTANT:
	// code = number of channels
	if err := m.sendBinaryCommand(
		opCreateBuffer,
		dev,
		int32(len(channels)),
		maskPayload,
	); err != nil {
		return nil, err
	}

	// // Send payload immediately after header
	// if err := m.writeAll(maskPayload); err != nil {
	// 	return nil, err
	// }

	// Read binary response header
	// var hdr [8]byte
	// if err := m.readAll(hdr[:]); err != nil {
	// 	return nil, err
	// }
	// fmt.Println("Print binary response header", hdr)
	// rc := int32(binary.BigEndian.Uint32(hdr[4:8]))
	// if rc < 0 {
	// 	return nil, fmt.Errorf("CREATE_BUFFER failed rc=%d", rc)
	// }

	// bufID := uint16(rc)
	// fmt.Println("CreateBuffer3: buffer ID", bufID)
	return &Buffer{
		ID:       0,
		Dev:      dev,
		Channels: append([]uint8(nil), channels...),
		Cyclic:   cyclic,
	}, nil
}

func encodeChannelMask3(channels []uint8) []byte {
	var mask uint32
	for _, ch := range channels {
		if ch >= 32 {
			panic("channel index out of range")
		}
		mask |= 1 << ch
	}

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, mask)
	return buf
}

func encodeChannelMask2(channels []uint8) int32 {
	var mask int32
	for _, ch := range channels {
		if ch >= 32 {
			panic("channel index out of range")
		}
		mask |= 1 << ch
	}
	return mask
}

func (m *Manager) readBinaryHeader(cmd *iiodCommand) error {
	var hdr [8]byte
	if err := m.readAll(hdr[:]); err != nil {
		return err
	}

	fmt.Println("Read binary header", hdr)

	cmd.ClientID = binary.BigEndian.Uint16(hdr[0:2])
	cmd.Op = hdr[2]
	cmd.Dev = hdr[3]
	cmd.Code = int32(binary.BigEndian.Uint32(hdr[4:8]))

	return nil
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
	if blk.buffer.inFlight[blk.ID] > 0 {
		return fmt.Errorf("FreeBlock: block %d is still in-flight", blk.ID)
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

	if blk.buffer.inFlight == nil {
		blk.buffer.inFlight = make(map[uint16]int)
	}
	blk.buffer.inFlight[blk.ID]++
	defer func() {
		blk.buffer.inFlight[blk.ID]--
	}()

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

// TransferTxBlock writes a payload for the given block and returns the status code.
func (m *Manager) TransferTxBlock(blk *Block, payload []byte) (int, error) {
	if m == nil {
		return 0, fmt.Errorf("nil Manager")
	}
	if blk == nil || blk.buffer == nil {
		return 0, fmt.Errorf("TransferTxBlock: block or parent buffer is nil")
	}
	if m.Mode != ModeBinary {
		return 0, fmt.Errorf("TransferTxBlock: not in binary mode")
	}
	if len(payload) == 0 {
		return 0, fmt.Errorf("TransferTxBlock: payload is empty")
	}

	if blk.buffer.inFlight == nil {
		blk.buffer.inFlight = make(map[uint16]int)
	}
	blk.buffer.inFlight[blk.ID]++
	defer func() {
		blk.buffer.inFlight[blk.ID]--
	}()

	sizeLE := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeLE, uint64(len(payload)))

	resp, _, err := m.roundTripBinary(
		opTransferBlock,
		blk.buffer.Dev,
		composeBlockCode(blk.buffer.ID, blk.ID),
		[][]byte{sizeLE, payload},
		nil,
	)
	if err != nil {
		return 0, err
	}
	if resp.Code < 0 {
		return 0, ErrIiodStatus{Op: resp.Op, Dev: resp.Dev, Code: resp.Code}
	}
	return int(resp.Code), nil
}

// StartRXStream continuously issues TRANSFER_BLOCK and delivers payload copies to out until stop is signaled.
// A bounded queue mediates between the network producer and the caller-provided consumer channel so that
// spikes in either direction do not permanently stall the other side. Queue depth and watermarks are
// controlled via cfg.
func (m *Manager) StartRXStream(buf *Buffer, blk *Block, out chan<- []byte, stop <-chan struct{}, cfg StreamQueueConfig) error {
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

	q := newStreamQueue(cfg)
	if stop != nil {
		go func() {
			<-stop
			q.close(errQueueStopped)
		}()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	payload := make([]byte, blk.Size)

	producer := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}

			n, err := m.TransferBlock(blk, payload)
			if err != nil {
				errCh <- err
				q.close(err)
				return
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

			if err := q.enqueue(frame, stop); err != nil {
				if !errors.Is(err, errQueueClosed) && !errors.Is(err, errQueueStopped) {
					errCh <- err
				}
				return
			}
		}
	}

	consumer := func() {
		defer wg.Done()
		for {
			frame, err := q.dequeue(stop)
			if err != nil {
				if !errors.Is(err, errQueueClosed) && !errors.Is(err, errQueueStopped) {
					errCh <- err
				}
				return
			}

			select {
			case <-stop:
				q.close(errQueueStopped)
				return
			case out <- frame:
			}
		}
	}

	wg.Add(2)
	go producer()
	go consumer()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// StartTXStream continuously dequeues payloads and transmits them until stop is signaled.
func (m *Manager) StartTXStream(buf *Buffer, blk *Block, in <-chan []byte, stop <-chan struct{}, cfg StreamQueueConfig) error {
	if buf == nil {
		return fmt.Errorf("StartTXStream: buffer is nil")
	}
	if blk == nil {
		return fmt.Errorf("StartTXStream: block is nil")
	}
	if blk.buffer != buf {
		return fmt.Errorf("StartTXStream: block does not belong to buffer")
	}
	if in == nil {
		return fmt.Errorf("StartTXStream: input channel is nil")
	}

	q := newStreamQueue(cfg)
	if stop != nil {
		go func() {
			<-stop
			q.close(errQueueStopped)
		}()
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	producer := func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				q.close(errQueueStopped)
				return
			case frame, ok := <-in:
				if !ok {
					q.close(nil)
					return
				}
				if len(frame) > blk.Size {
					frame = frame[:blk.Size]
				}
				copyFrame := make([]byte, len(frame))
				copy(copyFrame, frame)
				if err := q.enqueue(copyFrame, stop); err != nil {
					if !errors.Is(err, errQueueClosed) && !errors.Is(err, errQueueStopped) {
						errCh <- err
					}
					return
				}
			}
		}
	}

	consumer := func() {
		defer wg.Done()
		for {
			frame, err := q.dequeue(stop)
			if err != nil {
				if !errors.Is(err, errQueueClosed) && !errors.Is(err, errQueueStopped) {
					errCh <- err
				}
				return
			}

			if _, err := m.TransferTxBlock(blk, frame); err != nil {
				errCh <- err
				q.close(err)
				return
			}
		}
	}

	wg.Add(2)
	go producer()
	go consumer()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
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
