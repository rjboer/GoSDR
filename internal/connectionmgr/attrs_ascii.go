package connectionmgr

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// ReadDeviceAttrASCII reads a device attribute using legacy ASCII.
// Equivalent format to libiio: "READ %s %s\r\n". :contentReference[oaicite:7]{index=7}
func (m *Manager) ReadDeviceAttrASCII(devID, attr string) (string, error) {
	if m == nil || m.conn == nil {
		return "", errors.New("not connected")
	}
	if devID == "" || attr == "" {
		return "", errors.New("devID and attr are required")
	}

	cmd := fmt.Sprintf("READ %s %s", devID, attr)
	log.Printf("[attr][READ][dev] -> %q", cmd)

	n, err := m.ExecASCII(cmd)
	if err != nil {
		return "", err
	}
	// ExecASCII is expected to return the payload line (without trailing newline)
	// or provide a method to read the following bytes. If your ExecASCII currently
	// returns only integer status, switch to ExecASCIIReadLine below.
	_ = n

	// If you already have a "readLine" helper in ascii.go, use it here.
	line, err := m.readLine()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ReadChannelAttrASCII reads a channel attribute (INPUT/OUTPUT) using legacy ASCII.
// Equivalent to libiio: "READ %s INPUT|OUTPUT %s %s\r\n". :contentReference[oaicite:8]{index=8}
func (m *Manager) ReadChannelAttrASCII(devID string, isOutput bool, chanID, attr string) (string, error) {
	if m == nil || m.conn == nil {
		return "", errors.New("not connected")
	}
	if devID == "" || chanID == "" || attr == "" {
		return "", errors.New("devID, chanID, attr are required")
	}

	dir := "INPUT"
	if isOutput {
		dir = "OUTPUT"
	}

	cmd := fmt.Sprintf("READ %s %s %s %s", devID, dir, chanID, attr)
	log.Printf("[attr][READ][chn] -> %q", cmd)

	n, err := m.ExecASCII(cmd)
	if err != nil {
		return "", err
	}
	_ = n

	line, err := m.readLine()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// WriteDeviceAttrASCII writes a device attribute using legacy ASCII.
// Equivalent to libiio: "WRITE %s %s %lu\r\n" + bytes, then read integer response. :contentReference[oaicite:9]{index=9}
func (m *Manager) WriteDeviceAttrASCII(devID, attr, value string) (int, error) {
	if m == nil || m.conn == nil {
		return 0, errors.New("not connected")
	}
	if devID == "" || attr == "" {
		return 0, errors.New("devID and attr are required")
	}
	// libiio writes raw bytes without appending newline; keep that behavior.
	payload := []byte(value)
	cmd := fmt.Sprintf("WRITE %s %s %d", devID, attr, len(payload))
	log.Printf("[attr][WRITE][dev] -> %q (len=%d) payload=%q", cmd, len(payload), value)

	// Send command line (CRLF) + payload bytes.
	if err := m.writeLine(cmd); err != nil {
		return 0, err
	}
	if err := m.writeAll(payload); err != nil {
		return 0, err
	}

	// Read integer response (0 or negative errno).
	resp, err := m.readInteger()
	if err != nil {
		return 0, err
	}
	return resp, nil
}

// WriteChannelAttrASCII writes a channel attribute (INPUT/OUTPUT) using legacy ASCII.
// Equivalent to libiio: "WRITE %s INPUT|OUTPUT %s %s %lu\r\n" + bytes, then read integer. :contentReference[oaicite:10]{index=10}
func (m *Manager) WriteChannelAttrASCII(devID string, isOutput bool, chanID, attr, value string) (int, error) {
	if m == nil || m.conn == nil {
		return 0, errors.New("not connected")
	}
	if devID == "" || chanID == "" || attr == "" {
		return 0, errors.New("devID, chanID, attr are required")
	}

	dir := "INPUT"
	if isOutput {
		dir = "OUTPUT"
	}

	payload := []byte(value)
	cmd := fmt.Sprintf("WRITE %s %s %s %s %d", devID, dir, chanID, attr, len(payload))
	log.Printf("[attr][WRITE][chn] -> %q (len=%d) payload=%q", cmd, len(payload), value)

	if err := m.writeLine(cmd); err != nil {
		return 0, err
	}
	if err := m.writeAll(payload); err != nil {
		return 0, err
	}

	resp, err := m.readInteger()
	if err != nil {
		return 0, err
	}
	return resp, nil
}

//
// Convenience helpers (Step 3)
//

// SetLOFrequencyHzASCII sets LO frequency (you must pass the correct device/channel/attr for your Pluto XML).
// Example commonly used on AD9361: dev="ad9361-phy", isOutput=false, chan="altvoltage0", attr="frequency".
func (m *Manager) SetLOFrequencyHzASCII(devID string, isOutput bool, chanID string, hz int64) error {
	resp, err := m.WriteChannelAttrASCII(devID, isOutput, chanID, "frequency", strconv.FormatInt(hz, 10))
	if err != nil {
		return err
	}
	if resp != 0 {
		return fmt.Errorf("LO frequency write returned %d", resp)
	}
	return nil
}

// SetSampleRateHzASCII sets sample rate on the device (attr name varies; on Pluto often "sampling_frequency").
// For AD9361, this is usually on channels like "voltage0" input: attr "sampling_frequency".
func (m *Manager) SetSampleRateHzASCII(devID string, isOutput bool, chanID string, hz int64) error {
	resp, err := m.WriteChannelAttrASCII(devID, isOutput, chanID, "sampling_frequency", strconv.FormatInt(hz, 10))
	if err != nil {
		return err
	}
	if resp != 0 {
		return fmt.Errorf("sample rate write returned %d", resp)
	}
	return nil
}

// SetHardwareGainDBASCII sets hardware gain in dB (attr name often "hardwaregain").
// This is typically on output channels for TX or input channels for RX depending on device.
func (m *Manager) SetHardwareGainDBASCII(devID string, isOutput bool, chanID string, gainDB float64) error {
	// Many drivers accept float strings; keep plain formatting.
	val := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", gainDB), "0"), ".")
	resp, err := m.WriteChannelAttrASCII(devID, isOutput, chanID, "hardwaregain", val)
	if err != nil {
		return err
	}
	if resp != 0 {
		return fmt.Errorf("hardwaregain write returned %d", resp)
	}
	return nil
}

// SetChannelEnabledASCII toggles a channel enable attribute.
// The exact attribute can differ by backend; common patterns include "en" or "enabled" or scan_elements "<chan>_en".
// This is a best-effort helper: pass attrName explicitly.
func (m *Manager) SetChannelEnabledASCII(devID string, isOutput bool, chanID, attrName string, enabled bool) error {
	v := "0"
	if enabled {
		v = "1"
	}
	resp, err := m.WriteChannelAttrASCII(devID, isOutput, chanID, attrName, v)
	if err != nil {
		return err
	}
	if resp != 0 {
		return fmt.Errorf("channel enable write returned %d", resp)
	}
	return nil
}

//
// The following helpers are expected to exist in your ascii.go.
// If you don't have them, implement them there (NOT duplicated elsewhere):
//
// - func (m *Manager) ExecASCII(cmd string) (int, error)          // write cmd + CRLF, read integer header
// - func (m *Manager) readInteger() (int, error)                 // reads one integer line
// - func (m *Manager) readLine() (string, error)                 // reads one line (ending in \n)
// - func (m *Manager) writeLine(cmd string) error                // writes cmd + "\r\n"
// - func (m *Manager) writeAll(b []byte) error                   // writes all bytes
//
