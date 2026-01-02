package connectionmgr

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// readInteger reads a single ASCII integer terminated by '\n'. The call
// leverages readLine's fixed-length strategy to avoid per-byte socket reads
// while preserving protocol semantics.
//
// Uses regex to extract the integer from the response, handling cases where
// the server may include extra text or formatting.
func (m *Manager) readInteger() (int, error) {
	line, err := m.readLine(64, false)
	if err != nil {
		return 0, err
	}

	// Use regex to extract integer from the line
	// Pattern: optional whitespace, optional minus sign, one or more digits
	re := regexp.MustCompile(`^\s*(-?\d+)`)
	matches := re.FindStringSubmatch(string(line))

	if len(matches) < 2 {
		trimmed := strings.TrimSpace(string(line))
		trimmed = strings.Trim(trimmed, "\x00")
		return 0, fmt.Errorf("no integer found in response %q", trimmed)
	}

	val, convErr := strconv.Atoi(matches[1])
	if convErr != nil {
		return 0, fmt.Errorf("parse integer %q: %w", matches[1], convErr)
	}
	return val, nil
}

// ExecCommand writes an ASCII command line (adding CRLF if missing), then
// reads the server's integer status line.
//
// Parameters:
//   - cmd: the ASCII command to send (for example TIMEOUT, PRINT, OPEN, CLOSE).
//
// Behavior:
//   - refuses to run when the manager is disconnected or already in binary
//     mode, because ASCII bootstrap commands are not valid there.
//   - sends the command verbatim (ensuring a trailing CRLF) and returns the
//     integer parsed from the next line of the stream.
//
// Returns:
//   - the integer status/value reported by the device (0 for success or
//     negative errno codes) or an error if the socket write/read fails.
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

// ExecASCII forwards to ExecCommand; it exists for callers that still use the
// legacy ASCII helper name.
func (m *Manager) ExecASCII(cmd string) (int, error) {
	return m.ExecCommand(cmd)
}
