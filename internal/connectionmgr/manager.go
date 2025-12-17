package connectionmgr

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

type Mode int

const (
	ModeASCII Mode = iota
	ModeBinary
)

type Manager struct {
	Address string
	Mode    Mode

	Timeout time.Duration
	Logger  *log.Logger

	conn net.Conn
}

// ---------- Construction / lifecycle ----------

func New(addr string) *Manager {
	return &Manager{
		Address: addr,
		Mode:    ModeASCII,
		Timeout: 5 * time.Second,
	}
}

func (m *Manager) Connect() error {
	c, err := net.DialTimeout("tcp", m.Address, m.Timeout)
	if err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	m.conn = c
	m.Mode = ModeASCII
	return nil
}

// Safe reinjection (tests, SSH tunnels, etc.)
func (m *Manager) SetConn(conn net.Conn) {
	m.conn = conn
}

func (m *Manager) Close() error {
	if m.conn != nil {
		return m.conn.Close()
	}
	return nil
}

func (m *Manager) SetTimeout(d time.Duration) {
	m.Timeout = d
	if m.conn != nil && d > 0 {
		_ = m.conn.SetDeadline(time.Now().Add(d))
	}
}

// ---------- Logging ----------

func (m *Manager) logf(format string, args ...any) {
	if m == nil {
		return
	}
	l := m.Logger
	if l == nil {
		l = log.Default()
	}
	l.Printf(format, args...)
}

func (m *Manager) SetLogger(l *log.Logger) {
	m.Logger = l
}

// ---------- Raw I/O (NO BUFFERING) ----------

func (m *Manager) readAll(p []byte) error {
	_, err := io.ReadFull(m.conn, p)
	return err
}

func (m *Manager) writeAll(p []byte) error {
	_, err := m.conn.Write(p)
	return err
}

// Read ASCII line, byte-by-byte, with hard limit
func (m *Manager) readLine(limit int) ([]byte, error) {
	if limit <= 0 {
		limit = 64 * 1024
	}

	var buf bytes.Buffer
	var b [1]byte

	for buf.Len() < limit {
		if err := m.readAll(b[:]); err != nil {
			return nil, err
		}
		buf.WriteByte(b[0])
		if b[0] == '\n' {
			return buf.Bytes(), nil
		}
	}

	return nil, fmt.Errorf("line exceeds limit=%d", limit)
}

// ---------- ASCII protocol helpers ----------

func hasLineEnding(s string) bool {
	return strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\r\n")
}

func (m *Manager) readInteger() (int, error) {
	line, err := m.readLine(1024)
	if err != nil {
		return 0, err
	}

	s := strings.TrimSpace(string(line))
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid integer response %q", s)
	}
	return n, nil
}

// ExecCommand sends an ASCII command and reads a single integer response.
//
// Mirrors iiod_client_exec_command():
//
//	write command
//	read integer line
func (m *Manager) ExecCommand(cmd string) (int, error) {
	if m.Mode != ModeASCII {
		return 0, fmt.Errorf("ExecCommand: not in ASCII mode")
	}
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

// ---------- Higher-level operations ----------

// FetchXML sends PRINT and returns the XML payload.
func (m *Manager) FetchXML() ([]byte, error) {
	n, err := m.ExecCommand("PRINT")
	if err != nil {
		return nil, fmt.Errorf("PRINT failed: %w", err)
	}
	if n <= 0 {
		return nil, fmt.Errorf("PRINT returned non-positive length %d", n)
	}

	buf := make([]byte, n+1) // +1 for trailing '\n'
	if err := m.readAll(buf); err != nil {
		return nil, fmt.Errorf("read xml: %w", err)
	}

	return buf[:n], nil
}

// TryUpgradeToBinary sends BINARY and switches mode on success.
func (m *Manager) TryUpgradeToBinary() (bool, error) {
	ret, err := m.ExecCommand("BINARY")
	if err != nil {
		return false, fmt.Errorf("BINARY command failed: %w", err)
	}

	m.logf("[conman] BINARY returned code=%d", ret)

	if ret != 0 {
		return false, nil
	}

	m.Mode = ModeBinary
	return true, nil
}
