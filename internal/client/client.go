package iiod

import (
	"bufio"
	"io"
	"net"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	// Connection and protocol
	Transport    Transport // interface: TextTransport, BinaryTransport
	ProtoVersion int
	Reader       *bufio.Reader
	Writer       io.Writer
	Conn         net.Conn

	// Parsed XML context
	XmlContext     *IIODcontext
	DeviceIndexMap map[string]uint16
	AttributeCodes map[string]uint16

	// Feature flags / capabilities
	SupportsBinary bool
	SupportsText   bool
	SupportsLegacy bool

	// Debug + logging
	DebugMode  bool
	DebugLevel int
	Logf       func(format string, args ...any)

	// Metrics
	Metrics struct {
		RXBuffers    atomic.Int64
		TXBuffers    atomic.Int64
		BytesRead    atomic.Int64
		BytesWritten atomic.Int64
	}

	// Device + attribute mappings
	deviceIndexMap map[string]uint16
	attributeCodes map[string]uint16

	// Optional SSH fallback
	SshEnabled  bool
	SshClient   *ssh.Client
	SshRootPath string
}

type Transport interface {
	Send(cmd []byte) error
	Recv() ([]byte, error)
	RecvN(n int) ([]byte, error)
	Close() error
}
