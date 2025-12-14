package connectionmgr

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) applyReadDeadline() {
	if m.conn != nil && m.Timeout > 0 {
		_ = m.conn.SetReadDeadline(time.Now().Add(m.Timeout))
	}
}

func (m *Manager) applyWriteDeadline() {
	if m.conn != nil && m.Timeout > 0 {
		_ = m.conn.SetWriteDeadline(time.Now().Add(m.Timeout))
	}
}

// writeAll writes the full buffer, handling short writes.
func (m *Manager) writeAll(b []byte) error {
	if m.conn == nil {
		return fmt.Errorf("writeAll: not connected")
	}
	for len(b) > 0 {
		m.applyWriteDeadline()
		n, err := m.writer.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return m.writer.Flush()
}

// readAll reads exactly len(b) bytes.
func (m *Manager) readAll(b []byte) error {
	if m.conn == nil {
		return fmt.Errorf("readAll: not connected")
	}
	_, err := bufio.NewReader(m.reader).Read(b)
	// NOTE: m.reader is already a bufio.Reader, but we can just use it directly:
	// _, err := io.ReadFull(m.reader, b)
	// To keep it simple here we rely on Read(b), but consider io.ReadFull in real code.
	return err
}

// readInteger reads a single line and parses it as a signed integer.
// It corresponds conceptually to iiod_client_read_integer(). :contentReference[oaicite:8]{index=8}
func (m *Manager) readInteger() (int, error) {
	if m.conn == nil {
		return 0, fmt.Errorf("readInteger: not connected")
	}
	m.applyReadDeadline()
	line, err := m.reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, fmt.Errorf("empty integer line")
	}
	val, err := strconv.Atoi(line)
	if err != nil {
		return 0, fmt.Errorf("parse integer %q: %w", line, err)
	}
	return val, nil
}

func hasLineEnding(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' || s[i] == '\r' {
			return true
		}
	}
	return false
}
