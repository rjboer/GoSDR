package connectionmgr

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"
	"regexp"
	"time"
)

func (m *Manager) SocketReader(out chan<- []byte) {
	if m.conn == nil {
		//there is no connection yet, so we need to create one
		c, err := net.DialTimeout("tcp", m.Address, m.Timeout)
		if err != nil {
			log.Println("Connect failed, please check firewall settings, adaptors and vpn settings")
			log.Println("This can be done by  running the command:Test-NetConnection 192.168.2.1 -Port 30431 in powershell")
			log.Println("Connect failed: %v", err)
			return
		}
		m.conn = c
		m.clientID = 0
		log.Println("Connected to", m.Address)
	}
	defer m.conn.Close()
	conn := m.conn
	buf := make([]byte, 4096)
	log.Println("Starting reader(RX channel), buffersize:", len(buf))
	for {
		n, err := conn.Read(buf)
		if err != nil {
			close(out)
			return
		}
		chunk := make([]byte, n)
		copy(chunk, buf[:n])
		out <- chunk
	}
}

func (m *Manager) ConsumeStream(
	ctrl <-chan any,
	onASCIILine func(line string),
	onBinaryFrame func(hdr BinHdr, payload []byte),
) {
	// ---------------------------
	// Local types and controls
	// ---------------------------
	type modeT int
	const (
		modeASCII modeT = iota
		modeBinary
	)

	// Control messages you can send on ctrl channel:
	type SetMode string               // "ascii" or "binary"
	type SetBinPayload int            // set expected payload length after each 8-byte header (for ops where caller knows)
	type SetDebug bool                // enable/disable verbose logs
	type SetReadTimeout time.Duration // optional: only affects logs/expectations; real deadlines belong in the socket reader

	// Binary header (IIOD command/response header is 8 bytes)
	type BinHdr struct {
		ClientID uint16
		Op       uint8
		Dev      uint8
		Code     int32
	}

	// Export BinHdr type name to callback signature
	type _exportBinHdr = BinHdr
	_ = _exportBinHdr{}

	// ---------------------------
	// State
	// ---------------------------
	var (
		mode           = modeASCII
		expectBinBytes = 0 // payload length after each 8-byte header (0 means "header-only / unknown")
		debug          = true
		readTimeout    = 5 * time.Second

		asciiBuf bytes.Buffer
		binBuf   bytes.Buffer
	)

	// Regex that extracts newline-terminated messages.
	// This finds the *first* shortest line ending in '\n' repeatedly.
	lineRE := regexp.MustCompile(`(?s)^.*?\n`)

	logf := func(format string, args ...any) {
		if debug {
			log.Printf(format, args...)
		}
	}

	// Helper: decode 8-byte header (network byte order)
	decodeHdr := func(b []byte) BinHdr {
		// b must be len>=8
		return BinHdr{
			ClientID: binary.BigEndian.Uint16(b[0:2]),
			Op:       b[2],
			Dev:      b[3],
			Code:     int32(binary.BigEndian.Uint32(b[4:8])),
		}
	}

	// Drain ASCII buffer by regex framing, handling partial last line.
	drainASCII := func() {
		for {
			data := asciiBuf.Bytes()
			if len(data) == 0 {
				return
			}
			loc := lineRE.FindIndex(data)
			if loc == nil || loc[0] != 0 {
				// No complete line at start (partial line); wait for more bytes.
				return
			}
			lineBytes := data[loc[0]:loc[1]]
			asciiBuf.Next(loc[1]) // consume
			line := string(lineBytes)
			logf("[DISPATCH][ASCII] line=%q (buf_remain=%d)", line, asciiBuf.Len())
			if onASCIILine != nil {
				onASCIILine(line)
			}
		}
	}

	// Drain binary buffer by fixed framing: 8-byte header + expectBinBytes payload.
	// If expectBinBytes==0: emit header-only frames (useful for probing GET_VERSION, etc.),
	// but note: many responses may actually include extra bytes depending on opcode.
	drainBinary := func() {
		for {
			if binBuf.Len() < 8 {
				return
			}

			peek := binBuf.Bytes()
			hdrBytes := peek[:8]
			hdr := decodeHdr(hdrBytes)

			need := 8 + expectBinBytes
			if binBuf.Len() < need {
				// partial frame
				logf("[DISPATCH][BIN] partial: have=%d need=%d (waiting) hdr_peek=% X",
					binBuf.Len(), need, hdrBytes)
				return
			}

			// Consume header
			_ = binBuf.Next(8)

			// Consume payload if configured
			var payload []byte
			if expectBinBytes > 0 {
				payload = make([]byte, expectBinBytes)
				copy(payload, binBuf.Next(expectBinBytes))
			} else {
				payload = nil
			}

			logf("[DISPATCH][BIN] hdr={client=%d op=%d dev=%d code=%d} payload=%d buf_remain=%d",
				hdr.ClientID, hdr.Op, hdr.Dev, hdr.Code, len(payload), binBuf.Len())

			if onBinaryFrame != nil {
				onBinaryFrame(hdr, payload)
			}
		}
	}

	// ---------------------------
	// Main loop
	// ---------------------------
	log.Printf("[DISPATCH] starting (mode=%v expectBinBytes=%d timeout=%s)", mode, expectBinBytes, readTimeout)

	for {
		select {
		case cmsg, ok := <-ctrl:
			if !ok {
				// Control channel closed; continue running on input only.
				ctrl = nil
				continue
			}
			switch v := cmsg.(type) {
			case SetMode:
				switch string(v) {
				case "ascii", "ASCII":
					mode = modeASCII
					log.Printf("[CTRL] mode=ASCII")
					// When switching to ASCII, do not discard binBuf; you may want to keep it
					// for debug. If you want strict separation, explicitly reset it:
					// binBuf.Reset()
				case "binary", "BINARY":
					mode = modeBinary
					log.Printf("[CTRL] mode=BINARY")
					// Same note as above; consider asciiBuf.Reset() if you want a hard cutover.
				default:
					log.Printf("[CTRL][WARN] unknown mode %q (ignored)", string(v))
				}
			case SetBinPayload:
				if int(v) < 0 {
					log.Printf("[CTRL][WARN] negative payload size %d (ignored)", int(v))
					continue
				}
				expectBinBytes = int(v)
				log.Printf("[CTRL] expectBinBytes=%d", expectBinBytes)

			case SetDebug:
				debug = bool(v)
				log.Printf("[CTRL] debug=%v", debug)

			case SetReadTimeout:
				readTimeout = time.Duration(v)
				log.Printf("[CTRL] readTimeout=%s (note: actual conn deadlines belong in reader)", readTimeout)

			default:
				log.Printf("[CTRL][WARN] unsupported control type %T (ignored)", v)
			}

			// After mode change or payload update, try draining any already-buffered bytes.
			if mode == modeASCII {
				drainASCII()
			} else {
				drainBinary()
			}

		case chunk, ok := <-in:
			if !ok {
				log.Printf("[DISPATCH] input closed; draining buffers then exit (ascii=%d bin=%d)",
					asciiBuf.Len(), binBuf.Len())
				// Best-effort drain
				drainASCII()
				drainBinary()
				return
			}
			if len(chunk) == 0 {
				continue
			}

			logf("[RX] chunk=%d bytes, mode=%v", len(chunk), mode)

			if mode == modeASCII {
				asciiBuf.Write(chunk)
				logf("[ASCII BUF] size=%d", asciiBuf.Len())
				drainASCII()
			} else {
				binBuf.Write(chunk)
				logf("[BIN BUF] size=%d (expect payload=%d)", binBuf.Len(), expectBinBytes)
				drainBinary()
			}
		}
	}
}

// BinHdr is the binary header type used by the binary callback.
// (This is duplicated outside ConsumeStream only so it can appear in signatures cleanly.)
type BinHdr struct {
	ClientID uint16
	Op       uint8
	Dev      uint8
	Code     int32
}
