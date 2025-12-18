package connectionmgr

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestEnterBinaryMode(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second}
	m.SetConn(client)

	done := make(chan struct{})
	go func() {
		defer close(done)

		buf := make([]byte, len("BINARY\r\n"))
		if _, err := io.ReadFull(server, buf); err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if string(buf) != "BINARY\r\n" {
			t.Errorf("unexpected command: %q", string(buf))
			return
		}
		_, _ = server.Write([]byte("0\n"))
	}()

	if err := m.EnterBinaryMode(); err != nil {
		t.Fatalf("EnterBinaryMode: %v", err)
	}
	if m.Mode != ModeBinary {
		t.Fatalf("mode not updated: %v", m.Mode)
	}

	<-done
}

func TestCreateBufferSendsMask(t *testing.T) {
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

		if hdr[2] != opCreateBuffer || hdr[3] != 1 {
			t.Errorf("unexpected op/dev: %02x/%d", hdr[2], hdr[3])
		}
		if binary.BigEndian.Uint32(hdr[4:]) != 0 {
			t.Errorf("expected code 0, got %d", binary.BigEndian.Uint32(hdr[4:]))
		}

		payload := make([]byte, 4)
		if _, err := io.ReadFull(server, payload); err != nil {
			t.Errorf("read payload: %v", err)
			return
		}

		if got := binary.LittleEndian.Uint32(payload); got != 0x29 {
			t.Errorf("unexpected mask: 0x%x", got)
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = 1
		binary.BigEndian.PutUint32(resp[4:8], uint32(len(payload)))
		_, _ = server.Write(resp[:])
		_, _ = server.Write(payload)
	}()

	buf, err := m.CreateBuffer(1, []uint8{0, 3, 5}, false)
	if err != nil {
		t.Fatalf("CreateBuffer: %v", err)
	}
	if buf.ID != 0 || buf.Dev != 1 {
		t.Fatalf("buffer metadata mismatch: %+v", buf)
	}
	if len(buf.Channels) != 3 || buf.Channels[0] != 0 || buf.Channels[1] != 3 || buf.Channels[2] != 5 {
		t.Fatalf("channels not sorted: %+v", buf.Channels)
	}

	<-done
}

func TestTransferBlockReadsPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	buf := &Buffer{ID: 1, Dev: 2}
	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		sizePayload := make([]byte, 8)

		// CREATE_BLOCK
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read create hdr: %v", err)
			return
		}
		if hdr[2] != opCreateBlock {
			t.Errorf("unexpected create op: %02x", hdr[2])
			return
		}
		if _, err := io.ReadFull(server, sizePayload); err != nil {
			t.Errorf("read size payload: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = buf.Dev
		binary.BigEndian.PutUint32(resp[4:8], 0)
		_, _ = server.Write(resp[:])

		// TRANSFER_BLOCK
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read transfer hdr: %v", err)
			return
		}
		if hdr[2] != opTransferBlock {
			t.Errorf("unexpected transfer op: %02x", hdr[2])
			return
		}
		if _, err := io.ReadFull(server, sizePayload); err != nil {
			t.Errorf("read transfer payload: %v", err)
			return
		}

		payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = buf.Dev
		binary.BigEndian.PutUint32(resp[4:8], uint32(len(payload)))
		_, _ = server.Write(resp[:])
		_, _ = server.Write(payload)
	}()

	blk, err := m.CreateBlock(buf, 8)
	if err != nil {
		t.Fatalf("CreateBlock: %v", err)
	}
	dst := make([]byte, 4) // intentionally small to exercise discard
	n, err := m.TransferBlock(blk, dst)
	if err != nil {
		t.Fatalf("TransferBlock: %v", err)
	}
	if n != 8 {
		t.Fatalf("expected 8 bytes, got %d", n)
	}
	if dst[0] != 1 || dst[1] != 2 || dst[2] != 3 || dst[3] != 4 {
		t.Fatalf("unexpected dst contents: %v", dst)
	}

	<-done
}

