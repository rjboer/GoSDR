package connectionmgr

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
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

func TestReadChannelAttrASCIIInput(t *testing.T) {
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

		writeIntegerLine(t, server, 4)
		server.Write([]byte("beep\n"))
	}()

	value, err := mgr.ReadChannelAttrASCII("ad9361-phy", false, "voltage0", "raw")
	if err != nil {
		t.Fatalf("ReadChannelAttrASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "READ ad9361-phy INPUT voltage0 raw") {
		t.Fatalf("unexpected command sent: %q", received)
	}
	if value != "beep" {
		t.Fatalf("unexpected value returned: %q", value)
	}
}

func TestReadChannelAttrASCIIOutput(t *testing.T) {
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

		writeIntegerLine(t, server, 3)
		server.Write([]byte("yay\n"))
	}()

	value, err := mgr.ReadChannelAttrASCII("ad9361-phy", true, "voltage1", "raw")
	if err != nil {
		t.Fatalf("ReadChannelAttrASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "READ ad9361-phy OUTPUT voltage1 raw") {
		t.Fatalf("unexpected command sent: %q", received)
	}
	if value != "yay" {
		t.Fatalf("unexpected value returned: %q", value)
	}
}

func TestReadChannelAttrASCIINegativeLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 128))
		writeIntegerLine(t, server, -4)
	}()

	if _, err := mgr.ReadChannelAttrASCII("pluto", false, "voltage0", "status"); err == nil || !strings.Contains(err.Error(), "negative length") {
		t.Fatalf("expected negative length error, got %v", err)
	}
}

func TestReadBufferAttrASCIISuccess(t *testing.T) {
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
		server.Write([]byte("value\r\n"))
	}()

	value, err := mgr.ReadBufferAttrASCII("cf-ad9361-lpc", "samples")
	if err != nil {
		t.Fatalf("ReadBufferAttrASCII returned error: %v", err)
	}

	<-done

	if !strings.HasPrefix(received, "READ cf-ad9361-lpc BUFFER samples") {
		t.Fatalf("unexpected command sent: %q", received)
	}
	if value != "value" {
		t.Fatalf("unexpected value returned: %q", value)
	}
}

func TestReadBufferAttrASCIINegativeLength(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		server.Read(make([]byte, 64))
		writeIntegerLine(t, server, -7)
	}()

	if _, err := mgr.ReadBufferAttrASCII("cf-ad9361-lpc", "direction"); err == nil || !strings.Contains(err.Error(), "negative length") {
		t.Fatalf("expected negative length error, got %v", err)
	}
}

func TestWriteDeviceAttrASCIIPayloadOrdering(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)

		line, err := reader.ReadString('\n')
		if err != nil {
			done <- fmt.Errorf("failed to read command line: %w", err)
			return
		}

		value := "abc"
		expected := fmt.Sprintf("WRITE ad9361-phy gain %d\r\n", len(value))
		if line != expected {
			done <- fmt.Errorf("unexpected command line: %q", line)
			return
		}

		payload := make([]byte, len(value))
		if _, err := io.ReadFull(reader, payload); err != nil {
			done <- fmt.Errorf("failed to read payload: %w", err)
			return
		}
		if string(payload) != value {
			done <- fmt.Errorf("payload mismatch: %q", payload)
			return
		}

		_ = server.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		if _, err := reader.Peek(1); err == nil {
			done <- fmt.Errorf("unexpected data after payload")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("peek error after payload: %w", err)
			return
		}
		_ = server.SetReadDeadline(time.Time{})

		writeIntegerLine(t, server, 0)
		done <- nil
	}()

	status, err := mgr.WriteDeviceAttrASCII("ad9361-phy", "gain", "abc")
	if err != nil {
		t.Fatalf("WriteDeviceAttrASCII returned error: %v", err)
	}
	if status != 0 {
		t.Fatalf("expected status 0, got %d", status)
	}

	if goroutineErr := <-done; goroutineErr != nil {
		t.Fatalf("server goroutine error: %v", goroutineErr)
	}
}

func TestWriteDeviceAttrASCIIEmptyPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)

		line, err := reader.ReadString('\n')
		if err != nil {
			done <- fmt.Errorf("failed to read command line: %w", err)
			return
		}

		expected := "WRITE pluto status 0\r\n"
		if line != expected {
			done <- fmt.Errorf("unexpected command line: %q", line)
			return
		}

		_ = server.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		if _, err := reader.Peek(1); err == nil {
			done <- fmt.Errorf("unexpected payload bytes")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("peek error: %w", err)
			return
		}
		_ = server.SetReadDeadline(time.Time{})

		writeIntegerLine(t, server, 0)
		done <- nil
	}()

	status, err := mgr.WriteDeviceAttrASCII("pluto", "status", "")
	if err != nil {
		t.Fatalf("WriteDeviceAttrASCII returned error: %v", err)
	}
	if status != 0 {
		t.Fatalf("expected status 0, got %d", status)
	}

	if goroutineErr := <-done; goroutineErr != nil {
		t.Fatalf("server goroutine error: %v", goroutineErr)
	}
}

func TestWriteDeviceAttrASCIIInvalidStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		reader := bufio.NewReader(server)
		reader.ReadString('\n')
		io.ReadFull(reader, make([]byte, len("value")))
		writeRawStatusLine(t, server, "not-an-int")
	}()

	if _, err := mgr.WriteDeviceAttrASCII("pluto", "status", "value"); err == nil || !strings.Contains(err.Error(), "parse integer") {
		t.Fatalf("expected integer parse error, got %v", err)
	}
}

func TestWriteDeviceAttrASCIIErrorStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		reader := bufio.NewReader(server)
		reader.ReadString('\n')
		io.ReadFull(reader, make([]byte, len("payload")))
		writeIntegerLine(t, server, -22)
	}()

	status, err := mgr.WriteDeviceAttrASCII("pluto", "status", "payload")
	if err != nil {
		t.Fatalf("WriteDeviceAttrASCII returned error: %v", err)
	}
	if status != -22 {
		t.Fatalf("expected status -22, got %d", status)
	}
}

func TestWriteChannelAttrASCIIInputPayloadOrdering(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)

		line, err := reader.ReadString('\n')
		if err != nil {
			done <- fmt.Errorf("failed to read command line: %w", err)
			return
		}

		value := "abc123"
		expected := fmt.Sprintf("WRITE ad9361-phy INPUT voltage0 raw %d\r\n", len(value))
		if line != expected {
			done <- fmt.Errorf("unexpected command line: %q", line)
			return
		}

		payload := make([]byte, len(value))
		if _, err := io.ReadFull(reader, payload); err != nil {
			done <- fmt.Errorf("failed to read payload: %w", err)
			return
		}
		if string(payload) != value {
			done <- fmt.Errorf("payload mismatch: %q", payload)
			return
		}

		_ = server.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		if _, err := reader.Peek(1); err == nil {
			done <- fmt.Errorf("unexpected data after payload")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("peek error after payload: %w", err)
			return
		}
		_ = server.SetReadDeadline(time.Time{})

		writeIntegerLine(t, server, 0)
		done <- nil
	}()

	status, err := mgr.WriteChannelAttrASCII("ad9361-phy", false, "voltage0", "raw", "abc123")
	if err != nil {
		t.Fatalf("WriteChannelAttrASCII returned error: %v", err)
	}
	if status != 0 {
		t.Fatalf("expected status 0, got %d", status)
	}

	if goroutineErr := <-done; goroutineErr != nil {
		t.Fatalf("server goroutine error: %v", goroutineErr)
	}
}

func TestWriteChannelAttrASCIIOutputErrors(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)

		line, err := reader.ReadString('\n')
		if err != nil {
			done <- fmt.Errorf("failed to read command line: %w", err)
			return
		}

		expected := "WRITE cf-ad9361-lpc OUTPUT voltage1 calibscale 0\r\n"
		if line != expected {
			done <- fmt.Errorf("unexpected command line: %q", line)
			return
		}

		_ = server.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		if _, err := reader.Peek(1); err == nil {
			done <- fmt.Errorf("unexpected payload bytes")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("peek error: %w", err)
			return
		}
		_ = server.SetReadDeadline(time.Time{})

		writeIntegerLine(t, server, -110)
		done <- nil
	}()

	status, err := mgr.WriteChannelAttrASCII("cf-ad9361-lpc", true, "voltage1", "calibscale", "")
	if err != nil {
		t.Fatalf("WriteChannelAttrASCII returned error: %v", err)
	}
	if status != -110 {
		t.Fatalf("expected status -110, got %d", status)
	}

	if goroutineErr := <-done; goroutineErr != nil {
		t.Fatalf("server goroutine error: %v", goroutineErr)
	}
}

func TestWriteChannelAttrASCIIInvalidStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	mgr := &Manager{Mode: ModeASCII, conn: client}

	go func() {
		reader := bufio.NewReader(server)
		reader.ReadString('\n')
		writeRawStatusLine(t, server, "not-a-number")
	}()

	if _, err := mgr.WriteChannelAttrASCII("pluto", false, "voltage2", "offset", "7"); err == nil || !strings.Contains(err.Error(), "parse integer") {
		t.Fatalf("expected integer parse error, got %v", err)
	}
}

func writeRawStatusLine(t *testing.T, conn net.Conn, raw string) {
	t.Helper()

	payload := make([]byte, 64)
	copy(payload, []byte(raw))
	payload[len(payload)-1] = '\n'

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("failed to write raw status line: %v", err)
	}
}
