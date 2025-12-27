package connectionmgr

import (
	"net"
	"strings"
	"testing"
)

func TestGetTriggerASCIISuccess(t *testing.T) {
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

		writeIntegerLine(t, server, 4)
		server.Write([]byte("trig\n"))
	}()

	trigger, err := mgr.GetTriggerASCII("ad9361-phy")
	if err != nil {
		t.Fatalf("GetTriggerASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "GETTRIG ad9361-phy") {
		t.Fatalf("unexpected command sent: %q", received)
	}
	if trigger != "trig" {
		t.Fatalf("unexpected trigger returned: %q", trigger)
	}
}

func TestGetTriggerASCIIEmptyValue(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 32))
		writeIntegerLine(t, server, 0)
		server.Write([]byte("\n"))
	}()

	trigger, err := mgr.GetTriggerASCII("pluto")
	if err != nil {
		t.Fatalf("GetTriggerASCII returned error: %v", err)
	}
	if trigger != "" {
		t.Fatalf("expected empty trigger, got %q", trigger)
	}
}

func TestGetTriggerASCIINegativeLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 32))
		writeIntegerLine(t, server, -5)
	}()

	if _, err := mgr.GetTriggerASCII("pluto"); err == nil || !strings.Contains(err.Error(), "-5") {
		t.Fatalf("expected negative length error, got %v", err)
	}
}

func TestSetTriggerASCIIWithName(t *testing.T) {
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

	if err := mgr.SetTriggerASCII("pluto", "external"); err != nil {
		t.Fatalf("SetTriggerASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "SETTRIG pluto external") {
		t.Fatalf("unexpected command sent: %q", received)
	}
}

func TestSetTriggerASCIIEmptyName(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 32))
		writeIntegerLine(t, server, 0)
	}()

	if err := mgr.SetTriggerASCII("pluto", ""); err != nil {
		t.Fatalf("SetTriggerASCII returned error: %v", err)
	}
}

func TestSetTriggerASCIINegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 32))
		writeIntegerLine(t, server, -22)
	}()

	err := mgr.SetTriggerASCII("pluto", "external")
	if err == nil || !strings.Contains(err.Error(), "-22") {
		t.Fatalf("expected errno error, got %v", err)
	}
}
