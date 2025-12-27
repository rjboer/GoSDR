package connectionmgr

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
)

func TestSetKernelBuffersCountASCIISuccess(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan struct{})
	var received string
	go func() {
		defer close(done)

		buf := make([]byte, 128)
		n, _ := server.Read(buf)
		received = string(buf[:n])

		writeIntegerLine(t, server, 0)
	}()

	if err := mgr.SetKernelBuffersCountASCII("cf-ad9361-lpc", 4); err != nil {
		t.Fatalf("SetKernelBuffersCountASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "SET cf-ad9361-lpc BUFFERS_COUNT 4") {
		t.Fatalf("unexpected command sent: %q", received)
	}
}

func TestSetKernelBuffersCountASCIINegativeValidation(t *testing.T) {
	mgr := &Manager{}
	if err := mgr.SetKernelBuffersCountASCII("cf-ad9361-lpc", -1); err == nil {
		t.Fatalf("expected validation error for negative count")
	}
}

func TestSetKernelBuffersCountASCIINegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		buf := make([]byte, 128)
		server.Read(buf) // consume command
		writeIntegerLine(t, server, -12)
	}()

	err := mgr.SetKernelBuffersCountASCII("cf-ad9361-lpc", 2)
	if err == nil || !strings.Contains(err.Error(), "-12") {
		t.Fatalf("expected errno error, got: %v", err)
	}
}

func TestReadBufferASCIIMaskLineConsumed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}
	payload := []byte{0xde, 0xad, 0xbe, 0xef}

	done := make(chan error, 1)
	go func() {
		defer close(done)

		recv := make([]byte, 128)
		server.Read(recv) // consume READBUF command

		writeIntegerLine(t, server, len(payload))
		writeStringLine(t, server, "00000003")
		if _, err := server.Write(payload); err != nil {
			done <- err
			return
		}
		server.Write([]byte("\n"))
		writeIntegerLine(t, server, 0)
		server.Close()
	}()

	dst := make([]byte, len(payload))

	n, err := mgr.ReadBufferASCII("cf-ad9361-lpc", dst)
	if err != nil {
		t.Fatalf("ReadBufferASCII returned error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes read, got %d", len(payload), n)
	}
	if !bytes.Equal(dst, payload) {
		t.Fatalf("payload contaminated by mask line: got %x want %x", dst, payload)
	}

	// Ensure trailing newline was consumed so the next integer can be read cleanly.
	next, err := mgr.readInteger()
	if err != nil {
		t.Fatalf("failed to read next integer: %v", err)
	}
	if next != 0 {
		t.Fatalf("unexpected next integer: got %d", next)
	}

	if err := <-done; err != nil {
		t.Fatalf("server goroutine failed: %v", err)
	}
}

func TestReadBufferASCIIStreamingResponder(t *testing.T) {
	payload := []byte{0x00, 0xff, '\n', 0x7f}

	client, responder := newASCIIMockResponder(t, []asciiMockStep{
		newReadbufStep("cf-ad9361-lpc", len(payload), "00000003", payload),
	})
	mgr := &Manager{Mode: ModeASCII, conn: client}

	dst := make([]byte, len(payload))

	n, err := mgr.ReadBufferASCII("cf-ad9361-lpc", dst)
	responder.wait(t)

	if err != nil {
		t.Fatalf("ReadBufferASCII returned error: %v", err)
	}
	if n != len(payload) {
		t.Fatalf("expected %d bytes read, got %d", len(payload), n)
	}
	if !bytes.Equal(dst, payload) {
		t.Fatalf("payload mismatch: got %x want %x", dst, payload)
	}
}

func TestReadBufferASCIINegativeLength(t *testing.T) {
	client, responder := newASCIIMockResponder(t, []asciiMockStep{
		{
			name:           "READBUF errno",
			expectLine:     "READBUF cf-ad9361-lpc 8\r\n",
			responseStatus: intPtr(-9),
		},
	})
	mgr := &Manager{Mode: ModeASCII, conn: client}

	dst := make([]byte, 8)

	n, err := mgr.ReadBufferASCII("cf-ad9361-lpc", dst)
	responder.wait(t)

	if err == nil || !strings.Contains(err.Error(), "-9") {
		t.Fatalf("expected errno error, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected zero bytes read on error, got %d", n)
	}
}

