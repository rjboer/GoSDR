package connectionmgr

import (
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ReadDeviceAttrASCII sends a legacy ASCII READ for a device-level attribute.
//
// Parameters:
//   - devID: device identifier string (for example "ad9361-phy").
//   - attr: attribute name to read.
//
// Protocol:
//   - issues "READ <devID> <attr>\r\n" and expects the next line to contain the
//     attribute value.
//
// Returns the trimmed attribute string or an error if the manager is not
// connected, parameters are missing, or the server returns an invalid payload.
func (m *Manager) ReadDeviceAttrASCII(devID, attr string) (string, error) {
	if m == nil || m.conn == nil {
		return "", errors.New("not connected")
	}
	if devID == "" || attr == "" {
		return "", errors.New("devID and attr are required")
	}

	cmd := fmt.Sprintf("READ %s %s", devID, attr)
	log.Printf("[attr][READ][dev] -> %q", cmd)

	length, err := m.ExecASCII(cmd)
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("READ returned negative length %d", length)
	}

	payloadLen := length + 1 // account for trailing '\n'
	line, err := m.readLine(payloadLen, true)
	if err != nil {
		return "", fmt.Errorf("READ payload read failed: %w", err)
	}
	if len(line) != payloadLen {
		return "", fmt.Errorf("READ payload truncated: expected %d bytes, got %d", payloadLen, len(line))
	}

	return strings.TrimRight(string(line), "\r\n"), nil
}

// ReadChannelAttrASCII reads a channel attribute through the ASCII protocol.
//
// Parameters:
//   - devID: device identifier string.
//   - isOutput: whether the channel direction is OUTPUT (INPUT otherwise).
//   - chanID: channel identifier (for example "voltage0").
//   - attr: attribute name to read.
//
// Protocol:
//   - issues "READ <devID> INPUT|OUTPUT <chanID> <attr>\r\n" and reads one
//     payload line containing the value.
//
// Returns the trimmed attribute string or an error if validation, write, or
// read fails.
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

	length, err := m.ExecASCII(cmd)
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("READ returned negative length %d", length)
	}

	payloadLen := length + 1 // account for trailing '\n'
	line, err := m.readLine(payloadLen, true)
	if err != nil {
		return "", fmt.Errorf("READ payload read failed: %w", err)
	}
	if len(line) != payloadLen {
		return "", fmt.Errorf("READ payload truncated: expected %d bytes, got %d", payloadLen, len(line))
	}
	return strings.TrimRight(string(line), "\r\n"), nil
}

// ReadBufferAttrASCII reads a buffer attribute through the ASCII protocol.
//
// Parameters:
//   - devID: device identifier string.
//   - attr: buffer attribute name to read.
//
// Protocol:
//   - issues "READ <devID> BUFFER <attr>\r\n" and expects the next line to
//     contain the attribute value.
//
// Returns the trimmed attribute string or an error if validation, write, or
// read fails.
func (m *Manager) ReadBufferAttrASCII(devID, attr string) (string, error) {
	if m == nil || m.conn == nil {
		return "", errors.New("not connected")
	}
	if devID == "" || attr == "" {
		return "", errors.New("devID and attr are required")
	}

	cmd := fmt.Sprintf("READ %s BUFFER %s", devID, attr)
	log.Printf("[attr][READ][buf] -> %q", cmd)

	length, err := m.ExecASCII(cmd)
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("READ returned negative length %d", length)
	}

	payloadLen := length + 1 // account for trailing '\n'
	line, err := m.readLine(payloadLen, true)
	if err != nil {
		return "", fmt.Errorf("READ payload read failed: %w", err)
	}
	if len(line) != payloadLen {
		return "", fmt.Errorf("READ payload truncated: expected %d bytes, got %d", payloadLen, len(line))
	}

	value := strings.TrimRight(string(line), "\r\n")
	value = strings.Trim(value, "\x00")
	return value, nil
}

