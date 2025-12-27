package connectionmgr

import (
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestSetTimeoutASCIISuccess(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan struct{})
	var received string
	go func() {
		defer close(done)

		buf := make([]byte, 64)
		n, _ := server.Read(buf)
		received = string(buf[:n])

		writeIntegerLine(t, server, 0)
	}()

	if err := mgr.SetTimeoutASCII(1500); err != nil {
		t.Fatalf("SetTimeoutASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "TIMEOUT 1500") {
		t.Fatalf("unexpected command sent: %q", received)
	}
}

func TestSetTimeoutASCIINegativeValidation(t *testing.T) {
	mgr := &Manager{}
	if err := mgr.SetTimeoutASCII(-1); err == nil {
		t.Fatalf("expected validation error for negative timeout")
	}
}

func TestSetTimeoutASCIINegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		buf := make([]byte, 32)
		server.Read(buf) // consume command
		writeIntegerLine(t, server, -110)
	}()

	err := mgr.SetTimeoutASCII(500)
	if err == nil || !strings.Contains(err.Error(), "-110") {
		t.Fatalf("expected errno error, got: %v", err)
	}
}

func writeIntegerLine(t *testing.T, conn net.Conn, val int) {
	t.Helper()

	payload := make([]byte, 64)
	copy(payload, []byte(fmt.Sprintf("%d", val)))
	payload[len(payload)-1] = '\n'

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("failed to write status line: %v", err)
	}
}
