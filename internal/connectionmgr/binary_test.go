package connectionmgr

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestTransferBinaryRXBlockStatusOnly(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		// TRANSFER_BLOCK request
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read header: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = 1
		binary.BigEndian.PutUint32(resp[4:8], 0)
		_, _ = server.Write(resp[:])
	}()

	res, err := m.TransferBinaryRXBlock(1, 4, nil)
	if err != nil {
		t.Fatalf("TransferBinaryRXBlock: %v", err)
	}
	if !res.StatusOnly || res.StatusCode != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}

	<-done
}

func TestTransferBinaryRXBlockNegativeStatus(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	m := &Manager{Timeout: time.Second, Mode: ModeBinary}
	m.SetConn(client)

	done := make(chan struct{})
	go func() {
		defer close(done)

		var hdr [8]byte
		if _, err := io.ReadFull(server, hdr[:]); err != nil {
			t.Errorf("read header: %v", err)
			return
		}

		var resp [8]byte
		binary.BigEndian.PutUint16(resp[0:2], 0)
		resp[2] = opResponse
		resp[3] = 2
		code := int32(-5)
		binary.BigEndian.PutUint32(resp[4:8], uint32(code))
		_, _ = server.Write(resp[:])
	}()

	_, err := m.TransferBinaryRXBlock(2, 4, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	if _, ok := err.(ErrIiodStatus); !ok {
		t.Fatalf("expected ErrIiodStatus, got %T", err)
	}

	<-done
}
