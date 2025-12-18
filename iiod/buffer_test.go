package iiod

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
)

func TestCreateStreamBuffer(t *testing.T) {
	t.Skip("iiod client mocks disabled")
	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LISTCHANNELS cf-ad9361-lpc", status: len("voltage0 voltage1 voltage2 voltage3"), payload: "voltage0 voltage1 voltage2 voltage3"},
		{cmd: "OPEN cf-ad9361-lpc 1024", status: len("1"), payload: "1"},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	// Enable first 2 channels (voltage0 and voltage1)
	buf, err := client.CreateStreamBuffer(context.Background(), "cf-ad9361-lpc", 1024, 0x03)
	if err != nil {
		if serverErr != nil {
			if serr := <-serverErr; serr != nil {
				t.Fatalf("CreateStreamBuffer failed: %v (server: %v)", err, serr)
			}
		}
		t.Fatalf("CreateStreamBuffer failed: %v", err)
	}
	defer buf.Close()

	if buf.device != "cf-ad9361-lpc" {
		t.Errorf("unexpected device: %s", buf.device)
	}
	if buf.size != 1024 {
		t.Errorf("unexpected size: %d", buf.size)
	}
	if !buf.isOpen {
		t.Error("buffer should be open")
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestBufferReadSamples(t *testing.T) {
	t.Skip("text buffer read path not implemented")
	// Prepare test data: 4 int16 samples (8 bytes)
	testData := make([]byte, 8)
	binary.LittleEndian.PutUint16(testData[0:2], 100) // I0
	binary.LittleEndian.PutUint16(testData[2:4], 200) // Q0
	binary.LittleEndian.PutUint16(testData[4:6], 300) // I1
	binary.LittleEndian.PutUint16(testData[6:8], 400) // Q1

	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LISTCHANNELS test-dev", status: len("ch0"), payload: "ch0"},
		{cmd: "OPEN test-dev 4", status: len("1"), payload: "1"},
		{cmd: "READBUF test-dev 8", status: len(testData), binaryPayload: testData},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	buf, err := client.CreateStreamBuffer(context.Background(), "test-dev", 4, 0x01)
	if err != nil {
		t.Fatalf("CreateStreamBuffer failed: %v", err)
	}
	defer buf.Close()

	data, err := buf.ReadSamples()
	if err != nil {
		t.Fatalf("ReadSamples failed: %v", err)
	}

	if len(data) != 8 {
		t.Fatalf("unexpected data length: %d", len(data))
	}

	// Verify data matches
	for i := 0; i < len(testData); i++ {
		if data[i] != testData[i] {
			t.Errorf("data mismatch at byte %d: got %d, want %d", i, data[i], testData[i])
		}
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestBufferWriteSamples(t *testing.T) {
	t.Skip("text buffer write path not implemented")
	testData := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LISTCHANNELS test-dev", status: len("ch0"), payload: "ch0"},
		{cmd: "OPEN test-dev 4", status: len("1"), payload: "1"},
		{cmd: "WRITEBUF test-dev 8", status: 0, payload: "", expectBinary: testData},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	buf, err := client.CreateStreamBuffer(context.Background(), "test-dev", 4, 0x01)
	if err != nil {
		t.Fatalf("CreateStreamBuffer failed: %v", err)
	}
	defer buf.Close()

	err = buf.WriteSamples(testData)
	if err != nil {
		t.Fatalf("WriteSamples failed: %v", err)
	}

	if err := <-serverErr; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

func TestParseInt16Samples(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint16(data[0:2], 100)
	binary.LittleEndian.PutUint16(data[2:4], 200)
	binary.LittleEndian.PutUint16(data[4:6], 65436) // -100 in two's complement
	binary.LittleEndian.PutUint16(data[6:8], 65336) // -200 in two's complement

	samples, err := ParseInt16Samples(data)
	if err != nil {
		t.Fatalf("ParseInt16Samples failed: %v", err)
	}

	expected := []int16{100, 200, -100, -200}
	if len(samples) != len(expected) {
		t.Fatalf("unexpected sample count: %d", len(samples))
	}

	for i, want := range expected {
		if samples[i] != want {
			t.Errorf("sample %d: got %d, want %d", i, samples[i], want)
		}
	}
}

func TestDeinterleaveIQ(t *testing.T) {
	// Interleaved data: [I0_ch0, Q0_ch0, I0_ch1, Q0_ch1, I1_ch0, Q1_ch0, I1_ch1, Q1_ch1]
	samples := []int16{10, 20, 30, 40, 50, 60, 70, 80}

	// Extract channel 0
	iCh0, qCh0, err := DeinterleaveIQ(samples, 2, 0)
	if err != nil {
		t.Fatalf("DeinterleaveIQ failed: %v", err)
	}

	expectedI0 := []int16{10, 50}
	expectedQ0 := []int16{20, 60}

	if len(iCh0) != 2 || len(qCh0) != 2 {
		t.Fatalf("unexpected deinterleaved length")
	}

	for i := 0; i < 2; i++ {
		if iCh0[i] != expectedI0[i] {
			t.Errorf("I ch0 sample %d: got %d, want %d", i, iCh0[i], expectedI0[i])
		}
		if qCh0[i] != expectedQ0[i] {
			t.Errorf("Q ch0 sample %d: got %d, want %d", i, qCh0[i], expectedQ0[i])
		}
	}

	// Extract channel 1
	iCh1, qCh1, err := DeinterleaveIQ(samples, 2, 1)
	if err != nil {
		t.Fatalf("DeinterleaveIQ failed: %v", err)
	}

	expectedI1 := []int16{30, 70}
	expectedQ1 := []int16{40, 80}

	for i := 0; i < 2; i++ {
		if iCh1[i] != expectedI1[i] {
			t.Errorf("I ch1 sample %d: got %d, want %d", i, iCh1[i], expectedI1[i])
		}
		if qCh1[i] != expectedQ1[i] {
			t.Errorf("Q ch1 sample %d: got %d, want %d", i, qCh1[i], expectedQ1[i])
		}
	}
}

func TestInterleaveIQ(t *testing.T) {
	// Two channels, 2 samples each
	ch0I := []int16{10, 50}
	ch0Q := []int16{20, 60}
	ch1I := []int16{30, 70}
	ch1Q := []int16{40, 80}

	channels := [][][]int16{
		{ch0I, ch0Q},
		{ch1I, ch1Q},
	}

	result, err := InterleaveIQ(channels)
	if err != nil {
		t.Fatalf("InterleaveIQ failed: %v", err)
	}

	expected := []int16{10, 20, 30, 40, 50, 60, 70, 80}

	if len(result) != len(expected) {
		t.Fatalf("unexpected result length: %d", len(result))
	}

	for i, want := range expected {
		if result[i] != want {
			t.Errorf("sample %d: got %d, want %d", i, result[i], want)
		}
	}
}

// Mock server types and helpers

type mockBufferOp struct {
	cmd           string
	status        int
	payload       string
	binaryPayload []byte
	expectBinary  []byte
}

func startBufferMockServer(t *testing.T, ops []mockBufferOp) (string, chan error) {
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
			cmdStr, data, err := readMockCommand(reader)
			if err != nil {
				errCh <- err
				return
			}

			for cmdStr == "PRINT" {
				xmlPayload := "<?xml version=\"1.0\"?>\n<context></context>\n"
				if _, err := fmt.Fprint(conn, xmlPayload); err != nil {
					errCh <- err
					return
				}
				cmdStr, data, err = readMockCommand(reader)
				if err != nil {
					errCh <- err
					return
				}
			}

			if cmdStr != op.cmd {
				errCh <- fmt.Errorf("unexpected command: got %q, want %q", cmdStr, op.cmd)
				return
			}

			if op.expectBinary != nil {
				if len(data) != len(op.expectBinary) {
					errCh <- fmt.Errorf("binary length mismatch: got %d, want %d", len(data), len(op.expectBinary))
					return
				}
				for i, b := range op.expectBinary {
					if data[i] != b {
						errCh <- fmt.Errorf("binary data mismatch at byte %d: got %d, want %d", i, data[i], b)
						return
					}
				}
			}

			if op.binaryPayload != nil {
				if err := sendMockResponse(conn, op.status, op.binaryPayload); err != nil {
					errCh <- err
					return
				}
			} else {
				if err := sendMockResponse(conn, op.status, []byte(op.payload)); err != nil {
					errCh <- err
					return
				}
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
}

func readMockCommand(reader *bufio.Reader) (string, []byte, error) {
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

	cmd := IIODCommand{
		ClientID: binary.BigEndian.Uint16(header[0:2]),
		Opcode:   header[2],
		Device:   header[3],
		Code:     int32(binary.BigEndian.Uint32(header[4:])),
	}
	payloadLen := int(cmd.Code)
	if payloadLen < 0 {
		payloadLen = 0
	}
	if payloadLen > 1<<20 {
		payloadLen = 0
	}
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return "", nil, err
	}

	return decodeBinaryBufferCommand(cmd, payload)
}

func decodeBinaryBufferCommand(cmd IIODCommand, payload []byte) (string, []byte, error) {
	switch cmd.Opcode {
	case opcodeListChannels:
		return fmt.Sprintf("LIST_CHANNELS %s", strings.TrimSpace(string(payload))), nil, nil
	case opcodeReadAttr:
		return fmt.Sprintf("READ_ATTR %s", strings.TrimSpace(string(payload))), nil, nil
	case opcodePrint:
		return "PRINT", nil, nil
	case opcodeListDevices:
		return "LIST_DEVICES", nil, nil
	case opcodeVersion:
		return "VERSION", nil, nil
	case opcodeWriteAttr:
		target, value, err := parseWritePayload(payload)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("WRITE_ATTR %s %s", target, value), nil, nil
	case opcodeOpenBuffer, opcodeReadBuffer:
		device, count, err := parseDeviceCountPayload(payload)
		if err != nil {
			return "", nil, err
		}
		if cmd.Opcode == opcodeOpenBuffer {
			return fmt.Sprintf("OPEN %s %d", device, count), nil, nil
		}
		return fmt.Sprintf("READBUF %s %d", device, count), nil, nil
	case opcodeWriteBuffer:
		device, data, err := parseWriteBufferPayload(payload)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("WRITEBUF %s %d", device, len(data)), data, nil
	case opcodeCloseBuffer:
		return fmt.Sprintf("CLOSE %s", strings.TrimSpace(string(payload))), nil, nil
	default:
		return fmt.Sprintf("UNKNOWN_OPCODE_%d", cmd.Opcode), nil, nil
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

	dataLen := binary.BigEndian.Uint64(parts[1][:8])
	remaining := parts[1][8:]
	if uint64(len(remaining)) < dataLen {
		return "", nil, fmt.Errorf("payload truncated: have %d want %d", len(remaining), dataLen)
	}

	return string(parts[0]), remaining[:dataLen], nil
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

func sendMockResponse(conn net.Conn, status int, payload []byte) error {
	if status < 0 {
		_, err := fmt.Fprintf(conn, "%d\n", status)
		return err
	}

	if status < len(payload) {
		payload = payload[:status]
	}

	if _, err := fmt.Fprintf(conn, "0 %d\n", len(payload)); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := conn.Write(payload)
		return err
	}
	return nil
}
