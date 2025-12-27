package connectionmgr

import (
	"fmt"
	"log"
)

// SetKernelBuffersCountASCII configures the number of kernel buffers for a
// device using the ASCII SET command.
//
// Protocol:
//   - issues "SET <deviceID> BUFFERS_COUNT <count>\r\n" via ExecASCII.
//   - non-negative return codes indicate success; negative errno values are
//     surfaced as errors.
//
// Parameters:
//   - deviceID: IIO device identifier (for example "cf-ad9361-lpc").
//   - count: desired number of kernel buffers; must be zero or positive.
//
// Returns nil on success or an error for validation failures, transport
// errors, or negative device responses.
func (m *Manager) SetKernelBuffersCountASCII(deviceID string, count int) error {
	if deviceID == "" {
		return fmt.Errorf("deviceID is required")
	}
	if count < 0 {
		return fmt.Errorf("count must be >= 0")
	}

	status, err := m.ExecASCII(fmt.Sprintf("SET %s BUFFERS_COUNT %d", deviceID, count))
	if err != nil {
		return fmt.Errorf("SET BUFFERS_COUNT command failed: %w", err)
	}
	if status < 0 {
		return fmt.Errorf("SET BUFFERS_COUNT returned %d", status)
	}

	return nil
}

// OpenBufferASCII sends the ASCII OPEN command to allocate a buffer.
//
// Parameters:
//   - deviceID: IIO device identifier (for example "cf-ad9361-lpc").
//   - samples: number of samples per buffer.
//   - maskHex: channel mask as a hex string (e.g. "ffffffff" or "00000003").
//   - cyclic: whether to request a cyclic buffer.
//
// Protocol:
//   - issues "OPEN <deviceID> <samples> <maskHex>[ CYCLIC]\r\n" and expects an
//     integer response.
//
// Returns nil on success or an error when not in ASCII mode, the command fails
// to send, or the response is a negative errno code.
func (m *Manager) OpenBufferASCII(
	deviceID string,
	samples uint64,
	maskHex string,
	cyclic bool,
) error {
	if m.Mode != ModeASCII {
		return fmt.Errorf("OpenBufferASCII: not in ASCII mode")
	}

	cmd := fmt.Sprintf("OPEN %s %d %s", deviceID, samples, maskHex)
	if cyclic {
		cmd += " CYCLIC"
	}

	ret, err := m.ExecCommand(cmd)
	if err != nil {
		return err
	}
	if ret < 0 {
		return fmt.Errorf("OPEN failed: %d", ret)
	}
	return nil
}

// ReadBufferASCII reads raw bytes from an open buffer using the READBUF command.
//
// Parameters:
//   - deviceID: IIO device identifier.
//   - dst: caller-provided byte slice to fill.
//
// Protocol (legacy ASCII):
//
//	READBUF <dev> <len>\r\n
//	-> integer N (chunk bytes)
//	   if N > 0: then N bytes of binary payload follow immediately
//	   if N == 0: end
//	   if N < 0: error (negative errno)
//
// The method always drains the full N bytes announced by the server, copying up
// to len(dst) into dst and discarding overflow to preserve stream alignment.
// It returns the number of bytes copied into dst or an error when the mode is
// incorrect, IO fails, or the server returns a negative errno.
func (m *Manager) ReadBufferASCII(deviceID string, dst []byte) (int, error) {
	if m.Mode != ModeASCII {
		return 0, fmt.Errorf("ReadBufferASCII: not in ASCII mode")
	}

	cmd := fmt.Sprintf("READBUF %s %d", deviceID, len(dst))
	log.Printf("[READBUF] -> %q", cmd)

	if err := m.writeAll([]byte(cmd + "\r\n")); err != nil {
		return 0, err
	}

	log.Printf("[READBUF] waiting for size integer")
	n, err := m.readInteger()
	if err != nil {
		return 0, err
	}

	log.Printf("[READBUF] announced bytes=%d", n)

	if n < 0 {
		return 0, fmt.Errorf("READBUF error: %d", n)
	}
	if n == 0 {
		return 0, nil
	}

	toCopy := n
	if toCopy > len(dst) {
		toCopy = len(dst)
	}

	// Read payload
	if err := m.readAll(dst[:toCopy]); err != nil {
		return toCopy, err
	}

	// Discard overflow payload
	remaining := n - toCopy
	if remaining > 0 {
		if err := m.drainBytes(remaining); err != nil {
			return toCopy, err
		}
		log.Printf("[READBUF] truncated payload: copied=%d drained=%d", toCopy, remaining)
	}

	log.Printf("[READBUF] completed: total=%d bytes", toCopy)
	return toCopy, nil
}

// drainBytes reads and discards exactly n bytes from the socket.
// Keeps the protocol stream aligned when the server sends more than we can store.
func (m *Manager) drainBytes(n int) error {
	const chunk = 16 * 1024
	var scratch [chunk]byte

	left := n
	for left > 0 {
		want := left
		if want > chunk {
			want = chunk
		}
		if err := m.readAll(scratch[:want]); err != nil {
			return err
		}
		left -= want
	}
	return nil
}

// CloseBufferASCII issues the ASCII CLOSE command to release a buffer.
// It returns nil on success or an error when the manager is not in ASCII mode,
// the command fails, or the server replies with a negative errno.
func (m *Manager) CloseBufferASCII(deviceID string) error {
	if m.Mode != ModeASCII {
		return fmt.Errorf("CloseBufferASCII: not in ASCII mode")
	}

	ret, err := m.ExecCommand(fmt.Sprintf("CLOSE %s", deviceID))
	if err != nil {
		return err
	}
	if ret < 0 {
		return fmt.Errorf("CLOSE failed: %d", ret)
	}
	return nil
}