func TestReadBufferASCIIPayloadSizeMismatch(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan error, 1)
	go func() {
		defer close(done)

		recv := make([]byte, 128)
		server.Read(recv)

		writeIntegerLine(t, server, 4)
		writeStringLine(t, server, "00000003")
		if _, err := server.Write([]byte{0xaa, 0xbb}); err != nil {
			done <- err
			return
		}
		server.Write([]byte("\n"))
		server.Close()
	}()

	dst := make([]byte, 4)

	n, err := mgr.ReadBufferASCII("cf-ad9361-lpc", dst)

	if err == nil {
		t.Fatalf("expected short read error, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected unexpected EOF error, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected zero bytes read on error, got %d", n)
	}

	if err := <-done; err != nil {
		t.Fatalf("server goroutine failed: %v", err)
	}
}

func TestWriteBufferASCIISendsPayloadAndAlignsStream(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}
	payload := []byte("payload-bytes")

	done := make(chan error, 1)
	go func() {
		defer close(done)

		reader := bufio.NewReader(server)
		line, err := reader.ReadString('\n')
		if err != nil {
			done <- fmt.Errorf("failed to read command line: %w", err)
			return
		}

		expected := fmt.Sprintf("WRITEBUF %s %d\r\n", "cf-ad9361-dds", len(payload))
		if line != expected {
			done <- fmt.Errorf("unexpected command line: %q", line)
			return
		}

		buf := make([]byte, len(payload))
		if _, err := io.ReadFull(reader, buf); err != nil {
			done <- fmt.Errorf("failed to read payload: %w", err)
			return
		}
		if !bytes.Equal(buf, payload) {
			done <- fmt.Errorf("payload mismatch: %q", buf)
			return
		}

		writeIntegerLine(t, server, len(payload))
		writeIntegerLine(t, server, 0)
	}()

	written, err := mgr.WriteBufferASCII("cf-ad9361-dds", payload)
	if err != nil {
		t.Fatalf("WriteBufferASCII returned error: %v", err)
	}
	if written != len(payload) {
		t.Fatalf("expected %d bytes written, got %d", len(payload), written)
	}

	next, err := mgr.readInteger()
	if err != nil {
		t.Fatalf("failed to read next integer: %v", err)
	}
	if next != 0 {
		t.Fatalf("unexpected trailing integer: got %d", next)
	}

	if goroutineErr := <-done; goroutineErr != nil {
		t.Fatalf("server goroutine error: %v", goroutineErr)
	}
}

func TestWriteBufferASCIIPartialWrite(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}
	payload := []byte("12345")

	go func() {
		reader := bufio.NewReader(server)
		reader.ReadString('\n')
		io.ReadFull(reader, make([]byte, len(payload)))
		writeIntegerLine(t, server, len(payload)-2)
	}()

	written, err := mgr.WriteBufferASCII("cf-ad9361-dds", payload)
	if err == nil || !strings.Contains(err.Error(), "wrote") {
		t.Fatalf("expected partial write error, got %v", err)
	}
	if written != len(payload)-2 {
		t.Fatalf("expected written count %d, got %d", len(payload)-2, written)
	}
}

func TestWriteBufferASCIINegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}
	payload := []byte("abcdef")

	go func() {
		reader := bufio.NewReader(server)
		reader.ReadString('\n')
		io.ReadFull(reader, make([]byte, len(payload)))
		writeIntegerLine(t, server, -7)
	}()

	written, err := mgr.WriteBufferASCII("cf-ad9361-dds", payload)
	if err == nil || !strings.Contains(err.Error(), "-7") {
		t.Fatalf("expected negative status error, got %v", err)
	}
	if written != -7 {
		t.Fatalf("expected written count -7, got %d", written)
	}
}

func TestOpenBufferASCIIMockResponder(t *testing.T) {
	tests := []struct {
		name        string
		mask        string
		cyclic      bool
		samples     uint64
		expectLine  string
		responseVal int
	}{
		{
			name:        "non-cyclic",
			mask:        "00ff",
			cyclic:      false,
			samples:     1024,
			expectLine:  "OPEN iio:device0 1024 0x00ff\r\n",
			responseVal: 0,
		},
		{
			name:        "cyclic",
			mask:        "0X00fF",
			cyclic:      true,
			samples:     2048,
			expectLine:  "OPEN iio:device0 2048 0x00fF CYCLIC\r\n",
			responseVal: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, responder := newASCIIMockResponder(t, []asciiMockStep{
				{
					name:           "OPEN",
					expectLine:     tt.expectLine,
					responseStatus: intPtr(tt.responseVal),
				},
			})
			mgr := &Manager{Mode: ModeASCII, conn: client}

			err := mgr.OpenBufferASCII("iio:device0", tt.samples, tt.mask, tt.cyclic)
			responder.wait(t)

			if err != nil {
				t.Fatalf("OpenBufferASCII returned error: %v", err)
			}
		})
	}
}

