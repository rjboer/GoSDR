package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	plutoAddr = "192.168.2.1:30431"

	// confirmed responder opcode
	opGetVersion = 0x01
)

func main() {
	workingprobe4()
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func readLine(conn net.Conn) string {
	var buf []byte
	tmp := make([]byte, 1)
	for {
		// _, err := conn.ReadAll(tmp)
		// conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, err := conn.Read(tmp)
		must(err)
		buf = append(buf, tmp[0])
		if tmp[0] == '\n' {
			return string(buf)
		}
	}
}

func writeASCII(conn net.Conn, s string) {
	log.Printf("[ASCII TX] %q", s)
	_, err := conn.Write([]byte(s + "\n"))
	must(err)
}

func writeBinaryHeader(conn net.Conn, clientID uint16, op, dev uint8, code int32) {
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint16(hdr[0:2], clientID)
	hdr[2] = op
	hdr[3] = dev
	binary.BigEndian.PutUint32(hdr[4:8], uint32(code))

	log.Printf("[BIN TX] % X (client=%d op=%d dev=%d code=%d)",
		hdr, clientID, op, dev, code)

	_, err := conn.Write(hdr)
	must(err)
}

func readBinaryHeader(conn net.Conn) {
	hdr := make([]byte, 8)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err := io.ReadFull(conn, hdr)
	must(err)

	log.Printf("[BIN RX] % X", hdr)

	clientID := binary.BigEndian.Uint16(hdr[0:2])
	op := hdr[2]
	dev := hdr[3]
	code := int32(binary.BigEndian.Uint32(hdr[4:8]))

	log.Printf("[BIN RX DECODED] client=%d op=%d dev=%d code=%d",
		clientID, op, dev, code)
}

func workingprobe4() {
	// ============================================================
	// PlutoSDR IIOD ASCII ReadAll Probe with Lock + Splitter
	// ============================================================

	conn, err := net.Dial("tcp", plutoAddr)
	if err != nil {
		log.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Time{}) // clear deadline

	// ------------------------------
	// Shared state protected by lock
	// ------------------------------
	var (
		mu      sync.RWMutex
		lines   []string
		running = true
	)

	// ------------------------------
	// ASYNC READALL LOOP
	// ------------------------------
	go func() {
		defer func() {
			log.Println("[READER] stopped")
		}()

		buf := make([]byte, 4096)
		var acc []byte

		for running {
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

			n, err := conn.Read(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					continue
				}
				log.Printf("[READER] read error: %v", err)
				return
			}

			if n == 0 {
				continue
			}

			acc = append(acc, buf[:n]...)

			// Split on '\n'
			for {
				i := bytes.IndexByte(acc, '\n')
				if i < 0 {
					break
				}

				line := string(acc[:i+1])
				acc = acc[i+1:]

				mu.Lock()
				lines = append(lines, line)
				mu.Unlock()

				log.Printf("[ASCII RX] %q", line)
			}
		}
	}()

	// ------------------------------
	// Helper: write ASCII command
	// ------------------------------
	writeASCII := func(cmd string) {
		log.Printf("[ASCII TX] %q", cmd)
		_, err := conn.Write([]byte(cmd + "\n"))
		if err != nil {
			log.Fatalf("write failed: %v", err)
		}
	}

	// ------------------------------
	// Helper: read next ASCII line
	// ------------------------------
	readLine := func(timeout time.Duration) string {
		deadline := time.Now().Add(timeout)

		for time.Now().Before(deadline) {
			mu.RLock()
			if len(lines) > 0 {
				line := lines[0]
				mu.RUnlock()

				mu.Lock()
				lines = lines[1:]
				mu.Unlock()

				return line
			}
			mu.RUnlock()

			time.Sleep(10 * time.Millisecond)
		}

		log.Fatalf("timeout waiting for ASCII line")
		return ""
	}

	// ============================================================
	// ASCII PHASE
	// ============================================================

	writeASCII("TIMEOUT 2000")
	_ = readLine(2 * time.Second)

	writeASCII("PRINT")
	lenLine := readLine(2 * time.Second)
	log.Printf("[ASCII RX] XML length = %q", strings.TrimSpace(lenLine))

	xmlLen, err := strconv.Atoi(strings.TrimSpace(lenLine))
	if err != nil {
		log.Fatalf("invalid XML length: %v", err)
	}

	// Drain XML payload (already buffered by reader)
	readBytes := 0
	for readBytes < xmlLen {
		line := readLine(2 * time.Second)
		readBytes += len(line)
	}

	log.Printf("[ASCII RX] XML received (%d bytes)", xmlLen)

	// ============================================================
	// STOP ASCII READER CLEANLY
	// ============================================================

	running = false
	time.Sleep(100 * time.Millisecond)

	log.Println("====================================================")
	log.Println("ASCII readAll + splitter confirmed operational")
	log.Println("====================================================")
}
