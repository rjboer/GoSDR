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

	cmd := fmt.Sprintf(
		"OPEN %s %d %s",
		deviceID,
		samples,
		maskHex,
	)
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
// This implements the exact READBUF loop used by libiio:
//
//	READBUF <dev> <len>
//	-> integer N
//	   if N > 0: read N bytes
//	   if N == 0: done
//	   if N < 0: error
//
// The function returns the total number of bytes written into dst.
func (m *Manager) ReadBufferASCII(
	deviceID string,
	dst []byte,
) (int, error) {
	if m.Mode != ModeASCII {
		return 0, fmt.Errorf("ReadBufferASCII: not in ASCII mode")
	}
	if len(dst) == 0 {
		return 0, nil
	}

	cmd := fmt.Sprintf("READBUF %s %d", deviceID, len(dst))
	log.Printf("[READBUF] -> %q", cmd)

	if err := m.writeAll([]byte(cmd + "\r\n")); err != nil {
		return 0, err
	}

	total := 0
	iteration := 0

	for total < len(dst) {
		iteration++
		log.Printf("[READBUF] iteration=%d waiting for size integer", iteration)

		n, err := m.readInteger()
		if err != nil {
			return total, err
		}

		log.Printf("[READBUF] iteration=%d announced bytes=%d", iteration, n)

		if n < 0 {
			return total, fmt.Errorf("READBUF error: %d", n)
		}

		if n == 0 {
			log.Printf("[READBUF] iteration=%d server signaled end", iteration)
			break
		}

		if total+n > len(dst) {
			n = len(dst) - total // clamp to requested size
		}

		if err := m.readAll(dst[total : total+n]); err != nil {
			return total, err
		}

		log.Printf("[READBUF] iteration=%d read %d bytes", iteration, n)
		total += n
	}

	log.Printf("[READBUF] completed: total=%d bytes", total)
	return total, nil
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
