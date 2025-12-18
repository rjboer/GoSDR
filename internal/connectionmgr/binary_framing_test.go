package connectionmgr

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func writeHdr(t *testing.T, c net.Conn, clientID uint16, op, dev uint8, code int32) {
	t.Helper()
	var hdr [8]byte
	binary.BigEndian.PutUint16(hdr[0:2], clientID)
	hdr[2] = op
	hdr[3] = dev
	binary.BigEndian.PutUint32(hdr[4:8], uint32(code))
	_, err := c.Write(hdr[:])
	if err != nil {
		t.Fatalf("write hdr: %v", err)
	}
}

func TestBinaryFraming_NoDesyncOnOddChunking(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{}
	m.SetConn(client)
	m.SetClientID(1)

	const (
		op  = 0x99
		dev = 3
	)
	payload1 := make([]byte, 4096)
	payload2 := make([]byte, 44096)

	for i := range payload1 {
		payload1[i] = byte(i)
	}
	for i := range payload2 {
		payload2[i] = byte(255 - (i % 256))
	}

	// Fake server goroutine: respond to two commands with:
	// [hdr][payload] but split into adversarial chunk boundaries.
	done := make(chan struct{})
	go func() {
		defer close(done)

		// Read first command hdr (8 bytes)
		var cmd [8]byte
		_, _ = server.Read(cmd[:]) // net.Pipe is in-order; ignoring partial for brevity

		// Respond: hdr + payload1, but split: 3 bytes, then 5 bytes, then payload in pieces.
		writeHdr(t, server, 1, op, dev, int32(len(payload1)))
		_, _ = server.Write(payload1[:17])
		_, _ = server.Write(payload1[17:123])
		_, _ = server.Write(payload1[123:])

		// Read second command hdr
		_, _ = server.Read(cmd[:])

		// Respond second: header then payload2, split header across writes to mimic TCP oddities.
		var hdr [8]byte
		binary.BigEndian.PutUint16(hdr[0:2], 1)
		hdr[2] = op
		hdr[3] = dev
		binary.BigEndian.PutUint32(hdr[4:8], uint32(len(payload2)))

		_, _ = server.Write(hdr[:2])
		_, _ = server.Write(hdr[2:7])
		_, _ = server.Write(hdr[7:])

		// Payload2 split
		_, _ = server.Write(payload2[:1])
		_, _ = server.Write(payload2[1:4096])
		_, _ = server.Write(payload2[4096:])
	}()

	// Client round trip 1
	got1 := make([]byte, len(payload1))
	if _, _, err := m.roundTripBinary(op, dev, 0, nil, got1); err != nil {
		t.Fatalf("rt1: %v", err)
	}
	if len(got1) != len(payload1) {
		t.Fatalf("rt1 size: got %d want %d", len(got1), len(payload1))
	}
	for i := range got1 {
		if got1[i] != payload1[i] {
			t.Fatalf("rt1 payload mismatch at %d", i)
		}
	}

	// Client round trip 2
	got2 := make([]byte, len(payload2))
	if _, _, err := m.roundTripBinary(op, dev, 0, nil, got2); err != nil {
		t.Fatalf("rt2: %v", err)
	}
	if len(got2) != len(payload2) {
		t.Fatalf("rt2 size: got %d want %d", len(got2), len(payload2))
	}
	for i := range got2 {
		if got2[i] != payload2[i] {
			t.Fatalf("rt2 payload mismatch at %d", i)
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}
