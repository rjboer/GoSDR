package main

import (
	"log"

	"github.com/rjboer/GoSDR/internal/connectionmgr"
)

const (
	plutoAddr = "192.168.2.1:30431"
	device    = "cf-ad9361-lpc"
	blockSize = 65536
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	log.Println("====================================================")
	log.Println(" PlutoSDR Binary RX Streaming Integration Test")
	log.Println("====================================================")

	m := connectionmgr.New(plutoAddr)

	// ------------------------------------------------------------------
	// STEP 1 — Connect (ASCII mode)
	// ------------------------------------------------------------------
	log.Println("[STEP 1] Connecting to PlutoSDR...")
	if err := m.Connect(); err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer m.Close()

	// ------------------------------------------------------------------
	// STEP 2 — Fetch context XML (sanity check)
	// ------------------------------------------------------------------
	log.Println("[STEP 2] Fetching XML...")
	xml, err := m.FetchXML()
	if err != nil {
		log.Fatalf("PRINT failed: %v", err)
	}
	log.Printf("[INFO] XML size = %d bytes\n", len(xml))

	// ------------------------------------------------------------------
	// STEP 3 — Basic RX attribute setup (best effort)
	// ------------------------------------------------------------------
	log.Println("[STEP 3] Configuring RX attributes (best effort)")

	_, _ = m.WriteChannelAttrASCII("ad9361-phy", true,
		"altvoltage0", "frequency", "915000000")

	_, _ = m.WriteChannelAttrASCII("ad9361-phy", true,
		"voltage0", "sampling_frequency", "2000000")

	_, _ = m.WriteChannelAttrASCII("ad9361-phy", true,
		"voltage0", "hardwaregain", "20")

	// Attribute failures are expected and not fatal here

	// ------------------------------------------------------------------
	// STEP 4 — Upgrade to binary protocol
	// ------------------------------------------------------------------
	log.Println("[STEP 4] Switching to binary mode...")
	if ok, err := m.TryUpgradeToBinary(); err != nil || !ok {
		log.Fatalf("binary upgrade failed: ok=%v, err=%v", ok, err)
	}

	// ------------------------------------------------------------------
	// STEP 5 — Create RX buffer
	// ------------------------------------------------------------------
	log.Println("[STEP 5] CreateBuffer...")
	// For cf-ad9361-lpc, dev=0 and channels 0,1 are typical for I/Q
	rxBuf, err := m.CreateBuffer(0, []uint8{0, 1}, false)
	if err != nil {
		log.Fatalf("CreateBuffer failed: %v", err)
	}

	log.Println("[STEP 6] EnableBuffer...")
	if err := m.EnableBuffer(rxBuf); err != nil {
		log.Fatalf("EnableBuffer failed: %v", err)
	}

	// ------------------------------------------------------------------
	// STEP 7 — Create RX block
	// ------------------------------------------------------------------
	log.Println("[STEP 7] CreateBlock...")
	rxBlock, err := m.CreateBlock(rxBuf, blockSize)
	if err != nil {
		log.Fatalf("CreateBlock failed: %v", err)
	}

	// ------------------------------------------------------------------
	// STEP 8 — Streaming loop (finite)
	// ------------------------------------------------------------------
	log.Println("[STEP 8] Starting RX stream...")

	const iterations = 10000
	totalBytes := 0
	data := make([]byte, blockSize)

	for i := 0; i < iterations; i++ {
		ret, err := m.TransferBlock(rxBlock, data)
		if err != nil {
			log.Fatalf("TransferBlock error: %v", err)
		}
		if ret < 0 {
			log.Fatalf("TransferBlock returned error code %d", ret)
		}
		if ret == 0 {
			log.Fatalf("TransferBlock returned zero-length payload")
		}

		totalBytes += ret

		if (i+1)%1000 == 0 {
			log.Printf(
				"[RX] iter=%d bytes=%d total=%d\n",
				i+1, ret, totalBytes,
			)
		}
	}

	log.Println("----------------------------------------------------")
	log.Printf(" RX streaming OK — received %d bytes total\n", totalBytes)
	log.Println("----------------------------------------------------")

	// ------------------------------------------------------------------
	// STEP 9 — Cleanup
	// ------------------------------------------------------------------
	log.Println("[STEP 9] Disabling buffer...")
	_ = m.DisableBuffer(rxBuf)

	log.Println("Test completed successfully.")
}
