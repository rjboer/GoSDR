package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/rjboer/GoSDR/internal/connectionmgr"
)

// loggingConn wraps a net.Conn and dumps every byte that crosses the wire.
// It is intentionally verbose to aid diagnostics against real or mocked IIOD
// servers.
type loggingConn struct {
	net.Conn
}

func (c *loggingConn) logDirection(dir string, data []byte) {
	if len(data) == 0 {
		return
	}
	log.Printf("[wire][%s] %d bytes\n%s", dir, len(data), hex.Dump(data))
}

func (c *loggingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.logDirection("in ", p[:n])
	}
	return n, err
}

func (c *loggingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.logDirection("out", p[:n])
	}
	return n, err
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	uri := flag.String("uri", "192.168.3.1:30431", "IIOD target host:port")
	samples := flag.Uint64("samples", 4096, "Number of samples for OPEN")
	mask := flag.String("mask", "1", "Channel mask in hex (e.g. 1 or 0x3)")
	cyclic := flag.Bool("cyclic", false, "Request a cyclic buffer")
	readBytes := flag.Int("bytes", 0, "Bytes to request via READBUF (default: samples)")
	flag.Parse()

	log.Printf("[BOOT] starting ASCII diagnostic with uri=%s samples=%d mask=%s cyclic=%v bytes=%d", *uri, *samples, *mask, *cyclic, *readBytes)

	m := connectionmgr.New(*uri)
	m.SetTimeout(2 * time.Second)

	conn, err := net.DialTimeout("tcp", m.Address, m.Timeout)
	if err != nil {
		log.Fatalf("dial %s failed: %v", m.Address, err)
	}
	log.Printf("[BOOT] TCP connection established to %s", m.Address)
	m.SetConn(&loggingConn{Conn: conn})
	m.Mode = connectionmgr.ModeASCII
	m.SetTimeout(m.Timeout)
	log.Printf("[BOOT] manager configured for ASCII mode with timeout=%s", m.Timeout)

	if ret, err := m.ExecCommand(fmt.Sprintf("TIMEOUT %d", m.Timeout.Milliseconds())); err != nil {
		log.Printf("[WARN] TIMEOUT command failed (continuing with local deadline): %v", err)
	} else {
		log.Printf("[INFO] Remote TIMEOUT set, device replied with %d", ret)
	}

	log.Printf("[INFO] Fetching XML context from %s", m.Address)
	xml, err := m.FetchXML()
	if err != nil {
		log.Fatalf("fetch XML failed: %v", err)
	}
	log.Printf("[INFO] Retrieved XML context (%d bytes)", len(xml))
	if len(xml) > 0 {
		preview := xml
		if len(preview) > 256 {
			preview = preview[:256]
		}
		log.Printf("[INFO] XML preview: %q...", preview)
	}

	log.Printf("[INFO] Opening buffer: device=cf-ad9361-lpc samples=%d mask=%s cyclic=%v", *samples, *mask, *cyclic)
	if err := m.OpenBufferASCII("cf-ad9361-lpc", *samples, *mask, *cyclic); err != nil {
		log.Fatalf("open buffer failed: %v", err)
	}
	defer func() {
		if err := m.CloseBufferASCII("cf-ad9361-lpc"); err != nil {
			log.Printf("[WARN] close buffer error: %v", err)
		}
	}()

	requested := *readBytes
	if requested <= 0 {
		requested = int(*samples)
	}
	log.Printf("[INFO] Preparing READBUF request: bytes=%d (samples=%d)", requested, *samples)
	buf := make([]byte, requested)

	// We use the standard ReadBufferASCII. Because we wrapped the connection in
	// loggingConn, the user can verify the "Mask" line existence by looking at
	// the stdout logs.
	log.Printf("[INFO] Sending READBUF via Manager...")

	n, err := m.ReadBufferASCII("cf-ad9361-lpc", buf)
	if err != nil {
		log.Fatalf("read buffer failed: %v", err)
	}
	log.Printf("[INFO] ReadBufferASCII success: received %d bytes", n)

	previewLen := n
	if previewLen > 32 {
		previewLen = 32
	}
	preview := strings.ToUpper(hex.EncodeToString(buf[:previewLen]))
	log.Printf("[INFO] Sample preview (%d bytes): %s", previewLen, preview)

	if n < requested {
		log.Printf("[WARN] Requested %d bytes but received %d", requested, n)
	}

	if err := m.Close(); err != nil {
		log.Printf("[WARN] connection close error: %v", err)
	}

	log.Println("[DONE] ASCII diagnostic completed")
}
