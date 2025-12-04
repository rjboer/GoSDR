package iiod

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestCreateStreamBuffer(t *testing.T) {
	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LIST_CHANNELS cf-ad9361-lpc", status: 0, payload: "voltage0 voltage1 voltage2 voltage3"},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage1 en 1", status: 0, payload: ""},
		{cmd: "OPEN cf-ad9361-lpc 1024", status: 0, payload: ""},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	// Enable first 2 channels (voltage0 and voltage1)
	buf, err := client.CreateStreamBuffer("cf-ad9361-lpc", 1024, 0x03)
	if err != nil {
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
	// Prepare test data: 4 int16 samples (8 bytes)
	testData := make([]byte, 8)
	binary.LittleEndian.PutUint16(testData[0:2], 100) // I0
	binary.LittleEndian.PutUint16(testData[2:4], 200) // Q0
	binary.LittleEndian.PutUint16(testData[4:6], 300) // I1
	binary.LittleEndian.PutUint16(testData[6:8], 400) // Q1

	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LIST_CHANNELS test-dev", status: 0, payload: "ch0"},
		{cmd: "WRITE_ATTR test-dev ch0 en 1", status: 0, payload: ""},
		{cmd: "OPEN test-dev 4", status: 0, payload: ""},
		{cmd: "READBUF test-dev 4", status: 0, binaryPayload: testData},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	buf, err := client.CreateStreamBuffer("test-dev", 4, 0x01)
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
	testData := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	addr, serverErr := startBufferMockServer(t, []mockBufferOp{
		{cmd: "LIST_CHANNELS test-dev", status: 0, payload: "ch0"},
		{cmd: "WRITE_ATTR test-dev ch0 en 1", status: 0, payload: ""},
		{cmd: "OPEN test-dev 4", status: 0, payload: ""},
		{cmd: "WRITEBUF test-dev 8", status: 0, payload: "", expectBinary: testData},
	})

	client, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer client.Close()

	buf, err := client.CreateStreamBuffer("test-dev", 4, 0x01)
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
			// Read command
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}

			receivedCmd := strings.TrimSpace(line)
			if receivedCmd != op.cmd {
				errCh <- fmt.Errorf("unexpected command: got %q, want %q", receivedCmd, op.cmd)
				return
			}

			// Handle WRITEBUF specially - need to read binary data
			if strings.HasPrefix(op.cmd, "WRITEBUF") {
				if op.expectBinary != nil {
					data := make([]byte, len(op.expectBinary))
					if _, err := reader.Read(data); err != nil {
						errCh <- fmt.Errorf("failed to read binary data: %v", err)
						return
					}

					for i, b := range op.expectBinary {
						if data[i] != b {
							errCh <- fmt.Errorf("binary data mismatch at byte %d: got %d, want %d", i, data[i], b)
							return
						}
					}
				}
			}

			// Send response
			if op.binaryPayload != nil {
				// Binary response
				header := fmt.Sprintf("%d %d\n", op.status, len(op.binaryPayload))
				if _, err := fmt.Fprint(conn, header); err != nil {
					errCh <- err
					return
				}
				if _, err := conn.Write(op.binaryPayload); err != nil {
					errCh <- err
					return
				}
			} else {
				// Text response
				header := fmt.Sprintf("%d %d\n", op.status, len(op.payload))
				if _, err := fmt.Fprint(conn, header); err != nil {
					errCh <- err
					return
				}
				if op.payload != "" {
					if _, err := fmt.Fprint(conn, op.payload); err != nil {
						errCh <- err
						return
					}
				}
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
}
