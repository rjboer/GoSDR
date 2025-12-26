package connectionmgr

import (
	"fmt"
	"strconv"
	"strings"
)

// readInteger reads a single ASCII integer terminated by '\n'. The call
// leverages readLine's fixed-length strategy to avoid per-byte socket reads
// while preserving protocol semantics.
func (m *Manager) readInteger() (int, error) {
	line, err := m.readLine(64, false)
	if err != nil {
		return 0, err
	}

	trimmed := strings.TrimSpace(string(line))
	trimmed = strings.Trim(trimmed, "\x00")
	if trimmed == "" {
		return 0, fmt.Errorf("empty integer line")
	}

	val, convErr := strconv.Atoi(trimmed)
	if convErr != nil {
		return 0, fmt.Errorf("parse integer %q: %w", trimmed, convErr)
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
