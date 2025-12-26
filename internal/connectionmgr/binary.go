package connectionmgr

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
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
type BinaryHeader struct {
	ClientID uint16
	Opcode   uint8
	Dev      uint8
	Code     int32
}

// =======================
// Binary opcodes (uint8) – explicit hex values.
const (
	opResponse uint8 = 0x00
	opPrint    uint8 = 0x01
	opTimeout  uint8 = 0x02

	// Attribute reads
	opReadAttr    uint8 = 0x03
	opReadDbgAttr uint8 = 0x04
	opReadBufAttr uint8 = 0x05
	opReadChnAttr uint8 = 0x06

	// Attribute writes
	opWriteAttr    uint8 = 0x07
	opWriteDbgAttr uint8 = 0x08
	opWriteBufAttr uint8 = 0x09
	opWriteChnAttr uint8 = 0x0a

	// Triggers
	opGetTrig uint8 = 0x0b
	opSetTrig uint8 = 0x0c

	// Buffer lifecycle
	opCreateBuffer  uint8 = 0x0d
	opFreeBuffer    uint8 = 0x0e
	opEnableBuffer  uint8 = 0x0f
	opDisableBuffer uint8 = 0x10

	// Block lifecycle / streaming
	opCreateBlock        uint8 = 0x11
	opFreeBlock          uint8 = 0x12
	opTransferBlock      uint8 = 0x13
	opEnqueueBlockCyclic uint8 = 0x14
	opRetryDequeueBlock  uint8 = 0x15

	// Event streaming
	opCreateEvStream uint8 = 0x16
	opFreeEvStream   uint8 = 0x17
	opReadEvent      uint8 = 0x18
)

const (
	RespNone                   uint8 = iota // no response expected; do not read anything
	RespStatusOnly                          // after response header: int32 status
	RespStatusAndLPBytes                    // after response header: int32 status + uint32 len + len bytes
	RespStatusAndU32                        // after response header: int32 status + uint32 id/handle
	RespStatusAndU32AndLPBytes              // (rare) if you ever need it; keep available
)

// ResponsePlan describes what (if anything) should be read after the 8-byte response header.
type ResponsePlan struct {
	ExpectHeader bool
	Opcode       uint8
	Shape        uint8
}

var responsePlans = map[uint8]ResponsePlan{
	// server->client only; should never be sent as a request
	opResponse: {ExpectHeader: false, Opcode: 0x00, Shape: RespNone},
	opPrint:    {ExpectHeader: true, Opcode: 0x01, Shape: RespStatusAndLPBytes},
	opTimeout:  {ExpectHeader: false, Opcode: 0x02, Shape: RespNone},

	// Reads: header + status + LP bytes
	opReadAttr:    {ExpectHeader: true, Opcode: 0x03, Shape: RespStatusAndLPBytes},
	opReadDbgAttr: {ExpectHeader: true, Opcode: 0x04, Shape: RespStatusAndLPBytes},
	opReadBufAttr: {ExpectHeader: true, Opcode: 0x05, Shape: RespStatusAndLPBytes},
	opReadChnAttr: {ExpectHeader: true, Opcode: 0x06, Shape: RespStatusAndLPBytes},

	// Writes: header + status
	opWriteAttr:    {ExpectHeader: true, Opcode: 0x07, Shape: RespStatusOnly},
	opWriteDbgAttr: {ExpectHeader: true, Opcode: 0x08, Shape: RespStatusOnly},
	opWriteBufAttr: {ExpectHeader: true, Opcode: 0x09, Shape: RespStatusOnly},
	opWriteChnAttr: {ExpectHeader: true, Opcode: 0x0a, Shape: RespStatusOnly},

	// Triggers: GET returns string, SET returns status
	opGetTrig: {ExpectHeader: true, Opcode: 0x0b, Shape: RespStatusAndLPBytes},
	opSetTrig: {ExpectHeader: true, Opcode: 0x0c, Shape: RespStatusOnly},

	// Buffer ops: create returns id, others status
	opCreateBuffer:  {ExpectHeader: true, Opcode: 0x0d, Shape: RespStatusAndU32},
	opFreeBuffer:    {ExpectHeader: true, Opcode: 0x0e, Shape: RespStatusOnly},
	opEnableBuffer:  {ExpectHeader: true, Opcode: 0x0f, Shape: RespStatusOnly},
	opDisableBuffer: {ExpectHeader: true, Opcode: 0x10, Shape: RespStatusOnly},

	// Block ops: create returns id, others status; transfer varies (RX returns data)
	opCreateBlock:        {ExpectHeader: true, Opcode: 0x11, Shape: RespStatusAndU32},
	opFreeBlock:          {ExpectHeader: true, Opcode: 0x12, Shape: RespStatusOnly},
	opTransferBlock:      {ExpectHeader: true, Opcode: 0x13, Shape: RespStatusAndLPBytes}, // For TX you may treat as RespStatusOnly; see note below
	opEnqueueBlockCyclic: {ExpectHeader: true, Opcode: 0x14, Shape: RespStatusOnly},
	opRetryDequeueBlock:  {ExpectHeader: true, Opcode: 0x15, Shape: RespStatusOnly},

	// Events: create returns stream id, read returns blob, free returns status
	opCreateEvStream: {ExpectHeader: true, Opcode: 0x16, Shape: RespStatusAndU32},
	opFreeEvStream:   {ExpectHeader: true, Opcode: 0x17, Shape: RespStatusOnly},
	opReadEvent:      {ExpectHeader: true, Opcode: 0x18, Shape: RespStatusAndLPBytes},
}

