package sdr

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/rjboer/GoSDR/iiod"
)

type plutoMockOp struct {
	cmd           string
	status        int
	payload       string
	binaryPayload []byte
	expectBinary  []byte
}

func startPlutoMockServer(t *testing.T, ops []plutoMockOp) (string, chan error) {
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

		for _, op := range ops {
			cmdStr, data, err := readPlutoCommand(reader)
			if err != nil {
				errCh <- fmt.Errorf("read command: %w", err)
				return
			}
			for cmdStr == "PRINT" {
				xmlPayload := "<context></context>"
				if err := sendPlutoResponse(conn, len(xmlPayload), []byte(xmlPayload)); err != nil {
					errCh <- fmt.Errorf("write xml response: %w", err)
					return
				}
				cmdStr, data, err = readPlutoCommand(reader)
				if err != nil {
					errCh <- fmt.Errorf("read command: %w", err)
					return
				}
			}

			if cmdStr != op.cmd {
				errCh <- fmt.Errorf("unexpected command %q, want %q", cmdStr, op.cmd)
				return
			}

			if len(op.expectBinary) > 0 {
				if string(data) != string(op.expectBinary) {
					errCh <- fmt.Errorf("binary payload mismatch: got %v want %v", data, op.expectBinary)
					return
				}
			}

			payload := []byte(op.payload)
			if len(op.binaryPayload) > 0 {
				payload = op.binaryPayload
			}

			if err := sendPlutoResponse(conn, op.status, payload); err != nil {
				errCh <- err
				return
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
}

const (
	plutoOpcodeVersion      = 0
	plutoOpcodePrint        = 1
	plutoOpcodeListDevices  = 2
	plutoOpcodeListChannels = 3
	plutoOpcodeOpenBuffer   = 4
	plutoOpcodeCloseBuffer  = 5
	plutoOpcodeWriteAttr    = 7
	plutoOpcodeReadBuffer   = 8
	plutoOpcodeWriteBuffer  = 9
)

func readPlutoCommand(reader *bufio.Reader) (string, []byte, error) {
	peek, err := reader.Peek(1)
	if err != nil {
		return "", nil, err
	}

	if peek[0] >= 'A' && peek[0] <= 'Z' {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		return strings.TrimSpace(line), nil, nil
	}

	header := make([]byte, 8)
	if _, err := io.ReadFull(reader, header); err != nil {
		return "", nil, err
	}

	cmd := iiod.IIODCommand{Opcode: header[0], Flags: header[1], Address: binary.BigEndian.Uint16(header[2:]), Length: binary.BigEndian.Uint32(header[4:])}
	payload := make([]byte, cmd.Length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return "", nil, err
	}

	return decodePlutoCommand(cmd, payload)
}

func decodePlutoCommand(cmd iiod.IIODCommand, payload []byte) (string, []byte, error) {
	switch cmd.Opcode {
	case plutoOpcodePrint:
		return "PRINT", nil, nil
	case plutoOpcodeVersion:
		return "VERSION", nil, nil
	case plutoOpcodeListDevices:
		return "LIST_DEVICES", nil, nil
	case plutoOpcodeListChannels:
		return fmt.Sprintf("LIST_CHANNELS %s", strings.TrimSpace(string(payload))), nil, nil
	case plutoOpcodeWriteAttr:
		target, value, err := parseWritePayload(payload)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("WRITE_ATTR %s %s", target, value), nil, nil
	case plutoOpcodeOpenBuffer, plutoOpcodeReadBuffer:
		device, count, err := parseDeviceCountPayload(payload)
		if err != nil {
			return "", nil, err
		}
		if cmd.Opcode == plutoOpcodeOpenBuffer {
			return fmt.Sprintf("OPEN %s %d", device, count), nil, nil
		}
		return fmt.Sprintf("READBUF %s %d", device, count), nil, nil
	case plutoOpcodeWriteBuffer:
		device, data, err := parseWriteBufferPayload(payload)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("WRITEBUF %s %d", device, len(data)), data, nil
	case plutoOpcodeCloseBuffer:
		return fmt.Sprintf("CLOSE %s", strings.TrimSpace(string(payload))), nil, nil
	default:
		return fmt.Sprintf("UNKNOWN_%d", cmd.Opcode), nil, nil
	}
}

func parseDeviceCountPayload(payload []byte) (string, uint64, error) {
	parts := bytes.SplitN(payload, []byte{'\n'}, 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("payload missing device separator")
	}
	if len(parts[1]) < 8 {
		return "", 0, fmt.Errorf("payload too short for count")
	}
	count := binary.BigEndian.Uint64(parts[1][:8])
	return string(parts[0]), count, nil
}

func parseWriteBufferPayload(payload []byte) (string, []byte, error) {
	parts := bytes.SplitN(payload, []byte{'\n'}, 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("payload missing device separator")
	}
	if len(parts[1]) < 8 {
		return "", nil, fmt.Errorf("payload too short for data length")
	}
	length := binary.BigEndian.Uint64(parts[1][:8])
	remaining := parts[1][8:]
	if uint64(len(remaining)) < length {
		return "", nil, fmt.Errorf("payload truncated: have %d want %d", len(remaining), length)
	}
	return string(parts[0]), remaining[:length], nil
}

func parseWritePayload(payload []byte) (string, string, error) {
	parts := bytes.SplitN(payload, []byte{'\n'}, 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("payload missing target separator")
	}
	if len(parts[1]) < 8 {
		return "", "", fmt.Errorf("payload too short for value length")
	}
	length := binary.BigEndian.Uint64(parts[1][:8])
	value := parts[1][8:]
	if uint64(len(value)) < length {
		return "", "", fmt.Errorf("payload truncated: have %d want %d", len(value), length)
	}
	return string(parts[0]), string(value[:length]), nil
}

func sendPlutoResponse(conn net.Conn, status int, payload []byte) error {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(status))
	if _, err := conn.Write(header); err != nil {
		return fmt.Errorf("write response header: %w", err)
	}

	if status < 0 || len(payload) == 0 {
		return nil
	}

	if _, err := conn.Write(payload); err != nil {
		return fmt.Errorf("write response payload: %w", err)
	}

	return nil
}

func TestPlutoBufferLifecycle(t *testing.T) {
	numSamples := 4
	iqPayload := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		binary.LittleEndian.PutUint16(iqPayload[i*4:], uint16(100+i))
		binary.LittleEndian.PutUint16(iqPayload[i*4+2:], uint16(200+i))
	}

	txIQ := []complex64{
		complex(0.25, -0.25),
		complex(-0.5, 0.5),
		complex(0.1, 0.2),
		complex(-0.1, -0.2),
	}
	txI, txQ := complexToIQ(txIQ)
	interleaved, err := iiod.InterleaveIQ([][][]int16{{txI, txQ}, {txI, txQ}})
	if err != nil {
		t.Fatalf("interleave tx data: %v", err)
	}
	txPayload := iiod.FormatInt16Samples(interleaved)

	ops := []plutoMockOp{
		{cmd: "LIST_DEVICES", status: len("ad9361-phy cf-ad9361-lpc cf-ad9361-dds"), payload: "ad9361-phy cf-ad9361-lpc cf-ad9361-dds"},
		{cmd: "WRITE_ATTR ad9361-phy sampling_frequency 2000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage1 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage0 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 hardwaregain 10", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 hardwaregain 11", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy out hardwaregain 0", status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-lpc", status: len("voltage0 voltage1"), payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-lpc", numSamples), status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-dds", status: len("voltage0 voltage1"), payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-dds", numSamples), status: 0, payload: ""},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: len(iqPayload), binaryPayload: iqPayload},
		{cmd: fmt.Sprintf("WRITEBUF %s %d", "cf-ad9361-dds", len(txPayload)), status: 0, expectBinary: txPayload},
		{cmd: fmt.Sprintf("CLOSE %s", "cf-ad9361-lpc"), status: 0, payload: ""},
		{cmd: fmt.Sprintf("CLOSE %s", "cf-ad9361-dds"), status: 0, payload: ""},
	}

	addr, errCh := startPlutoMockServer(t, ops)

	p := NewPluto()
	cfg := Config{
		URI:        addr,
		SampleRate: 2_000_000,
		RxLO:       2.3e9,
		RxGain0:    10,
		RxGain1:    11,
		TxGain:     0,
		NumSamples: numSamples,
		PhaseDelta: 0,
		ToneOffset: 0,
	}

	if err := p.Init(context.Background(), cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer p.Close()

	_, _, err = p.RX(context.Background())
	if err != nil {
		t.Fatalf("RX failed: %v", err)
	}

	if err := p.TX(context.Background(), txIQ, txIQ); err != nil {
		t.Fatalf("TX failed: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestPlutoRecoverableReadError(t *testing.T) {
	numSamples := 2
	iqPayload := make([]byte, numSamples*4)
	for i := 0; i < numSamples; i++ {
		binary.LittleEndian.PutUint16(iqPayload[i*4:], uint16(300+i))
		binary.LittleEndian.PutUint16(iqPayload[i*4+2:], uint16(400+i))
	}

	ops := []plutoMockOp{
		{cmd: "LIST_DEVICES", status: len("ad9361-phy cf-ad9361-lpc cf-ad9361-dds"), payload: "ad9361-phy cf-ad9361-lpc cf-ad9361-dds"},
		{cmd: "WRITE_ATTR ad9361-phy sampling_frequency 4000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage1 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage0 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 hardwaregain 5", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 hardwaregain 5", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy out hardwaregain 0", status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-lpc", status: len("voltage0 voltage1"), payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-lpc", numSamples), status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-dds", status: len("voltage0 voltage1"), payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-dds", numSamples), status: 0, payload: ""},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: 1, payload: "rx stall"},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: len(iqPayload), binaryPayload: iqPayload},
		{cmd: fmt.Sprintf("CLOSE %s", "cf-ad9361-lpc"), status: 0, payload: ""},
		{cmd: fmt.Sprintf("CLOSE %s", "cf-ad9361-dds"), status: 0, payload: ""},
	}

	addr, errCh := startPlutoMockServer(t, ops)

	p := NewPluto()
	cfg := Config{
		URI:        addr,
		SampleRate: 4_000_000,
		RxLO:       2.3e9,
		RxGain0:    5,
		RxGain1:    5,
		TxGain:     0,
		NumSamples: numSamples,
	}

	if err := p.Init(context.Background(), cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	defer p.Close()

	if _, _, err := p.RX(context.Background()); err == nil {
		t.Fatal("expected RX error on stalled buffer")
	}

	if _, _, err := p.RX(context.Background()); err != nil {
		t.Fatalf("RX recovery failed: %v", err)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}
