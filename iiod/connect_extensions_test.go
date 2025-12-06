package iiod

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
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
		conn:   clientConn,
		reader: bufio.NewReader(clientConn),
	}
	return client, serverConn
}

func TestListDevicesFromXMLParsesIDs(t *testing.T) {
	xmlPayload := "<?xml version=\"1.0\"?>\n<context version-major=\"1\" version-minor=\"0\">\n<device id=\"dev0\" name=\"demo\"></device>\n<device id=\"dev1\" name=\"aux\"></device>\n</context>\n"

	client, serverConn := newPipeClient()
	defer client.Close()
	defer serverConn.Close()

	serverErr := make(chan error, 1)
	go func() {
		defer close(serverErr)
		reader := bufio.NewReader(serverConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		if strings.TrimSpace(line) != "PRINT" {
			serverErr <- fmt.Errorf("unexpected command %q", strings.TrimSpace(line))
			return
		}
		if _, err := fmt.Fprint(serverConn, xmlPayload); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	devices, err := client.ListDevicesFromXML(context.Background())
	if err != nil {
		t.Fatalf("ListDevicesFromXML failed: %v", err)
	}

	if len(devices) != 2 || devices[0] != "dev0" || devices[1] != "dev1" {
		t.Fatalf("unexpected devices parsed: %v", devices)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
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

	reads, err := client.BatchReadAttrsWithContext(context.Background(), []AttrOperation{
		{Device: "dev0", Attr: "freq"},
		{Device: "dev0", Attr: "gain"},
	})
	if err != nil {
		t.Fatalf("BatchReadAttrsWithContext failed: %v", err)
	}

	if len(reads) != 2 || reads[0] != "100" || reads[1] != "10" {
		t.Fatalf("unexpected read results: %+v", reads)
	}

	if err := client.BatchWriteAttrsWithContext(context.Background(), []AttrOperation{
		{Device: "dev0", Attr: "phase", Value: "5", IsWrite: true},
		{Device: "dev0", Attr: "mode", Value: "fast", IsWrite: true},
	}); err != nil {
		t.Fatalf("BatchWriteAttrsWithContext failed: %v", err)
	}
}