//
// =======================
// Low-level primitives
// =======================
//

// sendBinaryCommand sends a binary command to the server and returns the response.
// I can add additional checks later.
func (m *Manager) sendBinaryCommand(
	opcode, dev uint8,
	code int32,
	payloads ...[]byte,
) (*BinaryHeader, ResponsePlan, error) {

	log.Printf("sendBinaryCommand function: %v", dev)
	if opcode > 0x18 {
		return nil, ResponsePlan{}, fmt.Errorf("sendBinaryCommand: op out of range: %d", opcode)
	}
	if dev > 0x7f {
		return nil, ResponsePlan{}, fmt.Errorf("sendBinaryCommand: dev out of range: %d", dev)
	}

	plan, ok := responsePlans[opcode]
	log.Println("plan: ", plan)
	log.Println("ok: ", ok)
	if !ok {
		plan = ResponsePlan{ExpectHeader: false, Opcode: opcode, Shape: RespNone}
	}

	// prevent server-only opcodes
	// if opcode <= opTimeout {
	// 	return nil, plan, fmt.Errorf("sendBinaryCommand: opcode 0x%02x is server-only", opcode)
	// }

	if m == nil || m.conn == nil {
		return nil, plan, fmt.Errorf("sendBinaryCommand: not connected")
	}

	// ---- build header ----
	var hdr [8]byte
	binary.BigEndian.PutUint16(hdr[0:2], m.clientID)
	hdr[2] = opcode
	hdr[3] = dev
	binary.BigEndian.PutUint32(hdr[4:8], uint32(code))

	log.Println("hdr: ", hdr)
	// ---- send header ----
	if err := m.writeAll(hdr[:]); err != nil {
		return nil, plan, err
	}

	// ---- send payload(s) ----
	for _, payload := range payloads {
		if len(payload) == 0 {
			continue
		}
		log.Println("payload: ", payload)
		if err := m.writeAll(payload); err != nil {
			return nil, plan, err
		}
	}

	// ---- no response expected ----
	if !plan.ExpectHeader {
		return nil, plan, nil
	}
	log.Println("Read response on binary command: ", plan)
	m.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	fmt.Println("time now: ", time.Now())
	// ---- read exactly one response header ----
	var rhdr [8]byte
	n, err := io.ReadFull(m.conn, rhdr[:])
	fmt.Println("time now: ", time.Now())
	log.Println("number of bytes read: ", n)
	log.Println("read buffer: ", rhdr)
	log.Println("error: ", err)
	if err != nil {
		return nil, plan, err
	}

	if plan.Opcode != opcode {
		log.Printf("[IIOD RX] unexpected opcode in response header: 0x%02x", plan.Opcode)
	}

	resp := &BinaryHeader{
		ClientID: binary.BigEndian.Uint16(rhdr[0:2]),
		Opcode:   rhdr[2],
		Dev:      rhdr[3],
		Code:     int32(binary.BigEndian.Uint32(rhdr[4:8])),
	}

	return resp, plan, nil
}

