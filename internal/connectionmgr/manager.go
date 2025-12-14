package connectionmgr

import (
	"bufio"
	"fmt"
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

	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
}

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
	m.reader = bufio.NewReader(c)
	m.writer = bufio.NewWriter(c)
	m.Mode = ModeASCII
	return nil
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

// ExecCommand sends an ASCII command and returns the integer response.
//
// This mirrors iiod_client_exec_command() semantics: write full command,
// then read a single integer line via readInteger() in iiod-client.c. :contentReference[oaicite:5]{index=5}
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

// FetchXML sends PRINT (or ZPRINT if you want later) and returns the XML blob.
func (m *Manager) FetchXML() ([]byte, error) {
	// PRINT -> integer = xml_len, then xml_len bytes (+ trailing \n)
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

	// Drop the last newline; payload itself does not include it.
	return buf[:n], nil
}

// TryUpgradeToBinary tries the BINARY command.
//
// Returns:
//
//	ok == true  => server accepted binary (return code 0) and we're in Binary mode
//	ok == false => server rejected binary (non-zero code); stay ASCII; no error
//	err != nil  => I/O / parse error
//
// This mirrors iiod_client_enable_binary() behaviour. :contentReference[oaicite:6]{index=6}
func (m *Manager) TryUpgradeToBinary() (ok bool, err error) {
	ret, err := m.ExecCommand("BINARY")
	if err != nil {
		return false, fmt.Errorf("BINARY command failed: %w", err)
	}
	if ret != 0 {
		// Not supported or refused; stay ASCII and don’t treat as fatal.
		return false, nil
	}
	m.Mode = ModeBinary
	return true, nil
}
