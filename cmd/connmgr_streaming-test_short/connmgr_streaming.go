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
	blockSize := 16 * 1024
	devIndex := uint8(0)      // Target device index for the binary buffer
	channels := []uint8{0, 1} // RX channel indices to enable
	streamTransfers := 5      // Number of blocks to read before exiting
	cyclic := false           // Whether to enqueue blocks cyclically

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
	// 4. Enter BINARY mode
	// ---------------------------------------------------------------------
	log.Println("[STEP 5] Entering binary streaming mode...")
	if err := m.EnterBinaryMode(); err != nil {
		log.Fatalf("EnterBinaryMode failed: %v", err)
	}

	// ---------------------------------------------------------------------
	// 5. Create/enable binary buffer
	// ---------------------------------------------------------------------
	log.Println("[STEP 6] Creating binary buffer...")
	buf, err := m.CreateBuffer(devIndex, channels, cyclic)
	if err != nil {
		log.Fatalf("CreateBuffer failed: %v", err)
	}
	defer func() {
		log.Println("[CLEANUP] Disabling buffer...")
		if err := m.DisableBuffer(buf); err != nil {
			log.Printf("[WARN] DisableBuffer error: %v", err)
		}
		log.Println("[CLEANUP] Freeing buffer...")
		if err := m.FreeBuffer(buf); err != nil {
			log.Printf("[WARN] FreeBuffer error: %v", err)
		}
	}()

	log.Println("[STEP 7] Enabling buffer...")
	if err := m.EnableBuffer(buf); err != nil {
		log.Fatalf("EnableBuffer failed: %v", err)
	}

	// ---------------------------------------------------------------------
	// 6. Create block
	// ---------------------------------------------------------------------
	log.Printf("[STEP 8] Creating block (size=%d)...", blockSize)
	blk, err := m.CreateBlock(buf, blockSize)
	if err != nil {
		log.Fatalf("CreateBlock failed: %v", err)
	}
	defer func() {
		log.Println("[CLEANUP] Freeing block...")
		if err := m.FreeBlock(blk); err != nil {
			log.Printf("[WARN] FreeBlock error: %v", err)
		}
	}()

	// ---------------------------------------------------------------------
	// 7. Stream blocks
	// ---------------------------------------------------------------------
	log.Println("[STEP 9] Streaming RX data...")
	payload := make([]byte, blockSize)
	for i := 0; i < streamTransfers; i++ {
		n, err := m.TransferBlock(blk, payload)
		if err != nil {
			log.Fatalf("TransferBlock failed: %v", err)
		}
		log.Printf("[STREAM] Block %d received %d bytes", i+1, n)
	}

	log.Println("====================================================")
	log.Println(" Streaming test completed successfully")
	log.Println("====================================================")
}
