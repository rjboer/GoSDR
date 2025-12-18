package connectionmgr

import (
	"fmt"
	"io"
	"log"
	"net"
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

	clientID uint16 // libiio client identifier (0 unless multiplexing is added)

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
	m.clientID = 0
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

// SetClientID overrides the libiio client identifier used in binary headers.
func (m *Manager) SetClientID(id uint16) {
	if m == nil {
		return
	}
	m.clientID = id
	m.Mode = ModeBinary
}

// ---------- Raw I/O (NO BUFFERING) ----------

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

// readLine reads a single LF-terminated line (ASCII) up to maxLen bytes.
// It reads from the raw socket byte-by-byte to avoid buffering issues and
// returns the raw bytes, including the trailing '\n'.
func (m *Manager) readLine(maxLen int) ([]byte, error) {
	if m.conn == nil {
		return nil, fmt.Errorf("readLine: not connected")
	}
	if maxLen <= 0 {
		return nil, fmt.Errorf("readLine: invalid maxLen %d", maxLen)
	}

	buf := make([]byte, 0, maxLen)
	var one [1]byte

	for len(buf) < maxLen {
		m.applyReadDeadline()
		_, err := m.conn.Read(one[:])
		if err != nil {
			return nil, err
		}
		b := one[0]
		buf = append(buf, b)
		if b == '\n' {
			return buf, nil
		}
	}

	return buf, fmt.Errorf("readLine: exceeded maxLen %d without newline", maxLen)
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
