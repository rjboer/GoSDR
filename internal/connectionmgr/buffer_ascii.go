package connectionmgr

import (
	"errors"
	"fmt"
	"log"
	"strings"
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

	mask := strings.TrimPrefix(strings.TrimPrefix(maskHex, "0x"), "0X")
	if mask == "" {
		return fmt.Errorf("OpenBufferASCII: maskHex is required")
	}

	cmd := fmt.Sprintf("OPEN %s %d 0x%s", deviceID, samples, mask)
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
// The method consumes the announced mask line, reads exactly N bytes into dst
// (erroring when dst is too small), and then drains the trailing newline to keep
// the stream aligned. It returns the number of bytes copied into dst or an
// error when the mode is incorrect, IO fails, or the server returns a negative
// errno.
func (m *Manager) ReadBufferASCII(deviceID string, dst []byte) (int, error) {
	n, _, err := m.ReadBufferASCIIWithMask(deviceID, dst)
	return n, err
}

// ReadBufferASCIIWithMask reads raw bytes from an open buffer using the READBUF
// command and returns the channel mask string announced by the server.
func (m *Manager) ReadBufferASCIIWithMask(deviceID string, dst []byte) (int, string, error) {
	if m.Mode != ModeASCII {
		return 0, "", fmt.Errorf("ReadBufferASCII: not in ASCII mode")
	}

	cmd := fmt.Sprintf("READBUF %s %d", deviceID, len(dst))
	log.Printf("[READBUF] -> %q", cmd)

	if err := m.writeAll([]byte(cmd + "\r\n")); err != nil {
		return 0, "", err
	}

	log.Printf("[READBUF] waiting for size integer")
	n, err := m.readInteger()
	if err != nil {
		return 0, "", err
	}

	log.Printf("[READBUF] announced bytes=%d", n)

	if n < 0 {
		return 0, "", fmt.Errorf("READBUF error: %d", n)
	}
	if n == 0 {
		return 0, "", nil
	}

	if n > len(dst) {
		return 0, "", fmt.Errorf("READBUF announced %d bytes but destination capacity is %d", n, len(dst))
	}

	maskLine, err := m.readLine(64, true)
	if err != nil {
		return 0, "", fmt.Errorf("READBUF: failed to consume mask line: %w", err)
	}
	log.Printf("[READBUF] raw mask line=%q", maskLine)
	mask := strings.TrimSpace(string(maskLine))

	// Read payload
	if err := m.readAll(dst[:n]); err != nil {
		return 0, "", err
	}

	// Consume the trailing newline to keep the socket aligned for the next
	// command.
	var newline [1]byte
	if err := m.readAll(newline[:]); err != nil {
		return 0, "", err
	}
	log.Printf("[READBUF] trailing delimiter byte=%q", newline[0])
	if newline[0] != '\n' {
		return 0, "", fmt.Errorf("READBUF: expected trailing newline, got %q", newline[0])
	}

	log.Printf("[READBUF] completed: total=%d bytes mask=%s", n, mask)
	return n, mask, nil
}

// WriteBufferASCII writes raw bytes to an open buffer using the WRITEBUF command.
//
// Parameters:
//   - deviceID: IIO device identifier.
//   - payload: raw bytes to stream to the device.
//
// Protocol (legacy ASCII):
//
//	WRITEBUF <dev> <bytes>\r\n
//	<- integer N (bytes accepted)
//
// The method streams the payload via writeAll, then parses the returned integer
// status. Negative statuses are surfaced as errors. If the server reports a
// positive byte count that does not match the payload length, the partial write
// count is returned alongside an error to keep the stream aligned for the next
// command.
func (m *Manager) WriteBufferASCII(deviceID string, payload []byte) (int, error) {
	if m == nil || m.conn == nil {
		return 0, errors.New("not connected")
	}
	if m.Mode != ModeASCII {
		return 0, fmt.Errorf("WriteBufferASCII: not in ASCII mode")
	}
	if deviceID == "" {
		return 0, errors.New("deviceID is required")
	}

	cmd := fmt.Sprintf("WRITEBUF %s %d", deviceID, len(payload))
	log.Printf("[WRITEBUF] -> %q", cmd)

	if err := m.writeLine(cmd); err != nil {
		return 0, err
	}
	if err := m.writeAll(payload); err != nil {
		return 0, err
	}

	written, err := m.readInteger()
	if err != nil {
		return 0, err
	}
	if written < 0 {
		return written, fmt.Errorf("WRITEBUF returned %d", written)
	}
	if written != len(payload) {
		return written, fmt.Errorf("WRITEBUF wrote %d of %d bytes", written, len(payload))
	}

	return written, nil
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
