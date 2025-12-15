package main

import (
	"fmt"
	"log"
	"time"

	"github.com/rjboer/GoSDR/internal/connectionmgr"
)

func main() {
	// ---------------------------------------------------------------------
	// Configuration (no flags, just variables you edit by hand)
	// ---------------------------------------------------------------------
	iiodAddress := "192.168.2.1:30431" // or "pluto.local:30431"
	ioTimeout := 5 * time.Second       // end-to-end I/O timeout
	doBinaryHandshakeTest := true      // set to false to skip BINARY test
	printFullXML := false              // true = dump full XML, false = preview

	// ---------------------------------------------------------------------
	// Logging setup
	// ---------------------------------------------------------------------
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	log.Println("====================================================")
	log.Println(" IIOD Connection Manager Test")
	log.Println("====================================================")
	log.Printf("Config: address=%s, timeout=%s, binaryTest=%v, printFullXML=%v",
		iiodAddress, ioTimeout, doBinaryHandshakeTest, printFullXML)

	// ---------------------------------------------------------------------
	// 1. Connect (ASCII mode)
	// ---------------------------------------------------------------------
	log.Println("[STEP 1] Creating connection manager...")
	m := connectionmgr.New(iiodAddress)
	m.SetTimeout(ioTimeout)
	log.Printf("Manager created: Address=%s Mode=%v Timeout=%s",
		m.Address, m.Mode, m.Timeout)

	log.Println("[STEP 2] Connecting to IIOD...")
	if err := m.Connect(); err != nil {
		log.Fatalf("[FATAL] Connect failed: %v", err)
	}
	defer func() {
		log.Println("[CLEANUP] Closing primary connection...")
		if err := m.Close(); err != nil {
			log.Printf("[WARN] Close error: %v", err)
		} else {
			log.Println("[CLEANUP] Primary connection closed.")
		}
	}()

	log.Printf("[INFO] Connected to %s (mode=%v)", m.Address, m.Mode)

	// ---------------------------------------------------------------------
	// 2. Set remote timeout via TIMEOUT (same pattern as libiio)
	// ---------------------------------------------------------------------
	log.Println("[STEP 3] Setting remote TIMEOUT...")
	remoteMs := (ioTimeout / 2).Milliseconds()
	timeoutCmd := fmt.Sprintf("TIMEOUT %d", remoteMs)

	log.Printf("[DEBUG] Executing ASCII command: %q", timeoutCmd)
	if ret, err := m.ExecCommand(timeoutCmd); err != nil {
		log.Printf("[WARN] TIMEOUT command error (tinyiiod may not support this): %v", err)
	} else {
		log.Printf("[INFO] TIMEOUT response integer=%d", ret)
	}

	// ---------------------------------------------------------------------
	// 3. Fetch context XML via PRINT
	// ---------------------------------------------------------------------
	log.Println("[STEP 4] Fetching context XML via PRINT...")
	xml, err := m.FetchXML()
	if err != nil {
		log.Fatalf("[FATAL] FetchXML failed: %v", err)
	}

	log.Printf("[INFO] XML received: %d bytes", len(xml))

	if printFullXML {
		log.Println("[DEBUG] Printing full XML:")
		fmt.Println("=========== BEGIN XML ===========")
		fmt.Println(string(xml))
		fmt.Println("=========== END XML =============")
	} else {
		const preview = 600
		log.Printf("[DEBUG] Printing XML preview (first %d bytes or less)...", preview)
		fmt.Println("=========== XML PREVIEW =========")
		if len(xml) > preview {
			fmt.Println(string(xml[:preview]))
			fmt.Println("... [truncated]")
		} else {
			fmt.Println(string(xml))
		}
		fmt.Println("=========== END PREVIEW =========")
	}

	// Here is where you would normally hand the XML off to your own parser:
	//
	//   ctx, err := yourxmlpkg.ParseContext(xml)
	//   if err != nil {
	//       log.Fatalf("[FATAL] XML parse error: %v", err)
	//   }
	//   log.Printf("[INFO] Parsed context: %d devices", len(ctx.Devices))
	//
	// For now we only exercise transport & basic commands.

	// ---------------------------------------------------------------------
	// 4. Optional: test BINARY handshake on a *separate* connection
	// ---------------------------------------------------------------------
	if doBinaryHandshakeTest {
		log.Println("[STEP 5] Testing BINARY handshake on a new connection...")
		bm := connectionmgr.New(iiodAddress)
		bm.SetTimeout(ioTimeout)

		log.Printf("[DEBUG] Binary-test manager created: Address=%s Timeout=%s",
			bm.Address, bm.Timeout)

		log.Println("[DEBUG] Connecting binary-test manager...")
		if err := bm.Connect(); err != nil {
			log.Fatalf("[FATAL] Binary-test connect failed: %v", err)
		}
		defer func() {
			log.Println("[CLEANUP] Closing binary-test connection...")
			if err := bm.Close(); err != nil {
				log.Printf("[WARN] Close error (binary-test): %v", err)
			} else {
				log.Println("[CLEANUP] Binary-test connection closed.")
			}
		}()

		log.Printf("[INFO] Binary-test connection established (mode=%v)", bm.Mode)

		log.Println("[DEBUG] Sending BINARY command...")
		ok, err := bm.TryUpgradeToBinary()
		if err != nil {
			log.Printf("[ERROR] BINARY handshake error: %v", err)
		} else if ok {
			log.Printf("[INFO] BINARY handshake succeeded, mode now=%v", bm.Mode)
		} else {
			log.Printf("[INFO] BINARY handshake not supported (non-zero return code); staying ASCII is fine.")
		}
	} else {
		log.Println("[STEP 5] Skipping BINARY handshake test (doBinaryHandshakeTest=false).")
	}

	log.Println("====================================================")
	log.Println(" All steps completed. Connection manager test done.")
	log.Println("====================================================")
}