// ReadChannelAttrASCII2 mirrors ReadChannelAttrASCII but also returns the raw
// status code. This helper is retained for callers that need to differentiate
// transport errors from device-side errno returns until they migrate to the
// newer API.
func (m *Manager) ReadChannelAttrASCII2(
	dev string,
	isOutput bool,
	channel string,
	attr string,
) (string, int, error) {

	if m == nil {
		return "", 0, fmt.Errorf("nil Manager")
	}
	if m.conn == nil {
		return "", 0, fmt.Errorf("ReadChannelAttrASCII: not connected")
	}
	if m.Mode != ModeASCII {
		return "", 0, fmt.Errorf("ReadChannelAttrASCII: not in ASCII mode")
	}

	dir := "INPUT"
	if isOutput {
		dir = "OUTPUT"
	}

	cmd := fmt.Sprintf("READ %s %s %s %s", dev, dir, channel, attr)
	m.logf("[attr][READ][chn] -> %q", cmd)

	// --- Send command ---
	if err := m.writeAll([]byte(cmd + "\n")); err != nil {
		return "", 0, fmt.Errorf("READ write failed: %w", err)
	}

	// --- Read status line ---
	line, err := m.readLine(64, false)
	if err != nil {
		return "", 0, fmt.Errorf("READ status read failed: %w", err)
	}

	// IMPORTANT: strip whitespace AND NULs
	statusStr := strings.TrimSpace(string(line))
	statusStr = strings.Trim(statusStr, "\x00")

	if statusStr == "" {
		return "", 0, fmt.Errorf("READ returned empty status line")
	}

	status, err := strconv.Atoi(statusStr)
	if err != nil {
		return "", 0, fmt.Errorf("READ invalid status %q: %w", statusStr, err)
	}

	// --- Non-zero status: STOP HERE ---
	if status != 0 {
		m.logf("[attr][READ][RC=%d] %s/%s/%s/%s", status, dev, dir, channel, attr)
		return "", status, nil
	}

	// --- Read exactly ONE payload line ---
	valLine, err := m.readLine(4096, true)
	if err != nil {
		return "", status, fmt.Errorf("READ value read failed: %w", err)
	}

	value := strings.TrimRight(string(valLine), "\r\n")
	value = strings.Trim(value, "\x00")

	m.logf("[attr][READ][OK] %s/%s/%s/%s = %q", dev, dir, channel, attr, value)

	return value, status, nil
}

// WriteDeviceAttrASCII writes a device attribute using the ASCII protocol.
//
// Parameters:
//   - devID: device identifier string.
//   - attr: attribute name to write.
//   - value: raw payload written without an automatic newline.
//
// Protocol:
//   - issues "WRITE <devID> <attr> <len>\r\n" followed by <len> raw bytes.
//   - reads the following integer status (0 for success, negative errno on
//     failure).
//
// Returns the integer status or an error if validation or socket IO fails.
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

// WriteChannelAttrASCII writes a channel attribute (INPUT or OUTPUT) via ASCII
// WRITE.
//
// Parameters mirror WriteDeviceAttrASCII but include the channel identifier and
// direction flag.
//
// Protocol:
//   - issues "WRITE <devID> INPUT|OUTPUT <chanID> <attr> <len>\r\n" then sends
//     <len> raw bytes, and finally reads the integer status line.
//
// Returns the integer status or an error if validation or socket IO fails.
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

// SetLOFrequencyHzASCII writes the LO frequency attribute using ASCII WRITE.
//
// The caller supplies the exact device/channel/attribute triplet expected by
// the target XML (for example dev="ad9361-phy", chan="altvoltage0",
// attr="frequency"). A non-zero response status is surfaced as an error so
// callers see device-side errno codes.
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

// SetSampleRateHzASCII writes the sampling rate via ASCII WRITE. Attribute names
// depend on the backend (commonly "sampling_frequency"). Non-zero device
// responses are returned as errors.
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

// SetHardwareGainDBASCII writes a gain value using ASCII WRITE. The attribute is
// often named "hardwaregain" and may exist on RX or TX channels depending on
// the device. The helper formats the float without trailing zeros and reports
// non-zero statuses as errors.
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

