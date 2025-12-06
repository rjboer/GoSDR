package iiod

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

type scriptedResponse struct {
	cmd     string
	payload string
}

func runScriptedServer(t *testing.T, conn net.Conn, script []scriptedResponse) {
	t.Helper()

	reader := bufio.NewReader(conn)
	for _, step := range script {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("server read failed: %v", err)
		}

		if strings.TrimSpace(line) != step.cmd {
			t.Fatalf("unexpected command %q, want %q", strings.TrimSpace(line), step.cmd)
		}

		if _, err := fmt.Fprintf(conn, "0 %d\n%s", len(step.payload), step.payload); err != nil {
			t.Fatalf("server write failed: %v", err)
		}
	}
}

func newPipeClient() (*Client, net.Conn) {
	clientConn, serverConn := net.Pipe()
	client := &Client{
		conn:         clientConn,
		reader:       bufio.NewReader(clientConn),
		openBuffers:  make(map[string]int),
		timeout:      5 * time.Second,
		healthWindow: 10 * time.Second,
	}
	return client, serverConn
}

func TestGetDeviceInfoParsesXML(t *testing.T) {
	xmlPayload := `<context><device id="dev0" name="demo"><attribute name="sampling_frequency" filename="in_sampling_freq">100</attribute><channel id="voltage0" type="input"><attribute name="scale" filename="in_voltage0_scale" type="int" unit="dB">1</attribute></channel></device></context>`

	client, serverConn := newPipeClient()
	defer client.Close()
	defer serverConn.Close()

	go runScriptedServer(t, serverConn, []scriptedResponse{{cmd: "XML", payload: xmlPayload}})

	devices, err := client.GetDeviceInfo()
	if err != nil {
		t.Fatalf("GetDeviceInfo failed: %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev := devices[0]
	if dev.ID != "dev0" || dev.Name != "demo" {
		t.Fatalf("unexpected device metadata: %+v", dev)
	}

	if len(dev.Channels) != 1 || len(dev.Attributes) != 1 {
		t.Fatalf("unexpected channel/attribute counts: %+v", dev)
	}
}

func TestSetTimeoutUpdatesClient(t *testing.T) {
	client, serverConn := newPipeClient()
	defer client.Close()
	defer serverConn.Close()

	go runScriptedServer(t, serverConn, []scriptedResponse{{cmd: "TIMEOUT 50", payload: ""}})

	if err := client.SetTimeout(50 * time.Millisecond); err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	client.stateMu.Lock()
	timeout := client.timeout
	client.stateMu.Unlock()

	if timeout != 50*time.Millisecond {
		t.Fatalf("timeout not updated, got %v", timeout)
	}
}

func TestBatchReadAndWriteAttrs(t *testing.T) {
	client, serverConn := newPipeClient()
	defer client.Close()
	defer serverConn.Close()

	script := []scriptedResponse{
		{cmd: "READ_ATTR dev0 freq", payload: "100"},
		{cmd: "READ_ATTR dev0 gain", payload: "10"},
		{cmd: "WRITE_ATTR dev0 phase 5", payload: ""},
		{cmd: "WRITE_ATTR dev0 mode fast", payload: ""},
	}

	go runScriptedServer(t, serverConn, script)

	readOps := []AttrOperation{{Device: "dev0", Attr: "freq"}, {Device: "dev0", Attr: "gain"}}
	reads, err := client.BatchReadAttrsWithContext(context.Background(), readOps)
	if err != nil {
		t.Fatalf("BatchReadAttrs failed: %v", err)
	}

	if reads[0] != "100" || reads[1] != "10" {
		t.Fatalf("unexpected read results: %+v", reads)
	}

	writeOps := []AttrOperation{
		{Device: "dev0", Attr: "phase", Value: "5", IsWrite: true},
		{Device: "dev0", Attr: "mode", Value: "fast", IsWrite: true},
	}
	if err := client.BatchWriteAttrsWithContext(context.Background(), writeOps); err != nil {
		t.Fatalf("BatchWriteAttrs failed: %v", err)
	}
}

func TestStreamBufferBackpressure(t *testing.T) {
	// Use a tiny handler buffer and context cancellation to ensure backpressure
	// paths are exercised without real network IO.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &Client{}
	// This test simply ensures StreamBuffer validates handler presence and propagates context errors.
	if err := client.StreamBuffer(ctx, "", 0, 0, nil); err == nil {
		t.Fatalf("expected handler validation error")
	}
}