func TestStartRXStreamStopsOnSignal(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	buf := &Buffer{ID: 0, Dev: 1}
	blk := &Block{ID: 0, Size: 4, buffer: buf}

	stop := make(chan struct{})
	out := make(chan []byte, 2)
	errCh := make(chan error, 1)

	go func() {
		errCh <- m.StartRXStream(buf, blk, out, stop, StreamQueueConfig{})
	}()

	go func() {
		defer server.Close()
		var hdr [8]byte
		sizePayload := make([]byte, 8)

		for i := 0; ; i++ {
			if _, err := io.ReadFull(server, hdr[:]); err != nil {
				return
			}
			if _, err := io.ReadFull(server, sizePayload); err != nil {
				return
			}

			payload := []byte{}
			if i < 2 {
				payload = []byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)}
			}

			var resp [8]byte
			binary.BigEndian.PutUint16(resp[0:2], 0)
			resp[2] = opResponse
			resp[3] = buf.Dev
			binary.BigEndian.PutUint32(resp[4:8], uint32(len(payload)))
			_, _ = server.Write(resp[:])
			if len(payload) > 0 {
				_, _ = server.Write(payload)
			}

			select {
			case <-stop:
				return
			default:
			}
		}
	}()

	frame1 := <-out
	frame2 := <-out
	if len(frame1) != 4 || len(frame2) != 4 {
		t.Fatalf("unexpected frame lengths: %d %d", len(frame1), len(frame2))
	}

	close(stop)
	if err := <-errCh; err != nil {
		t.Fatalf("StartRXStream error: %v", err)
	}
}

func TestTransferTxBlockTracksInFlight(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	buf := &Buffer{ID: 0, Dev: 1, inFlight: make(map[uint16]int)}
	blk := &Block{ID: 0, Size: 4, buffer: buf}

	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		sizePayload := make([]byte, 8)
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read transfer hdr: %v", err)
			return
		}
		if _, err := io.ReadFull(server, sizePayload); err != nil {
			t.Errorf("read size: %v", err)
			return
		}
		payload := make([]byte, binary.LittleEndian.Uint64(sizePayload))
		if _, err := io.ReadFull(server, payload); err != nil {
			t.Errorf("read payload: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = buf.Dev
		binary.BigEndian.PutUint32(resp[4:8], 0)
		_, _ = server.Write(resp[:])
	}()

	if buf.inFlight[blk.ID] != 0 {
		t.Fatalf("expected clean in-flight state")
	}
	if _, err := m.TransferTxBlock(blk, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("TransferTxBlock: %v", err)
	}
	if buf.inFlight[blk.ID] != 0 {
		t.Fatalf("in-flight counter not cleared: %d", buf.inFlight[blk.ID])
	}

	<-done
}

func TestStreamQueueSignalsWatermarks(t *testing.T) {
	highCh := make(chan struct{}, 1)
	lowCh := make(chan struct{}, 1)
	q := newStreamQueue(StreamQueueConfig{Depth: 3, HighWatermark: 2, LowWatermark: 1, HighWatermarkCh: highCh, LowWatermarkCh: lowCh})

	stop := make(chan struct{})
	if err := q.enqueue([]byte{1}, stop); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := q.enqueue([]byte{2}, stop); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case <-highCh:
	case <-time.After(time.Second):
		t.Fatalf("high watermark not signaled")
	}

	if _, err := q.dequeue(stop); err != nil {
		t.Fatalf("dequeue: %v", err)
	}

	select {
	case <-lowCh:
	case <-time.After(time.Second):
		t.Fatalf("low watermark not signaled")
	}
}