func (m *Manager) readResponse(plan ResponsePlan) (
	status int32,
	u32 uint32,
	data []byte,
	err error,
) {
	fmt.Println("readResponse function")

	log.Printf("[IIOD RX] readResponse: shape=%d expectHeader=%v opcode=0x%02x",
		plan.Shape, plan.ExpectHeader, plan.Opcode)

	switch plan.Shape {

	case RespNone:
		log.Printf("[IIOD RX] RespNone: no payload to read")
		return 0, 0, nil, nil

	case RespStatusOnly:
		log.Printf("[IIOD RX] RespStatusOnly: reading 4-byte status")
		var b [4]byte
		if _, err = io.ReadFull(m.conn, b[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading status: %v", err)
			return 0, 0, nil, err
		}
		status = int32(binary.BigEndian.Uint32(b[:]))
		log.Printf("[IIOD RX] status=%d", status)
		return status, 0, nil, nil

	case RespStatusAndU32:
		log.Printf("[IIOD RX] RespStatusAndU32: reading 8 bytes (status + u32)")
		var b [8]byte
		if _, err = io.ReadFull(m.conn, b[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading status+u32: %v", err)
			return 0, 0, nil, err
		}
		status = int32(binary.BigEndian.Uint32(b[0:4]))
		u32 = binary.BigEndian.Uint32(b[4:8])
		log.Printf("[IIOD RX] status=%d u32=%d", status, u32)
		return status, u32, nil, nil

	case RespStatusAndLPBytes:
		log.Printf("[IIOD RX] RespStatusAndLPBytes: reading status")
		var sb [4]byte
		if _, err = io.ReadFull(m.conn, sb[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading status: %v", err)
			return 0, 0, nil, err
		}
		status = int32(binary.BigEndian.Uint32(sb[:]))
		log.Printf("[IIOD RX] status=%d", status)

		log.Printf("[IIOD RX] reading length prefix")
		var lb [4]byte
		if _, err = io.ReadFull(m.conn, lb[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading length: %v", err)
			return status, 0, nil, err
		}
		n := binary.BigEndian.Uint32(lb[:])
		log.Printf("[IIOD RX] payload length=%d", n)

		const maxPayload = 20 << 20
		if n > maxPayload {
			log.Printf("[IIOD RX] ERROR payload too large: %d > %d", n, maxPayload)
			return status, 0, nil, fmt.Errorf("payload too large: %d bytes", n)
		}

		if n == 0 {
			log.Printf("[IIOD RX] zero-length payload")
			return status, 0, []byte{}, nil
		}

		data = make([]byte, n)
		log.Printf("[IIOD RX] reading %d payload bytes", n)
		if _, err = io.ReadFull(m.conn, data); err != nil {
			log.Printf("[IIOD RX] ERROR reading payload: %v", err)
			return status, 0, nil, err
		}
		log.Printf("[IIOD RX] payload read complete (%d bytes)", n)
		return status, 0, data, nil

	case RespStatusAndU32AndLPBytes:
		log.Printf("[IIOD RX] RespStatusAndU32AndLPBytes: reading status + u32")
		var hb [8]byte
		if _, err = io.ReadFull(m.conn, hb[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading status+u32: %v", err)
			return 0, 0, nil, err
		}
		status = int32(binary.BigEndian.Uint32(hb[0:4]))
		u32 = binary.BigEndian.Uint32(hb[4:8])
		log.Printf("[IIOD RX] status=%d u32=%d", status, u32)

		log.Printf("[IIOD RX] reading length prefix")
		var lb [4]byte
		if _, err = io.ReadFull(m.conn, lb[:]); err != nil {
			log.Printf("[IIOD RX] ERROR reading length: %v", err)
			return status, u32, nil, err
		}
		n := binary.BigEndian.Uint32(lb[:])
		log.Printf("[IIOD RX] payload length=%d", n)

		const maxPayload = 20 << 20
		if n > maxPayload {
			log.Printf("[IIOD RX] ERROR payload too large: %d > %d", n, maxPayload)
			return status, u32, nil, fmt.Errorf("payload too large: %d bytes", n)
		}

		if n == 0 {
			log.Printf("[IIOD RX] zero-length payload")
			return status, u32, []byte{}, nil
		}

		data = make([]byte, n)
		log.Printf("[IIOD RX] reading %d payload bytes", n)
		if _, err = io.ReadFull(m.conn, data); err != nil {
			log.Printf("[IIOD RX] ERROR reading payload: %v", err)
			return status, u32, nil, err
		}
		log.Printf("[IIOD RX] payload read complete (%d bytes)", n)
		return status, u32, data, nil

	default:
		log.Printf("[IIOD RX] ERROR unknown response shape=%d", plan.Shape)
		return 0, 0, nil, fmt.Errorf("readBinaryResponse: unknown shape %d", plan.Shape)
	}
}

// recvBinaryResponseHeader reads exactly one binary response header.
// TODO: RJ remove this function, obsolete
func (m *Manager) recvBinaryResponseHeader() (BinaryHeader, error) {
	var hdr [8]byte
	if err := m.readAll(hdr[:]); err != nil {
		return BinaryHeader{}, err
	}

	fmt.Println("recvBinaryResponseHeader: op", hdr[2], "dev", hdr[3], "code", int32(binary.BigEndian.Uint32(hdr[4:8])))
	return BinaryHeader{
		ClientID: binary.BigEndian.Uint16(hdr[0:2]),
		Opcode:   hdr[2],
		Dev:      hdr[3],
		Code:     int32(binary.BigEndian.Uint32(hdr[4:8])),
	}, nil
}

// discardN drains exactly n bytes from the connection.
// TODO: RJ remove this function, obsolete
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

// u32 encodes a uint32 (BE).
func u32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// i32 encodes an int32 (BE), using uint32 bits.
func i32(v int32) []byte {
	return u32(uint32(v))
}

// lpBytes encodes a length-prefixed byte slice: uint32(len) + bytes.
func lpBytes(p []byte) []byte {
	b := make([]byte, 4+len(p))
	binary.BigEndian.PutUint32(b[0:4], uint32(len(p)))
	copy(b[4:], p)
	return b
}

// lpString is lpBytes([]byte(s)).
func lpString(s string) []byte { return lpBytes([]byte(s)) }

// nameValue encodes: lp(name) + lp(value)
func nameValue(name, value string) []byte {
	nb := []byte(name)
	vb := []byte(value)

	out := make([]byte, 4+len(nb)+4+len(vb))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(nb)))
	copy(out[4:4+len(nb)], nb)

	off := 4 + len(nb)
	binary.BigEndian.PutUint32(out[off:off+4], uint32(len(vb)))
	copy(out[off+4:], vb)
	return out
}

// u32SliceWithCount encodes: uint32(count) + count*uint32(items...).
func u32SliceWithCount(items []uint32) []byte {
	out := make([]byte, 4+4*len(items))
	binary.BigEndian.PutUint32(out[0:4], uint32(len(items)))
	off := 4
	for _, v := range items {
		binary.BigEndian.PutUint32(out[off:off+4], v)
		off += 4
	}
	return out
}
