package main

import (
	"fmt"
	"log"
	"time"

	"github.com/rjboer/GoSDR/internal/connectionmgr"
)

func main() {
	// ---------------------------------------------------------------------
	// Configuration
	// ---------------------------------------------------------------------
	iiodAddress := "192.168.2.1:30431"
	ioTimeout := 5 * time.Second
	doBinaryHandshakeTest := true

	deviceID := "cf-ad9361-lpc"
	channelMask := "00000003" // RX channels 0 + 1
	samples := uint64(1024)

	// ---------------------------------------------------------------------
	// Logging
	// ---------------------------------------------------------------------
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.Println("====================================================")
	log.Println(" IIOD Connection Manager Streaming Test")
	log.Println("====================================================")

	// ---------------------------------------------------------------------
	// 1. Connect
	// ---------------------------------------------------------------------
	log.Println("[STEP 1] Creating connection manager...")
	m := connectionmgr.New(iiodAddress)
	m.SetTimeout(ioTimeout)

	log.Println("[STEP 2] Connecting to IIOD...")
	if err := m.Connect(); err != nil {
		log.Fatalf("Connect failed: %v", err)
	}
	defer func() {
		log.Println("[CLEANUP] Closing connection...")
		_ = m.Close()
	}()

	log.Printf("[INFO] Connected to %s (mode=%v)", m.Address, m.Mode)

	// ---------------------------------------------------------------------
	// 2. TIMEOUT
	// ---------------------------------------------------------------------
	log.Println("[STEP 3] Setting remote TIMEOUT...")
	remoteMs := (ioTimeout / 2).Milliseconds()
	if ret, err := m.ExecCommand(fmt.Sprintf("TIMEOUT %d", remoteMs)); err != nil {
		log.Printf("[WARN] TIMEOUT failed: %v", err)
	} else {
		log.Printf("[INFO] TIMEOUT response=%d", ret)
	}

	// ---------------------------------------------------------------------
	// 3. Fetch XML
	// ---------------------------------------------------------------------
	log.Println("[STEP 4] Fetching context XML...")
	xml, err := m.FetchXML()
	if err != nil {
		log.Fatalf("FetchXML failed: %v", err)
	}
	log.Printf("[INFO] XML received: %d bytes", len(xml))

	// ---------------------------------------------------------------------
	// 4. Open ASCII buffer
	// ---------------------------------------------------------------------
	log.Println("[STEP 5] Opening ASCII buffer...")
	if err := m.OpenBufferASCII(deviceID, samples, channelMask, false); err != nil {
		log.Fatalf("OPEN failed: %v", err)
	}
	defer func() {
		log.Println("[CLEANUP] Closing buffer...")
		_ = m.CloseBufferASCII(deviceID)
	}()

	// ---------------------------------------------------------------------
	// 5. Read samples
	// ---------------------------------------------------------------------
	log.Println("[STEP 6] Reading samples...")
	buf := make([]byte, 64*1024)

	n, err := m.ReadBufferASCII(deviceID, buf)
	if err != nil {
		log.Fatalf("READBUF failed: %v", err)
	}

	log.Printf("[INFO] Read %d bytes from %s", n, deviceID)

	// At this point:
	//   buf[:n] → feed directly into your demuxer / extract pipeline

	// ---------------------------------------------------------------------
	// 6. Optional: test BINARY handshake
	// ---------------------------------------------------------------------
	if doBinaryHandshakeTest {
		log.Println("[STEP 7] Testing BINARY handshake on new connection...")
		bm := connectionmgr.New(iiodAddress)
		bm.SetTimeout(ioTimeout)

		if err := bm.Connect(); err != nil {
			log.Fatalf("Binary-test connect failed: %v", err)
		}
		defer bm.Close()

		ok, err := bm.TryUpgradeToBinary()
		if err != nil {
			log.Printf("[ERROR] BINARY handshake error: %v", err)
		} else if ok {
			log.Printf("[INFO] BINARY supported (unexpected on Pluto)")
		} else {
			log.Printf("[INFO] BINARY not supported (expected)")
		}
	}

	log.Println("====================================================")
	log.Println(" Streaming test completed successfully")
	log.Println("====================================================")
}