func TestCloseBufferASCIIMockResponder(t *testing.T) {
	client, responder := newASCIIMockResponder(t, []asciiMockStep{
		{
			name:           "CLOSE",
			expectLine:     "CLOSE cf-ad9361-lpc\r\n",
			responseStatus: intPtr(0),
		},
	})
	mgr := &Manager{Mode: ModeASCII, conn: client}

	err := mgr.CloseBufferASCII("cf-ad9361-lpc")
	responder.wait(t)

	if err != nil {
		t.Fatalf("CloseBufferASCII returned error: %v", err)
	}
}

func TestWriteBufferASCIIMockResponder(t *testing.T) {
	payload := []byte("payload-bytes")

	client, responder := newASCIIMockResponder(t, []asciiMockStep{
		{
			name:             "WRITEBUF",
			expectLine:       fmt.Sprintf("WRITEBUF %s %d\r\n", "cf-ad9361-dds", len(payload)),
			expectPayloadLen: len(payload),
			expectPayload:    payload,
			responseStatus:   intPtr(len(payload)),
		},
	})
	mgr := &Manager{Mode: ModeASCII, conn: client}

	written, err := mgr.WriteBufferASCII("cf-ad9361-dds", payload)
	responder.wait(t)

	if err != nil {
		t.Fatalf("WriteBufferASCII returned error: %v", err)
	}
	if written != len(payload) {
		t.Fatalf("expected %d bytes written, got %d", len(payload), written)
	}
}

func TestOpenBufferASCIICommandFormatting(t *testing.T) {
	tests := []struct {
		name    string
		cyclic  bool
		maskHex string
		wantCmd string
	}{
		{
			name:    "non-cyclic",
			cyclic:  false,
			maskHex: "00ff",
			wantCmd: "OPEN iio:device0 1024 0x00ff\r\n",
		},
		{
			name:    "cyclic",
			cyclic:  true,
			maskHex: "0X00fF",
			wantCmd: "OPEN iio:device0 2048 0x00fF CYCLIC\r\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			mgr := &Manager{Mode: ModeASCII, conn: client}

			done := make(chan struct{})
			var received string
			go func() {
				defer close(done)

				buf := make([]byte, 128)
				n, _ := server.Read(buf)
				received = string(buf[:n])

				writeIntegerLine(t, server, 0)
			}()

			samples := uint64(1024)
			if tt.cyclic {
				samples = 2048
			}

			if err := mgr.OpenBufferASCII("iio:device0", samples, tt.maskHex, tt.cyclic); err != nil {
				t.Fatalf("OpenBufferASCII returned error: %v", err)
			}

			<-done

			if received != tt.wantCmd {
				t.Fatalf("unexpected command: got %q want %q", received, tt.wantCmd)
			}
		})
	}
}

func TestCloseBufferASCIICommandSentAndParsed(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan struct{})
	var received string
	go func() {
		defer close(done)

		buf := make([]byte, 128)
		n, _ := server.Read(buf)
		received = string(buf[:n])

		writeIntegerLine(t, server, 0)
	}()

	if err := mgr.CloseBufferASCII("cf-ad9361-lpc"); err != nil {
		t.Fatalf("CloseBufferASCII returned error: %v", err)
	}

	<-done

	if received != "CLOSE cf-ad9361-lpc\r\n" {
		t.Fatalf("unexpected command sent: %q", received)
	}
}

func TestCloseBufferASCIIPropagatesErrno(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		buf := make([]byte, 128)
		server.Read(buf) // consume CLOSE command
		writeIntegerLine(t, server, -6)
	}()

	err := mgr.CloseBufferASCII("cf-ad9361-lpc")
	if err == nil || !strings.Contains(err.Error(), "-6") {
		t.Fatalf("expected errno error, got: %v", err)
	}
}

func TestOpenBufferASCIINegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		buf := make([]byte, 128)
		server.Read(buf) // consume OPEN command
		writeIntegerLine(t, server, -22)
	}()

	err := mgr.OpenBufferASCII("cf-ad9361-lpc", 512, "03", false)
	if err == nil || !strings.Contains(err.Error(), "-22") {
		t.Fatalf("expected errno error, got: %v", err)
	}
}
