package connectionmgr

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

// applyReadDeadline applies the configured read timeout to the socket.
func (m *Manager) applyReadDeadline() {
	if m.conn != nil && m.Timeout > 0 {
		_ = m.conn.SetReadDeadline(time.Now().Add(m.Timeout))
	}
}

// applyWriteDeadline applies the configured write timeout to the socket.
func (m *Manager) applyWriteDeadline() {
	if m.conn != nil && m.Timeout > 0 {
		_ = m.conn.SetWriteDeadline(time.Now().Add(m.Timeout))
	}
}

// writeAll writes the full buffer to the socket, handling short writes.
// Buffered writing is safe; reading is NOT buffered.
func (m *Manager) writeAll(b []byte) error {
	if m.conn == nil {
		return fmt.Errorf("writeAll: not connected")
	}

	for len(b) > 0 {
		m.applyWriteDeadline()
		n, err := m.conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}

// readAll reads exactly len(b) bytes from the socket.
// This MUST use the raw connection, not a buffered reader.
func (m *Manager) readAll(b []byte) error {
	if m.conn == nil {
		return fmt.Errorf("readAll: not connected")
	}

	m.applyReadDeadline()
	_, err := io.ReadFull(m.conn, b)
	return err
}

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

// readLine reads a single LF-terminated line (ASCII), returning it as a string.
// It reads from the raw socket byte-by-byte to avoid buffering issues.
func (m *Manager) readLine() (string, error) {
	if m.conn == nil {
		return "", fmt.Errorf("readLine: not connected")
	}

	var buf []byte
	var one [1]byte

	for {
		m.applyReadDeadline()
		_, err := m.conn.Read(one[:])
		if err != nil {
			return "", err
		}
		b := one[0]
		buf = append(buf, b)
		if b == '\n' {
			break
		}
	}
	return string(buf), nil
}

// ExecASCII is an alias for ExecCommand (legacy naming used by other helpers).
func (m *Manager) ExecASCII(cmd string) (int, error) {
	return m.ExecCommand(cmd)
}
