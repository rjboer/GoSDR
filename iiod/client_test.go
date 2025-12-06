package iiod

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
)

type mockCase struct {
	name        string
	invoke      func(*Client) (string, error)
	request     string
	status      int
	payload     string
	header      string
	wantsErr    bool
	wantPayload string
}

func TestClientCommands(t *testing.T) {
	cases := []mockCase{
		{
			name:    "context info",
			request: "VERSION",
			status:  0,
			payload: "1 0 Test IIOD",
			invoke: func(c *Client) (string, error) {
				info, err := c.GetContextInfo()
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("%d.%d %s", info.Major, info.Minor, info.Description), nil
			},
			wantPayload: "1.0 Test IIOD",
		},
		{
			name:        "list devices",
			request:     "LIST_DEVICES",
			status:      0,
			payload:     "adc dac",
			wantPayload: "adc dac",
			invoke: func(c *Client) (string, error) {
				devices, err := c.ListDevices()
				if err != nil {
					return "", err
				}
				return strings.Join(devices, " "), nil
			},
		},
		{
			name:        "get channels",
			request:     "LIST_CHANNELS adc",
			status:      0,
			payload:     "voltage0 voltage1",
			wantPayload: "voltage0 voltage1",
			invoke: func(c *Client) (string, error) {
				channels, err := c.GetChannels("adc")
				if err != nil {
					return "", err
				}
				return strings.Join(channels, " "), nil
			},
		},
		{
			name:        "create buffer",
			request:     "CREATE_BUFFER adc 1024",
			status:      0,
			payload:     "buffer-id",
			wantPayload: "buffer-id",
			invoke: func(c *Client) (string, error) {
				return c.CreateBuffer("adc", 1024)
			},
		},
		{
			name:    "read attr",
			request: "READ_ATTR adc voltage0 sampling_frequency",
			status:  0,
			payload: "2000000",
			invoke: func(c *Client) (string, error) {
				return c.ReadAttr("adc", "voltage0", "sampling_frequency")
			},
			wantPayload: "2000000",
		},
		{
			name:    "write attr",
			request: "WRITE_ATTR adc voltage0 sampling_frequency 1000000",
			status:  0,
			payload: "",
			invoke: func(c *Client) (string, error) {
				return "", c.WriteAttr("adc", "voltage0", "sampling_frequency", "1000000")
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			addr, serverErr := startMockServer(t, tc.request, tc.status, tc.payload, tc.header)
			client, err := Dial(addr)
			if err != nil {
				t.Fatalf("Dial failed: %v", err)
			}
			defer client.Close()

			payload, err := tc.invoke(client)
			if tc.wantsErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if payload != tc.wantPayload {
					t.Fatalf("unexpected payload: %q", payload)
				}
			}

			if err := <-serverErr; err != nil {
				t.Fatalf("server error: %v", err)
			}
		})
	}
}

func TestSendErrors(t *testing.T) {
	cases := []mockCase{
		{
			name:     "malformed header",
			request:  "VERSION",
			header:   "MALFORMED\n",
			invoke:   func(c *Client) (string, error) { return c.sendCommandString(context.Background(), "VERSION") },
			wantsErr: true,
		},
		{
			name:     "non zero status",
			request:  "LIST_DEVICES",
			status:   5,
			payload:  "error",
			invoke:   func(c *Client) (string, error) { return c.sendCommandString(context.Background(), "LIST_DEVICES") },
			wantsErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, serverErr := startMockServer(t, tc.request, tc.status, tc.payload, tc.header)
			client, err := Dial(addr)
			if err != nil {
				t.Fatalf("Dial failed: %v", err)
			}
			defer client.Close()

			if _, err := tc.invoke(client); err == nil {
				t.Fatalf("expected error")
			}

			if err := <-serverErr; err != nil {
				t.Fatalf("server error: %v", err)
			}
		})
	}
}

func TestListDevicesBinary(t *testing.T) {
	const opcodeListDevices = 2
	devicePayload := []byte("adc dac")
	addr, serverErr := startBinaryListDevicesServer(t, opcodeListDevices, int32(len(devicePayload)), devicePayload, "<context></context>")
	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	devices, err := client.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}

	if got, want := strings.Join(devices, " "), string(devicePayload); got != want {
		t.Fatalf("unexpected devices: got %q want %q", got, want)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestListDevicesFallbackToXML(t *testing.T) {
	const opcodeListDevices = 2
	xmlPayload := "<context><device id=\"adc\"></device><device id=\"dac\"></device></context>"
	addr, serverErr := startBinaryListDevicesServer(t, opcodeListDevices, 0, nil, xmlPayload)
	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	devices, err := client.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}

	if got, want := strings.Join(devices, " "), "adc dac"; got != want {
		t.Fatalf("unexpected devices: got %q want %q", got, want)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	client := &Client{}
	if err := client.Close(); err == nil {
		t.Fatalf("expected error closing nil client")
	}

	conn1, conn2 := net.Pipe()
	client = &Client{conn: conn1, reader: bufio.NewReader(conn1)}
	conn2.Close()

	if err := client.Close(); err != nil {
		t.Fatalf("expected first close to succeed: %v", err)
	}

	if err := client.Close(); err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("expected not connected error, got %v", err)
	}
}

func startMockServer(t *testing.T, expectedReq string, status int, payload, headerOverride string) (string, chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer listener.Close()

		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		for strings.TrimSpace(line) == "PRINT" {
			xmlPayload := "<context></context>"
			if _, err := fmt.Fprintf(conn, "0 %d\n%s", len(xmlPayload), xmlPayload); err != nil {
				errCh <- err
				return
			}
			line, err = reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
		}
		if strings.TrimSpace(line) != expectedReq {
			errCh <- fmt.Errorf("unexpected request %q", strings.TrimSpace(line))
			return
		}

		header := headerOverride
		if header == "" {
			header = fmt.Sprintf("%d %d\n", status, len(payload))
		}
		if _, err := fmt.Fprint(conn, header); err != nil {
			errCh <- err
			return
		}
		if payload != "" && headerOverride == "" {
			if _, err := fmt.Fprint(conn, payload); err != nil {
				errCh <- err
				return
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
}

func startBinaryListDevicesServer(t *testing.T, expectedOpcode uint8, status int32, payload []byte, xmlContext string) (string, chan error) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer listener.Close()

		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}

		if strings.TrimSpace(line) == "PRINT" {
			header := fmt.Sprintf("0 %d\n%s", len(xmlContext), xmlContext)
			if _, err := fmt.Fprint(conn, header); err != nil {
				errCh <- err
				return
			}
		}

		header := make([]byte, 8)
		if _, err := io.ReadFull(reader, header); err != nil {
			errCh <- err
			return
		}

		if header[0] != expectedOpcode {
			errCh <- fmt.Errorf("unexpected opcode %d", header[0])
			return
		}

		payloadLen := binary.BigEndian.Uint32(header[4:])
		if payloadLen > 0 {
			received := make([]byte, payloadLen)
			if _, err := io.ReadFull(reader, received); err != nil {
				errCh <- err
				return
			}
		}

		if err := binary.Write(conn, binary.BigEndian, status); err != nil {
			errCh <- err
			return
		}

		if status > 0 {
			if _, err := conn.Write(payload[:status]); err != nil {
				errCh <- err
				return
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
}
