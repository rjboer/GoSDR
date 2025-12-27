package connectionmgr

import (
	"net"
	"strings"
	"testing"
)

func TestReadDeviceAttrASCIISuccess(t *testing.T) {
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

		writeIntegerLine(t, server, 5)
		server.Write([]byte("hello\n"))
	}()

	value, err := mgr.ReadDeviceAttrASCII("ad9361-phy", "gain")
	if err != nil {
		t.Fatalf("ReadDeviceAttrASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "READ ad9361-phy gain") {
		t.Fatalf("unexpected command sent: %q", received)
	}
	if value != "hello" {
		t.Fatalf("unexpected value returned: %q", value)
	}
}

func TestReadDeviceAttrASCIIEmpty(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 64))
		writeIntegerLine(t, server, 0)
		server.Write([]byte("\n"))
	}()

	value, err := mgr.ReadDeviceAttrASCII("pluto", "status")
	if err != nil {
		t.Fatalf("ReadDeviceAttrASCII returned error: %v", err)
	}
	if value != "" {
		t.Fatalf("expected empty value, got %q", value)
	}
}

func TestReadDeviceAttrASCIINegativeLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 64))
		writeIntegerLine(t, server, -3)
	}()

	if _, err := mgr.ReadDeviceAttrASCII("pluto", "status"); err == nil || !strings.Contains(err.Error(), "negative length") {
		t.Fatalf("expected negative length error, got %v", err)
	}
}

func TestReadDeviceAttrASCIIShortPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		defer server.Close()

		server.Read(make([]byte, 64))
		writeIntegerLine(t, server, 4)
		server.Write([]byte("hi\n"))
	}()

	if _, err := mgr.ReadDeviceAttrASCII("pluto", "status"); err == nil || !strings.Contains(err.Error(), "failed to read 5 bytes") {
		t.Fatalf("expected short read error, got %v", err)
	}
}