func TestStartRXStreamHandlesJitteryConsumer(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	buf := &Buffer{ID: 0, Dev: 1}
	blk := &Block{ID: 0, Size: 4, buffer: buf}

	stop := make(chan struct{})
	out := make(chan []byte, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- m.StartRXStream(buf, blk, out, stop, StreamQueueConfig{Depth: 2, HighWatermark: 2, LowWatermark: 1})
	}()

	received := make([][]byte, 0, 5)
	go func() {
		var hdr [8]byte
		sizePayload := make([]byte, 8)
		for i := 0; i < 5; i++ {
			if _, err := io.ReadFull(server, hdr[:]); err != nil {
				return
			}
			if hdr[2] != opTransferBlock {
				t.Errorf("unexpected op %d", hdr[2])
				return
			}
			if _, err := io.ReadFull(server, sizePayload); err != nil {
				return
			}
			time.Sleep(time.Duration(i%2) * 20 * time.Millisecond)
			payload := []byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)}

			var resp [8]byte
			binary.BigEndian.PutUint16(resp[0:2], 0)
			resp[2] = opResponse
			resp[3] = buf.Dev
			binary.BigEndian.PutUint32(resp[4:8], uint32(len(payload)))
			_, _ = server.Write(resp[:])
			_, _ = server.Write(payload)
		}

		for {
			select {
			case <-stop:
				return
			default:
			}

			if _, err := io.ReadFull(server, hdr[:]); err != nil {
				return
			}
			if _, err := io.ReadFull(server, sizePayload); err != nil {
				return
			}

			var resp [8]byte
			binary.BigEndian.PutUint16(resp[0:2], 0)
			resp[2] = opResponse
			resp[3] = buf.Dev
			binary.BigEndian.PutUint32(resp[4:8], 0)
			_, _ = server.Write(resp[:])
		}
	}()

	deadline := time.After(2 * time.Second)
	for len(received) < 5 {
		select {
		case frame := <-out:
			received = append(received, frame)
			time.Sleep(15 * time.Millisecond)
		case <-deadline:
			t.Fatalf("timeout waiting for frames; got %d", len(received))
		}
	}
	close(stop)

	if err := <-errCh; err != nil {
		t.Fatalf("StartRXStream error: %v", err)
	}
	if len(received) != 5 {
		t.Fatalf("expected 5 frames, got %d", len(received))
	}
}

func TestStartTXStreamBackpressureAndShutdown(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	buf := &Buffer{ID: 0, Dev: 1}
	blk := &Block{ID: 0, Size: 4, buffer: buf}

	in := make(chan []byte, 4)
	for i := 0; i < cap(in); i++ {
		in <- []byte{byte(i), byte(i + 1), byte(i + 2), byte(i + 3)}
	}
	close(in)

	stop := make(chan struct{})
	errCh := make(chan error, 1)
	frames := make(chan []byte, 4)

	go func() {
		errCh <- m.StartTXStream(buf, blk, in, stop, StreamQueueConfig{Depth: 2, HighWatermark: 2, LowWatermark: 1})
	}()

	go func() {
		defer close(frames)
		var hdr [8]byte
		sizePayload := make([]byte, 8)
		for i := 0; i < 4; i++ {
			if _, err := io.ReadFull(server, hdr[:]); err != nil {
				return
			}
			if hdr[2] != opTransferBlock {
				t.Errorf("unexpected op %d", hdr[2])
				return
			}
			if _, err := io.ReadFull(server, sizePayload); err != nil {
				return
			}
			payload := make([]byte, binary.LittleEndian.Uint64(sizePayload))
			if _, err := io.ReadFull(server, payload); err != nil {
				return
			}
			time.Sleep(20 * time.Millisecond)
			frames <- payload

			var resp [8]byte
			binary.BigEndian.PutUint16(resp[0:2], 0)
			resp[2] = opResponse
			resp[3] = buf.Dev
			binary.BigEndian.PutUint32(resp[4:8], 0)
			_, _ = server.Write(resp[:])
		}
	}()

	deadline := time.After(2 * time.Second)
	received := 0
	for received < 4 {
		select {
		case frame, ok := <-frames:
			if !ok {
				t.Fatalf("frames channel closed early after %d", received)
			}
			if len(frame) == 0 {
				t.Fatalf("empty frame received")
			}
			received++
		case <-deadline:
			t.Fatalf("timeout waiting for tx frames; got %d", received)
		}
	}
	close(stop)

	if err := <-errCh; err != nil {
		t.Fatalf("StartTXStream error: %v", err)
	}
}
