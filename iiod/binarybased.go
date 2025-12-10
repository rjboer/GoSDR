package iiod

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

////////////////////////////////////////////////////////////////////////////////////////
// Constants from iiod-responder.h
////////////////////////////////////////////////////////////////////////////////////////

const (
	OpVersion   = 0
	OpContext   = 1
	OpReadAttr  = 7
	OpWriteAttr = 8

	OpListDevices  = 11
	OpListChannels = 12

	OpBufferOpen  = 20
	OpBufferRead  = 21
	OpBufferWrite = 22
	OpBufferClose = 23
)

////////////////////////////////////////////////////////////////////////////////////////
// Binary backend
////////////////////////////////////////////////////////////////////////////////////////

type BinaryBackend struct {
	conn net.Conn
}

func NewBinaryBackend(conn net.Conn) *BinaryBackend {
	return &BinaryBackend{conn: conn}
}

////////////////////////////////////////////////////////////////////////////////////////
// Low-level helpers
////////////////////////////////////////////////////////////////////////////////////////

func (bb *BinaryBackend) writeCommand(op uint16, device uint16, code uint16, payload []byte) error {
	var hdr [8]byte
	binary.LittleEndian.PutUint16(hdr[0:2], op)
	binary.LittleEndian.PutUint16(hdr[2:4], device)
	binary.LittleEndian.PutUint16(hdr[4:6], code)
	binary.LittleEndian.PutUint16(hdr[6:8], uint16(len(payload)))

	_, err := bb.conn.Write(hdr[:])
	if err != nil {
		return fmt.Errorf("write command header: %w", err)
	}

	if len(payload) > 0 {
		_, err = bb.conn.Write(payload)
		if err != nil {
			return fmt.Errorf("write command payload: %w", err)
		}
	}
	return nil
}

func (bb *BinaryBackend) readReply(maxBytes int) ([]byte, error) {
	var statusBuf [4]byte
	_, err := io.ReadFull(bb.conn, statusBuf[:])
	if err != nil {
		return nil, fmt.Errorf("binary reply status read: %w", err)
	}

	status := binary.LittleEndian.Uint32(statusBuf[:])
	if status != 0 {
		return nil, fmt.Errorf("binary reply error status: %d", status)
	}

	if maxBytes == 0 {
		return nil, nil
	}

	buf := make([]byte, maxBytes)
	n, err := bb.conn.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("binary reply payload read: %w", err)
	}

	return buf[:n], nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Backend interface implementation
////////////////////////////////////////////////////////////////////////////////////////

// GetXMLContext is unsupported in binary mode (server must support PRINT fallback).
func (bb *BinaryBackend) GetXMLContext(ctx context.Context) (string, error) {
	return "", errors.New("binary backend cannot fetch XML context; router must fallback to text mode")
}

func (bb *BinaryBackend) ReadAttr(ctx context.Context, device string, channel string, attr string) (string, error) {

	key := attr
	if channel != "" {
		key = channel + "/" + attr
	}

	// device index resolution is handled by connect.go before this backend is used
	devID := uint16(0)

	payload := []byte(key)

	err := bb.writeCommand(OpReadAttr, devID, 0, payload)
	if err != nil {
		return "", err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	data, err := bb.readReply(4096)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (bb *BinaryBackend) WriteAttr(ctx context.Context, device string, channel string, attr string, value string) error {

	key := attr
	if channel != "" {
		key = channel + "/" + attr
	}

	payload := append([]byte(key+"="), []byte(value)...)

	devID := uint16(0)

	err := bb.writeCommand(OpWriteAttr, devID, 0, payload)
	if err != nil {
		return err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = bb.readReply(0)
	return err
}

////////////////////////////////////////////////////////////////////////////////////////
// Device & Channel listing
////////////////////////////////////////////////////////////////////////////////////////

func (bb *BinaryBackend) ListDevices(ctx context.Context) ([]string, error) {
	err := bb.writeCommand(OpListDevices, 0, 0, nil)
	if err != nil {
		return nil, err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	data, err := bb.readReply(4096)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return []string{}, nil
	}

	return splitNullTerminated(data), nil
}

func (bb *BinaryBackend) GetChannels(ctx context.Context, device string) ([]string, error) {
	err := bb.writeCommand(OpListChannels, 0, 0, []byte(device))
	if err != nil {
		return nil, err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	data, err := bb.readReply(4096)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return []string{}, nil
	}

	return splitNullTerminated(data), nil
}

////////////////////////////////////////////////////////////////////////////////////////
// Buffer operations
////////////////////////////////////////////////////////////////////////////////////////

func (bb *BinaryBackend) OpenBuffer(ctx context.Context, device string, samples int) (int, error) {
	payload := []byte(fmt.Sprintf("%s:%d", device, samples))

	err := bb.writeCommand(OpBufferOpen, 0, 0, payload)
	if err != nil {
		return -1, err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	data, err := bb.readReply(64)
	if err != nil {
		return -1, err
	}

	var id int
	if _, err := fmt.Sscanf(string(data), "%d", &id); err != nil {
		return -1, fmt.Errorf("buffer open: malformed reply: %q", string(data))
	}

	return id, nil
}

func (bb *BinaryBackend) ReadBuffer(ctx context.Context, bufID int, nBytes int) ([]byte, error) {

	payload := []byte(fmt.Sprintf("%d:%d", bufID, nBytes))

	err := bb.writeCommand(OpBufferRead, 0, 0, payload)
	if err != nil {
		return nil, err
	}

	bb.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	data, err := bb.readReply(nBytes)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (bb *BinaryBackend) WriteBuffer(ctx context.Context, bufID int, data []byte) (int, error) {

	header := fmt.Sprintf("%d:%d:", bufID, len(data))
	payload := append([]byte(header), data...)

	err := bb.writeCommand(OpBufferWrite, 0, 0, payload)
	if err != nil {
		return 0, err
	}

	bb.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	reply, err := bb.readReply(64)
	if err != nil {
		return 0, err
	}

	var written int
	fmt.Sscanf(string(reply), "%d", &written)
	return written, nil
}

func (bb *BinaryBackend) CloseBuffer(ctx context.Context, bufID int) error {

	payload := []byte(fmt.Sprintf("%d", bufID))

	err := bb.writeCommand(OpBufferClose, 0, 0, payload)
	if err != nil {
		return err
	}

	bb.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, err = bb.readReply(0)
	return err
}

////////////////////////////////////////////////////////////////////////////////////////
// Shutdown
////////////////////////////////////////////////////////////////////////////////////////

func (bb *BinaryBackend) Close() error {
	return bb.conn.Close()
}

////////////////////////////////////////////////////////////////////////////////////////
// Utility helpers
////////////////////////////////////////////////////////////////////////////////////////

func splitNullTerminated(data []byte) []string {
	var out []string
	start := 0

	for i, b := range data {
		if b == 0 {
			if i > start {
				out = append(out, string(data[start:i]))
			}
			start = i + 1
		}
	}

	if start < len(data) {
		out = append(out, string(data[start:]))
	}

	return out
}
