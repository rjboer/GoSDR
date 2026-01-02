package connectionmgr

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestIntegratedSDRTEST(test *testing.T) {

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	addr := "192.168.3.1:30431"

	m := &Manager{
		Address: addr,
		Timeout: 5 * time.Second,
	}

	// ------------------------------------------------------------
	// 1) Connect
	// ------------------------------------------------------------
	log.Printf("[TEST] Connecting to Pluto at %s", addr)
	m.Connect()
	defer m.Close()

	m.clientID = 0x01 // any non-zero client ID is fine

	// ------------------------------------------------------------
	// 2) Switch to binary mode (ASCII bootstrap)
	// We dont switch to binary mode here, because we are already in binary mode
	//binary mode is implied by the use of the binary header.
	// it is therefor not needed to switch to binary mode using the ASCII protocol.
	// ------------------------------------------------------------
	// log.Printf("[TEST] Switching to binary mode")
	// if _, err := m.conn.Write([]byte("BINARY\r\n")); err != nil {
	// 	log.Fatalf("[TEST] failed to switch to binary mode: %v", err)
	// }
	// m.PrimeASCII()
	var err error
	log.Println("-------------------------STEP1-------------------------")
	m.ClientInfo.Version, err = m.GetVersionASCII()
	if err != nil {
		log.Fatalf("[TEST] GetVersionASCII failed: %v", err)
	}
	log.Printf("[TEST] Version received: %s", m.ClientInfo.Version)

	log.Println("-------------------------STEP2-------------------------")
	var help string
	help, err = m.HelpASCII()
	if err != nil {
		log.Fatalf("[TEST] HelpASCII failed: %v", err)
	}
	log.Printf("[TEST] Help menu: %s", help)
	log.Println("-------------------------STEP3-------------------------")
	m.PrintASCII() //PRINT\n function
	log.Println("-------------------------STEP4-------------------------")
	//m.SwitchToBinary()
	m.EnterBinaryMode3()
	log.Println("-------------------------STEP5-------------------------")

	var data []byte
	data, err = m.Print(0)
	if err != nil {
		log.Fatalf("[TEST] Print failedd: %v", err)
	}
	log.Printf("[TEST] Print received (%d bytes), data:%v", len(data), data)
	log.Println("-------------------------STEP6-------------------------")
	// ------------------------------------------------------------
	// 3) PrimeCTX function
	// ------------------------------------------------------------
	data, err = m.PrimeCTX(0)
	if err != nil {
		log.Fatalf("[TEST] PrimeCTX failedd: %v", err)
	}
	log.Printf("[TEST] PrimeCTX received (%d bytes), data:%v", len(data), data)
	log.Println("-------------------------STEP7-------------------------")
	return
	// ------------------------------------------------------------
	// 3) Retrieve XML using your existing wrapper
	// ------------------------------------------------------------
	log.Printf("[TEST] Retrieving XML")
	xml, err := m.GetXML(0)
	if err != nil {
		log.Fatalf("[TEST] GetXML failed: %v", err)
	}
	log.Printf("[TEST] XML received (%d bytes)", len(xml))

	if err := os.WriteFile("./pluto.xml", xml, 0644); err != nil {
		log.Printf("[TEST] WARNING: failed to write pluto.xml: %v", err)
	} else {
		log.Printf("[TEST] wrote pluto.xml")
	}
	return

	// ------------------------------------------------------------
	// 4) Read some device attributes (direct base-function usage)
	// ------------------------------------------------------------
	readDevAttr := func(name string) (string, error) {
		hdr, plan, err := m.sendBinaryCommand(opReadAttr, 0, 0, lpString(name))
		if err != nil {
			return "", err
		}
		if hdr == nil || hdr.Opcode != opResponse {
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
		hdr, plan, err := m.sendBinaryCommand(opReadChnAttr, 0, chIdx, lpString(name))
		if err != nil {
			return "", err
		}
		if hdr == nil || hdr.Opcode != opResponse {
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
