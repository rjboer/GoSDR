package connectionmgr

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/rjboer/GoSDR/internal/sdrxml"
)

type Mode int

const (
	ModeASCII Mode = iota
	ModeBinary
)

type Manager struct {
	Address    string
	Mode       Mode
	byteStream chan []byte
	Timeout    time.Duration
	Logger     *log.Logger
	ClientInfo ClientInfo_type
	clientID   uint16 // libiio client identifier (0 unless multiplexing is added)
	// nextBufferID increments for each newly created binary buffer.
	nextBufferID uint16

	conn net.Conn
	br   *bufio.Reader
}

type ClientInfo_type struct {
	Version    string
	XMLcontext sdrxml.SDRContext
}

var errBinaryRejected = errors.New("BINARY command rejected by server")

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
	m.br = bufio.NewReader(c)
	m.clientID = 0
	m.Mode = ModeASCII
	return nil
}

func (m *Manager) ReadByte() error {
	if m.br == nil {
		return fmt.Errorf("not connected")
	}
	_, err := m.br.ReadByte()
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}
	return nil
}

// Safe reinjection (tests, SSH tunnels, etc.)
func (m *Manager) SetConn(conn net.Conn) {
	m.conn = conn
	m.br = bufio.NewReader(conn)
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
	if m.conn != nil {
		_ = m.conn.SetReadDeadline(time.Now().Add(time.Second * 5))
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

// readAll reads exactly len(b) bytes from the buffered reader.
func (m *Manager) readAll(b []byte) error {
	if m.br == nil {
		return fmt.Errorf("readAll: not connected")
	}

	m.applyReadDeadline()
	_, err := io.ReadFull(m.br, b)
	return err
}

// readLine reads a single line ending in '\n'.
//
// Parameters:
//   - maxLenHint is optional and effectively ignored by bufio.Reader, but retained
//     for API compatibility. The buffer grows as needed.
//   - output controls optional debug logging.
//
// Returns the line including the trailing delimiter, or an error.
func (m *Manager) readLine(
	maxLenHint int, output bool,
) (line []byte, err error) {
	if m == nil || m.br == nil {
		return nil, fmt.Errorf("readLine: not connected")
	}

	m.applyReadDeadline()
	line, err = m.br.ReadBytes('\n')
	if err != nil {
		return line, fmt.Errorf("readLine failed: %w", err)
	}

	if output {
		log.Printf("[READ] %q", string(line))
	}

	return line, nil
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
	fmt.Println("len(buf)", len(buf))
	if err := m.readAll(buf); err != nil {
		return nil, fmt.Errorf("read xml: %w", err)
	}

	return buf[:n], nil
}

// TryUpgradeToBinary sends BINARY and switches mode on success.
func (m *Manager) TryUpgradeToBinary() (bool, error) {
	if err := m.EnterBinaryMode(); err != nil {
		// A non-nil error means an I/O or parsing problem. A nil error
		// indicates we switched to binary.
		if errors.Is(err, errBinaryRejected) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// EnterBinaryMode sends the BINARY command and marks the Manager as binary-only.
// After a successful switch, ASCII helpers must not be used.
func (m *Manager) EnterBinaryMode() error {
	fmt.Println("EnterBinaryMode Command")
	fmt.Println(`This command is obsolete,The server starts in ASCII
It implicitly switches to binary when it receives a binary command header
There is no ASCII “BINARY” state transition anymore`)
	fmt.Println("It will work for very old servers")
	if m == nil {
		return fmt.Errorf("nil Manager")
	}
	if m.conn == nil {
		return fmt.Errorf("EnterBinaryMode: not connected")
	}
	if m.Mode == ModeBinary {
		return nil
	}

	ret, err := m.ExecCommand("BINARY")
	if err != nil {
		return fmt.Errorf("BINARY command failed: %w", err)
	}

	if ret != 0 {
		return fmt.Errorf("BINARY command rejected:  %w, returncode:%d", err, ret)
	}

	m.Mode = ModeBinary
	return nil
}

// func (m *Manager) EnterBinaryMode2() error {
// 	if m.Mode == ModeBinary {
// 		return nil
// 	}

// 	// Step 1: ASCII command
// 	if _, err := m.ExecCommand("BINARY"); err != nil {
// 		return fmt.Errorf("BINARY command failed: %w", err)
// 	}

// 	// Step 2: read server binary hello
// 	var hdr iiodCommand
// 	if err := m.readAll((*[8]byte)(unsafe.Pointer(&hdr))[:]); err != nil {
// 		return fmt.Errorf("binary hello read failed: %w", err)
// 	}

// 	hdr.ClientID = binary.BigEndian.Uint16((*[2]byte)(unsafe.Pointer(&hdr))[:])
// 	fmt.Println("ClientID", hdr.ClientID)
// 	fmt.Println("Op", hdr.Op)
// 	fmt.Println("Code", hdr.Code)
// 	// fmt.Println("DataLen", hdr.DataLen)
// 	// fmt.Println("Data", hdr.Data)
// 	// if hdr.Op != opBinary {
// 	// 	return fmt.Errorf("unexpected binary hello op=%d", hdr.Op)
// 	// }
// 	if hdr.Code != 0 {
// 		return fmt.Errorf("binary hello returned error %d", hdr.Code)
// 	}

// 	// Step 3: commit mode switch
// 	m.Mode = ModeBinary
// 	return nil
// }

func (m *Manager) EnterBinaryMode3() error {

	m.Mode = ModeBinary
	return nil
}