// SetTimeoutASCII configures the server-side socket timeout using the ASCII
// TIMEOUT command.
//
// Protocol:
//   - issues "TIMEOUT <timeoutMs>\r\n" and reads the following integer status.
//   - a zero or positive status indicates success; negative values mirror errno
//     codes and are treated as errors.
//
// Parameters:
//   - timeoutMs: timeout in milliseconds. Negative values are rejected before
//     issuing the command.
//
// Returns nil on success or an error for validation failures, transport errors,
// or negative device statuses.
func (m *Manager) SetTimeoutASCII(timeoutMs int) error {
	if timeoutMs < 0 {
		return fmt.Errorf("timeoutMs must be >= 0")
	}

	status, err := m.ExecASCII(fmt.Sprintf("TIMEOUT %d", timeoutMs))
	if err != nil {
		return fmt.Errorf("TIMEOUT command failed: %w", err)
	}
	if status < 0 {
		return fmt.Errorf("TIMEOUT command returned %d", status)
	}
	return nil
}

// GetTriggerASCII fetches the active trigger name for a device using the ASCII
// GETTRIG command.
//
// Protocol:
//   - issues "GETTRIG <deviceID>\r\n".
//   - reads the subsequent integer length using readInteger().
//   - reads exactly length+1 bytes (payload plus trailing newline) using
//     readLine, trimming trailing whitespace from the result.
//
// Returns the trimmed trigger name or an error for validation failures,
// negative lengths, or IO errors.
func (m *Manager) GetTriggerASCII(deviceID string) (string, error) {
	if m == nil || m.conn == nil {
		return "", fmt.Errorf("not connected")
	}
	if deviceID == "" {
		return "", fmt.Errorf("deviceID is required")
	}

	if err := m.writeLine(fmt.Sprintf("GETTRIG %s", deviceID)); err != nil {
		return "", err
	}

	length, err := m.readInteger()
	if err != nil {
		return "", fmt.Errorf("GETTRIG length read failed: %w", err)
	}
	if length < 0 {
		return "", fmt.Errorf("GETTRIG returned negative length %d", length)
	}

	line, err := m.readLine(length+1, true)
	if err != nil {
		return "", fmt.Errorf("GETTRIG payload read failed: %w", err)
	}

	value := strings.TrimRightFunc(string(line), unicode.IsSpace)
	value = strings.TrimRight(value, "\x00")
	return value, nil
}

// SetTriggerASCII selects a trigger using the ASCII SETTRIG command. An empty
// triggerName clears the trigger configuration when supported by the device.
// Negative device responses are returned as errors.
func (m *Manager) SetTriggerASCII(deviceID, triggerName string) error {
	if m == nil || m.conn == nil {
		return fmt.Errorf("not connected")
	}
	if deviceID == "" {
		return fmt.Errorf("deviceID is required")
	}

	cmd := fmt.Sprintf("SETTRIG %s", deviceID)
	if triggerName != "" {
		cmd = fmt.Sprintf("%s %s", cmd, triggerName)
	}

	status, err := m.ExecASCII(cmd)
	if err != nil {
		return fmt.Errorf("SETTRIG command failed: %w", err)
	}
	if status < 0 {
		return fmt.Errorf("SETTRIG command returned %d", status)
	}

	return nil
}

// SetChannelEnabledASCII toggles a channel-enable style attribute via ASCII
// WRITE. The caller specifies attrName explicitly because attribute naming
// conventions vary (for example "en", "enabled", or scan_elements
// "<chan>_en"). Non-zero statuses are reported as errors.
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

