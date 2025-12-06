package iiod

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestGetContextInfoAndClose(t *testing.T) {
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

func TestDetectProtocolVersionFromXML(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}
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
		xml := "<?xml version=\"1.0\"?><context version-major=\"1\" version-minor=\"6\"></context>\n"
		if _, err := fmt.Fprint(serverConn, xml); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	version, err := client.DetectProtocolVersion(context.Background())
	if err != nil {
		t.Fatalf("DetectProtocolVersion failed: %v", err)
	}
	if version.Major != 1 || version.Minor != 6 {
		t.Fatalf("unexpected version: %+v", version)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestDetectProtocolVersionFallsBackToCommand(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}
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
		if _, err := fmt.Fprint(serverConn, "-22\n"); err != nil {
			serverErr <- err
			return
		}

		line, err = reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		if strings.TrimSpace(line) != "VERSION" {
			serverErr <- fmt.Errorf("unexpected fallback command %q", strings.TrimSpace(line))
			return
		}
		payload := "2 4 test"
		if _, err := fmt.Fprintf(serverConn, "0 %d\n%s", len(payload), payload); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	version, err := client.DetectProtocolVersion(context.Background())
	if err != nil {
		t.Fatalf("DetectProtocolVersion fallback failed: %v", err)
	}
	if version.Major != 2 || version.Minor != 4 {
		t.Fatalf("unexpected fallback version: %+v", version)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
