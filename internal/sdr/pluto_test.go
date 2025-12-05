package sdr

import (
	"bufio"
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
			line, err := reader.ReadString('\n')
			if err != nil {
				errCh <- fmt.Errorf("read command: %w", err)
				return
			}
			got := strings.TrimSpace(line)
			if got != op.cmd {
				errCh <- fmt.Errorf("unexpected command %q, want %q", got, op.cmd)
				return
			}

			if len(op.expectBinary) > 0 {
				var lengthPrefix [4]byte
				if _, err := io.ReadFull(reader, lengthPrefix[:]); err != nil {
					errCh <- fmt.Errorf("read length prefix: %w", err)
					return
				}
				length := binary.BigEndian.Uint32(lengthPrefix[:])
				data := make([]byte, length)
				if _, err := io.ReadFull(reader, data); err != nil {
					errCh <- fmt.Errorf("read binary payload: %w", err)
					return
				}
				if string(data) != string(op.expectBinary) {
					errCh <- fmt.Errorf("binary payload mismatch: got %v want %v", data, op.expectBinary)
					return
				}
			}

			payload := []byte(op.payload)
			if len(op.binaryPayload) > 0 {
				payload = op.binaryPayload
			}

			if _, err := fmt.Fprintf(conn, "%d %d\n", op.status, len(payload)); err != nil {
				errCh <- fmt.Errorf("write response header: %w", err)
				return
			}
			if len(payload) > 0 {
				if _, err := conn.Write(payload); err != nil {
					errCh <- fmt.Errorf("write response payload: %w", err)
					return
				}
			}
		}

		errCh <- nil
	}()

	return listener.Addr().String(), errCh
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
		{cmd: "LIST_DEVICES", status: 0, payload: "ad9361-phy cf-ad9361-lpc cf-ad9361-dds"},
		{cmd: "WRITE_ATTR ad9361-phy sampling_frequency 2000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage1 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage0 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 hardwaregain 10", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 hardwaregain 11", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy out hardwaregain 0", status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-lpc", status: 0, payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-lpc", numSamples), status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-dds", status: 0, payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-dds", numSamples), status: 0, payload: ""},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: 0, binaryPayload: iqPayload},
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
		{cmd: "LIST_DEVICES", status: 0, payload: "ad9361-phy cf-ad9361-lpc cf-ad9361-dds"},
		{cmd: "WRITE_ATTR ad9361-phy sampling_frequency 4000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage1 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy altvoltage0 frequency 2300000000", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 gain_control_mode manual", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage0 hardwaregain 5", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy voltage1 hardwaregain 5", status: 0, payload: ""},
		{cmd: "WRITE_ATTR ad9361-phy out hardwaregain 0", status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-lpc", status: 0, payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-lpc voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-lpc", numSamples), status: 0, payload: ""},
		{cmd: "LIST_CHANNELS cf-ad9361-dds", status: 0, payload: "voltage0 voltage1"},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage0 en 1", status: 0, payload: ""},
		{cmd: "WRITE_ATTR cf-ad9361-dds voltage1 en 1", status: 0, payload: ""},
		{cmd: fmt.Sprintf("OPEN %s %d", "cf-ad9361-dds", numSamples), status: 0, payload: ""},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: 1, payload: "rx stall"},
		{cmd: fmt.Sprintf("READBUF %s %d", "cf-ad9361-lpc", numSamples), status: 0, binaryPayload: iqPayload},
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
