package connectionmgr

import (
	"bytes"
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
