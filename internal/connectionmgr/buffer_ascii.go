package connectionmgr

import (
	"fmt"
	"log"
)

// OpenBufferASCII opens an IIO buffer using the legacy ASCII protocol.
//
// Equivalent to iiod_client_open_with_mask() in libiio.
//
// maskHex must be a hex string representing the channel mask,
// exactly as IIOD expects (e.g. "ffffffff" or "00000003").
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

// ReadBufferASCII reads raw bytes from an open buffer.
//
// Protocol (legacy ASCII):
//
//	READBUF <dev> <len>\r\n
//	-> integer N (chunk bytes)
//	   if N > 0: then N bytes of binary payload follow immediately
//	   if N == 0: end
//	   if N < 0: error (negative errno)
//
// IMPORTANT:
// If the server announces N bytes but the caller's dst does not have enough remaining
// capacity, we MUST still read and discard the remainder to keep the TCP stream aligned.
// Otherwise the next readInteger() will start in the middle of binary payload and parse
// garbage (your “announced bytes=-4096” symptom).
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

// CloseBufferASCII closes an open ASCII buffer.
//
// Equivalent to iiod_client_close_unlocked().
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
