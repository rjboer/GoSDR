package connectionmgr

import (
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
