package main

import (
	"context"
	"log"
	"time"

	"github.com/rjboer/GoSDR/internal/connectionmgr"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)

	addr := "192.168.2.1:30431"
	m := connectionmgr.New(addr)
	m.SetTimeout(15 * time.Second)

	if err := m.Connect(); err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer m.Close()

	_, _ = m.ExecCommand("TIMEOUT 2500")

	xml, err := m.FetchXML()
	if err != nil {
		log.Fatalf("PRINT/XML: %v", err)
	}
	log.Printf("XML bytes=%d", len(xml))

	// -----------------------------
	// Step 3: Attribute control
	// -----------------------------
	// NOTE: You MUST confirm these IDs in YOUR XML.
	// Typical Pluto/AD9361 examples:
	//   dev: "ad9361-phy"
	//   LO RX: chan "altvoltage0" attr "frequency"
	//   sampling_frequency: chan "voltage0" attr "sampling_frequency"
	//   hardwaregain: chan "voltage0" attr "hardwaregain"

	if err := m.SetLOFrequencyHzASCII("ad9361-phy", false, "altvoltage0", 915_000_000); err != nil {
		log.Printf("set LO: %v", err)
	}
	if err := m.SetSampleRateHzASCII("ad9361-phy", false, "voltage0", 2_000_000); err != nil {
		log.Printf("set sample rate: %v", err)
	}
	if err := m.SetHardwareGainDBASCII("ad9361-phy", false, "voltage0", 20.0); err != nil {
		log.Printf("set gain: %v", err)
	}

	// -----------------------------
	// Step 2: Open + stream
	// -----------------------------
	deviceID := "cf-ad9361-lpc"
	maskHex := "00000003" // RX0+RX1 (your earlier working mask)
	if err := m.OpenBufferASCII(deviceID, 1024, maskHex, false); err != nil {
		log.Fatalf("OPEN: %v", err)
	}
	defer m.CloseBufferASCII(deviceID)

	out := make(chan []byte, 8) // buffer = backpressure control

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := m.StartStreamASCII(ctx, connectionmgr.StreamASCIIConfig{
		DeviceID:            deviceID,
		BytesPerRead:        65536,
		Out:                 out,
		DropIfFull:          false, // true = drop frames instead of blocking
		CopyOut:             true,  // safest until you add pooling
		ReadTimeoutPerChunk: 15 * time.Second,
		LogPrefix:           "rx",
	})
	if err != nil {
		log.Fatalf("start stream: %v", err)
	}

	// Consumer: for now just log chunk sizes. Next you will call your demuxer here.
	go func() {
		for b := range out {
			log.Printf("[consumer] got chunk bytes=%d", len(b))
			// TODO: demux I/Q here
		}
	}()

	// Run briefly, then stop
	time.Sleep(3 * time.Second)
	h.Stop()
	log.Printf("stream stopped, err=%v", h.Err())
	close(out)
}
