package iiod

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestGetContextInfoAndClose(t *testing.T) {
	t.Skip("iiod client mocks disabled")
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}

	serverErr := make(chan error, 1)
	go func() {
		defer serverConn.Close()

		reader := bufio.NewReader(serverConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}

		if strings.TrimSpace(line) != "VERSION" {
			serverErr <- fmt.Errorf("unexpected command %q", strings.TrimSpace(line))
			return
		}

		payload := "1 2 Some IIOD"
		if _, err := fmt.Fprintf(serverConn, "0 %d\n%s", len(payload), payload); err != nil {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	info, err := client.GetContextInfo()
	if err != nil {
		t.Fatalf("GetContextInfo failed: %v", err)
	}

	if info.Major != 1 || info.Minor != 2 || info.Description != "Some IIOD" {
		t.Fatalf("unexpected context info: %+v", info)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
