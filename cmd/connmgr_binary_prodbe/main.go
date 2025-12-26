package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	man "github.com/RJBOER/GoSDR/internal/connectionmgr"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	addr := "192.168.2.1:30431"

	m := &man.Manager{
		Address: addr,
		Timeout: 5 * time.Second,
	}

	// ------------------------------------------------------------
	// 1) Connect
	// ------------------------------------------------------------
	log.Printf("[TEST] Connecting to Pluto at %s", addr)
	m.Connect()
	defer m.Close()

	m.EnterBinaryMode3()
	m.clientID = 0x1234 // any non-zero client ID is fine

	// ------------------------------------------------------------
	// 2) Switch to binary mode (ASCII bootstrap)
	// ------------------------------------------------------------
	log.Printf("[TEST] Switching to binary mode")
	if _, err := m.conn.Write([]byte("BINARY\r\n")); err != nil {
		log.Fatalf("[TEST] failed to switch to binary mode: %v", err)
	}

	// ------------------------------------------------------------
	// 3) Retrieve XML using your existing wrapper
	// ------------------------------------------------------------
	log.Printf("[TEST] Retrieving XML")
	xml, err := m.GetXML(0)
	if err != nil {
		log.Fatalf("[TEST] GetXML failed: %v", err)
	}
	log.Printf("[TEST] XML received (%d bytes)", len(xml))

	if err := os.WriteFile("pluto.xml", xml, 0644); err != nil {
		log.Printf("[TEST] WARNING: failed to write pluto.xml: %v", err)
	} else {
		log.Printf("[TEST] wrote pluto.xml")
	}

	// ------------------------------------------------------------
	// 4) Read some device attributes (direct base-function usage)
	// ------------------------------------------------------------
	readDevAttr := func(name string) (string, error) {
		hdr, plan, err := m.sendBinaryCommand(man.opReadAttr, 0, 0, man.lpString(name))
		if err != nil {
			return "", err
		}
		if hdr == nil || hdr.Opcode != man.opResponse {
			return "", fmt.Errorf("readDevAttr(%s): unexpected response opcode", name)
		}

		status, _, data, err := m.readResponse(plan)
		if err != nil {
			return "", err
		}
		if status != 0 {
			return "", fmt.Errorf("readDevAttr(%s): status=%d", name, status)
		}
		return strings.TrimSpace(string(data)), nil
	}

	log.Printf("[TEST] Reading common device attributes")
	for _, attr := range []string{
		"sampling_frequency",
		"rf_bandwidth",
		"gain_control_mode",
		"hardwaregain",
		"temperature",
	} {
		v, err := readDevAttr(attr)
		if err != nil {
			log.Printf("[TEST] %s: %v", attr, err)
			continue
		}
		log.Printf("[TEST] %s = %q", attr, v)
	}

	// ------------------------------------------------------------
	// 5) Read channel-indexed attributes (code = channel index)
	// ------------------------------------------------------------
	readChnAttr := func(chIdx int32, name string) (string, error) {
		hdr, plan, err := m.sendBinaryCommand(man.opReadChnAttr, 0, chIdx, man.lpString(name))
		if err != nil {
			return "", err
		}
		if hdr == nil || hdr.Opcode != man.opResponse {
			return "", fmt.Errorf("readChnAttr(%d,%s): unexpected response opcode", chIdx, name)
		}

		status, _, data, err := m.readResponse(plan)
		if err != nil {
			return "", err
		}
		if status != 0 {
			return "", fmt.Errorf("readChnAttr(%d,%s): status=%d", chIdx, name, status)
		}
		return strings.TrimSpace(string(data)), nil
	}

	log.Printf("[TEST] Reading channel attributes (hardwaregain)")
	for _, ch := range []int32{0, 1} {
		v, err := readChnAttr(ch, "hardwaregain")
		if err != nil {
			log.Printf("[TEST] ch=%d hardwaregain: %v", ch, err)
			continue
		}
		log.Printf("[TEST] ch=%d hardwaregain=%q", ch, v)

		if f, err := strconv.ParseFloat(v, 64); err == nil {
			log.Printf("[TEST] ch=%d hardwaregain parsed=%.2f", ch, f)
		}
	}

	// ------------------------------------------------------------
	// 6) Optional: write attribute (commented on purpose)
	// ------------------------------------------------------------
	/*
		log.Printf("[TEST] Setting gain_control_mode=manual")
		hdr, plan, err := m.sendBinaryCommand(
			opWriteAttr,
			0,
			0,
			nameValue("gain_control_mode", "manual"),
		)
		if err != nil {
			log.Printf("[TEST] write failed: %v", err)
		} else if hdr == nil || hdr.Opcode != opResponse {
			log.Printf("[TEST] unexpected response opcode on write")
		} else {
			status, _, _, err := m.readResponse(plan)
			if err != nil {
				log.Printf("[TEST] write response error: %v", err)
			} else {
				log.Printf("[TEST] write status=%d", status)
			}
		}
	*/

	log.Printf("[TEST] Pluto binary protocol test completed successfully")

}
