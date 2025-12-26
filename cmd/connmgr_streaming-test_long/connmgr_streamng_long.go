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
	// STEP 2 — Set TIMEOUT
	// ------------------------------------------------------------------
	log.Println("[STEP 2] Setting remote TIMEOUT...")
	if ret, err := m.ExecCommand("TIMEOUT 2000"); err != nil {
		log.Printf("[WARN] TIMEOUT failed: %v", err)
	} else {
		log.Printf("[INFO] TIMEOUT response=%d", ret)
	}

	// ------------------------------------------------------------------
	// STEP 3 — Fetch context XML (sanity check)
	// ------------------------------------------------------------------
	log.Println("[STEP 3] Fetching XML...")
	// xml, err := m.FetchXML()
	// if err != nil {
	// 	log.Fatalf("FetchXML failed: %v", err)
	// }
	// log.Printf("[INFO] XML size = %d bytes\n", len(xml))

	// ------------------------------------------------------------------
	// STEP 4 — Basic RX attribute setup (best effort)
	// ------------------------------------------------------------------
	log.Println("[STEP 4] Configuring RX attributes (best effort)")

	// // RX attributes should use isOutput=false (INPUT)
	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false,
	// 	"altvoltage0", "frequency", "915000000")

	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false,
	// 	"voltage0", "sampling_frequency", "2000000")

	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false,
	// 	"voltage0", "hardwaregain", "20")

	type AttrCheck struct {
		Dev      string
		IsOutput bool
		Chan     string
		Attr     string
		Want     string
	}

	// checks := []AttrCheck{
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "altvoltage0", Attr: "frequency", Want: "915000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "voltage0", Attr: "sampling_frequency", Want: "2000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "voltage0", Attr: "sampling_frequency", Want: "2000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "voltage0", Attr: "sampling_frequency", Want: "2000000"},

	// 	// Gain is only meaningful if gain mode is manual
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "voltage0", Attr: "gain_control_mode", Want: "manual"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "voltage0", Attr: "hardwaregain", Want: "20"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "altvoltage0", Attr: "frequency", Want: "915000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "altvoltage0", Attr: "frequency", Want: "915000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "altvoltage0", Attr: "frequency", Want: "915000000"},
	// 	// {Dev: "ad9361-phy", IsOutput: false, Chan: "altvoltage0", Attr: "frequency", Want: "915000000"},
	// }

	// 1) write in correct order
	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false, "altvoltage0", "frequency", "915000000")
	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false, "voltage0", "sampling_frequency", "2000000")
	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false, "voltage0", "gain_control_mode", "manual")
	// _, _ = m.WriteChannelAttrASCII("ad9361-phy", false, "voltage0", "hardwaregain", "20")

	// // 2) read-back verify
	// for _, c := range checks {
	// 	got, rc, err := m.ReadChannelAttrASCII2(c.Dev, c.IsOutput, c.Chan, c.Attr)
	// 	if err != nil {
	// 		log.Printf("[ATTR][READ][ERR] %s/%v/%s/%s: %v", c.Dev, c.IsOutput, c.Chan, c.Attr, err)
	// 		continue
	// 	}
	// 	if rc != 0 {
	// 		log.Printf("[ATTR][READ][RC=%d] %s/%v/%s/%s", rc, c.Dev, c.IsOutput, c.Chan, c.Attr)
	// 		continue
	// 	}
	// 	got = strings.TrimSpace(got)
	// 	if got != c.Want {
	// 		log.Printf("[ATTR][MISMATCH] %s/%v/%s/%s want=%q got=%q", c.Dev, c.IsOutput, c.Chan, c.Attr, c.Want, got)
	// 	} else {
	// 		log.Printf("[ATTR][OK] %s/%v/%s/%s=%q", c.Dev, c.IsOutput, c.Chan, c.Attr, got)
	// 	}
	// }

	// Attribute failures are expected and not fatal here

	// Drain any remaining ASCII responses
	// m.DrainASCII()
	// ------------------------------------------------------------------
	// STEP 5 — Entering binary streaming mode
	// ------------------------------------------------------------------
	log.Println("[STEP 5] setting the internal binary flag")
	if err := m.EnterBinaryMode3(); err != nil {
		log.Fatalf("EnterBinaryMode failed: %v", err)
	}
	// m.Mode = connectionmgr.ModeBinary

	// ------------------------------------------------------------------
	// STEP 6 — Create RX buffer
	// ------------------------------------------------------------------
	log.Println("[STEP 6] Sending CREATE_BUFFER (binary)")

	buf, err := m.CreateBuffer3(
		0,             // device index (cf-ad9361-lpc)
		[]uint8{0, 1}, // I/Q channels
		true,          // RX
	)
	if err != nil {
		log.Fatalf("CREATE_BUFFER failed: %v", err)
	}

	log.Printf("[OK] CREATE_BUFFER returned bufferID=%d", buf.ID)

	// ------------------------------------------------------------------
	// STEP 7 — Create RX buffer
	// ------------------------------------------------------------------
	// log.Println("[STEP 7] CreateBuffer...")
	// For cf-ad9361-lpc, dev=0 and channels 0,1 are typical for I/Q
	// rxBuf, err := m.CreateBuffer(0, []uint8{0, 1}, false)
	// if err != nil {
	// 	log.Fatalf("CreateBuffer failed: %v", err)
	// }

	log.Println("[STEP 7] EnableBuffer...")
	if err := m.EnableBuffer(buf); err != nil {
		log.Fatalf("EnableBuffer failed: %v", err)
	}

	// ------------------------------------------------------------------
	// STEP 8 — Create RX block
	// ------------------------------------------------------------------
	log.Println("[STEP 8] CreateBlock...")
	rxBlock, err := m.CreateBlock(buf, blockSize)
	if err != nil {
		log.Fatalf("CreateBlock failed: %v", err)
	}

	// ------------------------------------------------------------------
	// STEP 9 — Streaming loop (finite)
	// ------------------------------------------------------------------
	log.Println("[STEP 9] Starting RX stream...")

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
	// STEP 10 — Cleanup
	// ------------------------------------------------------------------
	log.Println("[STEP 10] Disabling buffer...")
	_ = m.DisableBuffer(buf)

	log.Println("Test completed successfully.")
}