// The following helpers are expected to exist in your ascii.go.
// If you don't have them, implement them there (NOT duplicated elsewhere):
//
// - func (m *Manager) ExecASCII(cmd string) (int, error)          // write cmd + CRLF, read integer header
// - func (m *Manager) readInteger() (int, error)                 // reads one integer line
// - func (m *Manager) readLine(maxLen int) ([]byte, error)       // reads one line (ending in \n)
// - func (m *Manager) writeLine(cmd string) error                // writes cmd + "\r\n"
// - func (m *Manager) writeAll(b []byte) error                   // writes all bytes
// DrainASCII consumes any pending bytes from the ASCII connection using a short
// read deadline. It is intended to realign the stream after a protocol error.
// The method returns nil on timeout (meaning drained) or propagates socket
// errors otherwise.
func (m *Manager) DrainASCII() error {
	buf := make([]byte, 1)
	_ = m.conn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	for {
		_, err := m.conn.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				fmt.Println("buffer:", buf)
				fmt.Println("drained buffer")
				return nil // drained
			}
			return err
		}
	}
}

// HelpfunctionASCII sends the HELP command and drains the multi-line response
// from the ASCII server. It is primarily for debugging or manual discovery and
// returns the first socket error encountered.
func (m *Manager) HelpfunctionASCII() error {

	log.Println("HelpfunctionASCII function")
	data := []byte("HELP\n")
	log.Printf("Sending \nBytes:%b \nData:%b \nText:%q\n", len(data), data, string(data))
	_, err := m.conn.Write(data)
	if err != nil {
		return err
	}
	m.readLine(512*1024, true)
	log.Println("HelpfunctionASCII: function done")
	return err
}

// VersionASCII issues the VERSION command over ASCII and drains the response
// body. The method is used during bootstrap to detect server capabilities and
// returns any write/read error encountered.
func (m *Manager) VersionASCII() error {

	log.Println("VersionASCII function")
	data := []byte("VERSION\n")
	log.Printf("Sending \nBytes:%b \nData:%b \nText:%q\n", len(data), data, string(data))
	_, err := m.conn.Write(data)
	if err != nil {
		return err
	}
	m.readLine(512*1024, true)
	log.Println("VersionASCII function done")
	return err
}

// ZPrintASCII sends the ZPRINT command (extended PRINT variant) and drains the
// response stream. Errors from socket writes or reads are returned to the
// caller.
func (m *Manager) ZPrintASCII() error {

	log.Println("ZPrintASCII function")
	data := []byte("ZPRINT\n")
	log.Printf("Sending \nBytes:%b \nData:%b \nText:%q\n", len(data), data, string(data))
	_, err := m.conn.Write(data)
	if err != nil {
		return err
	}
	m.readLine(512*1024, false)
	log.Println("ZPrintASCII function done")
	return err
}

// PrintASCII issues the PRINT command to fetch the ASCII device inventory and
// drains the response. It logs the number of bytes consumed and returns any
// socket error encountered.
func (m *Manager) PrintASCII() error {

	log.Println("PRINT function")
	data := []byte("PRINT\n")
	log.Printf("Sending \nBytes:%b \nData:%b \nText:%q\n", len(data), data, string(data))
	_, err := m.conn.Write(data)
	if err != nil {
		return err
	}
	log.Println("Print:Draining read buffer")
	data, err = m.readLine(512*1024, false)
	fmt.Println("Print:number of bytes read:", len(data))
	if err != nil {
		fmt.Println("Print:Error reading data:", err)
	}
	fmt.Println("Print:Drained read buffer, Done")
	return err
}

// SwitchToBinary attempts the ASCII "BINARY" mode switch and drains the
// response stream. Modern servers start in ASCII mode automatically; callers
// should transition to binary helpers after this call succeeds. Any socket
// error is returned.
func (m *Manager) SwitchToBinary() error {

	log.Println("SwitchToBinary function")

	data := []byte("BINARY\n")
	log.Printf("Sending \nBytes:%b \nData:%b \nText:%q\n", len(data), data, string(data))
	_, err := m.conn.Write(data)
	if err != nil {
		return err
	}
	log.Println("SwitchToBinary:Draining read buffer")
	m.readLine(1024*512, true)
	log.Println("SwitchToBinary:Drained read buffer, Done")
	return err
}
