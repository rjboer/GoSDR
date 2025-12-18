package connectionmgr

import (
	"fmt"
	"strconv"
)

// readInteger reads a single ASCII integer terminated by '\n'.
// It reads byte-by-byte directly from the socket to avoid any read-ahead.
// This matches libiio's iiod_client_read_integer() semantics exactly.
func (m *Manager) readInteger() (int, error) {
	if m.conn == nil {
		return 0, fmt.Errorf("readInteger: not connected")
	}

	var buf []byte
	var one [1]byte
	started := false

	for {
		m.applyReadDeadline()
		_, err := m.conn.Read(one[:])
		if err != nil {
			return 0, err
		}

		b := one[0]

		// Newline ends the integer ONLY if we started collecting
		if b == '\n' {
			if started {
				break
			}
			// Otherwise ignore stray newline
			continue
		}

		// Ignore CR
		if b == '\r' {
			continue
		}

		// Accept digits and optional minus
		if (b >= '0' && b <= '9') || b == '-' {
			started = true
			buf = append(buf, b)
			continue
		}

		// Otherwise: binary junk → skip
	}

	if len(buf) == 0 {
		return 0, fmt.Errorf("empty integer line")
	}

	val, err := strconv.Atoi(string(buf))
	if err != nil {
		return 0, fmt.Errorf("parse integer %q: %w", string(buf), err)
	}
	return val, nil
}

// ExecCommand sends a single ASCII command and reads the integer response.
// This is used for TIMEOUT, PRINT, OPEN, CLOSE, etc.
func (m *Manager) ExecCommand(cmd string) (int, error) {
	if m.conn == nil {
		return 0, fmt.Errorf("ExecCommand: not connected")
	}
	if m.Mode == ModeBinary {
		return 0, fmt.Errorf("ExecCommand: ASCII helpers are disabled in binary mode")
	}

	if !hasLineEnding(cmd) {
		cmd += "\r\n"
	}

	if err := m.writeAll([]byte(cmd)); err != nil {
		return 0, err
	}

	return m.readInteger()
}

// hasLineEnding checks whether the string already ends with CR or LF.
func hasLineEnding(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' || s[i] == '\r' {
			return true
		}
	}
	return false
}

// writeLine writes a command line terminated with CRLF.
func (m *Manager) writeLine(cmd string) error {
	if !hasLineEnding(cmd) {
		cmd += "\r\n"
	}
	return m.writeAll([]byte(cmd))
}

// ExecASCII is an alias for ExecCommand (legacy naming used by other helpers).
func (m *Manager) ExecASCII(cmd string) (int, error) {
	return m.ExecCommand(cmd)
}
