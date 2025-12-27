# Implement ASCII Protocol Support (Fallback)

As the hardware (PlutoSDR) reports syntax errors for the `BINARY` command, we will implement the full set of IIOD operations using the default ASCII protocol. This ensures compatibility while providing access to all hardware features.

## Proposed Changes

### [connectionmgr]

# Implement ASCII Protocol Support (Fallback)

As the hardware (PlutoSDR) reports syntax errors for the `BINARY` command, we will implement the full set of IIOD operations using the default ASCII protocol. This ensures compatibility while providing access to all hardware features.

## Proposed Changes

### [connectionmgr]

#### Naming Conventions
- **Device ID**: Must use the `iio:deviceX` format (e.g., `iio:device0`) or the vendor name (e.g., `ad9361-phy`) as found in the XML context.
- **Attributes**: Case-sensitive strings matching the XML [name](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/iiod/iiod/ops.c#140-154) field (e.g., `sampling_frequency`).

# Comprehensive ASCII Protocol Implementation Plan

This plan ensures 100% coverage of the IIOD ASCII protocol as defined in [parser.y](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/iiod/iiod/parser.y).

## 1. Command Coverage Matrix

| ASCII Command Pattern | Go Method (Existing/Planned) | Status |
| :--- | :--- | :--- |
| `HELP` | `HelpASCII()` | **Verify** |
| `VERSION` | `GetVersionASCII()` | **Verify** |
| `PRINT` | `GetContextXMLASCII()` | **Refactor** (Check trailing newline) |
| `ZPRINT` | `GetContextXMLCompressedASCII()` | **Verify** |
| `TIMEOUT <ms>` | `SetTimeoutASCII(ms)` | **Missing** |
| `OPEN <dev> <samples> <mask> [CYCLIC]` | [OpenBufferASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#8-38) | **Verify** |
| `CLOSE <dev>` | [CloseBufferASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#124-141) | **Verify** |
| `READBUF <dev> <bytes>` | [ReadBufferASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#39-103) | **Fix** (Handle Mask) |
| `WRITEBUF <dev> <bytes>` | `WriteBufferASCII(...)` | **Missing** |
| `READ <dev> <attr>` | [ReadDeviceAttrASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#13-42) | **Verify** |
| `READ <dev> DEBUG <attr>` | `ReadDebugAttrASCII(...)` | **Missing** |
| `READ <dev> BUFFER <attr>` | `ReadBufferAttrASCII(...)` | **Missing** |
| `READ <dev> <dir> <chan> <attr>` | [ReadChannelAttrASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#43-73) | **Verify** |
| `WRITE <dev> <attr> <len>` | [WriteDeviceAttrASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#144-173) | **Verify** |
| `WRITE <dev> DEBUG <attr> <len>` | `WriteDebugAttrASCII(...)` | **Missing** |
| `WRITE <dev> BUFFER <attr> <len>` | `WriteBufferAttrASCII(...)` | **Missing** |
| `WRITE <dev> <dir> <chan> <attr> <len>` | [WriteChannelAttrASCII(...)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#174-206) | **Verify** |
| `GETTRIG <dev>` | `GetTriggerASCII(...)` | **Missing** |
| `SETTRIG <dev> [<trig>]` | `SetTriggerASCII(...)` | **Missing** |
| `SET <dev> BUFFERS_COUNT <count>` | `SetKernelBuffersCountASCII(...)` | **Missing** |

## 2. Detailed Implementation Steps

The following methods will be implemented on the `*Manager` struct in [attrs_ascii.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go) (and [buffer_ascii.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go) for streaming).

### A. Context & Session Management

#### `func (m *Manager) SetTimeoutASCII(timeoutMs int) error`
- **Logic**:
  1. Construct command: `cmd := fmt.Sprintf("TIMEOUT %d", timeoutMs)`
  2. Execute: `ret, err := m.ExecASCII(cmd)`
  3. Validate: If `ret < 0` return formatted error `fmt.Errorf("TIMEOUT failed: %d", ret)`.

#### `func (m *Manager) GetTriggerASCII(deviceID string) (string, error)`
- **Logic**:
  1. Send: `m.writeLine(fmt.Sprintf("GETTRIG %s", deviceID))`
  2. Read Length: `n, err := m.readInteger()`
  3. Error Check: If `n < 0` return "", error.
  4. Read Value: `line, err := m.readLine(n, true)`
  5. Return: `strings.TrimSpace(string(line))`, nil.

#### `func (m *Manager) SetTriggerASCII(deviceID, triggerName string) error`
- **Logic**:
  1. Construct: `cmd := fmt.Sprintf("SETTRIG %s %s", deviceID, triggerName)` 
     - If `triggerName` is empty, just `SETTRIG %s`.
  2. Execute: `ret, err := m.ExecASCII(cmd)`
  3. Validate: Return error if `ret < 0`.

### B. Attribute Management (Missing Methods)

#### `func (m *Manager) ReadDebugAttrASCII(devID, attr string) (string, error)`
- **Logic**: Behave exactly like [ReadDeviceAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#13-42) but use `DEBUG_ATTR` token.
  1. Format: `READ %s DEBUG %s`
  2. Execute: [ExecASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#103-107) + [readLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#161-204) sequence.

#### `func (m *Manager) WriteDebugAttrASCII(devID, attr, value string) (int, error)`
- **Logic**: Behave exactly like [WriteDeviceAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#144-173) but use `DEBUG_ATTR` token.
  1. Format: `WRITE %s DEBUG %s %d` + payload.
  2. Execute: [writeLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#95-102) + [writeAll](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#129-146) + [readInteger](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#8-63).

#### `func (m *Manager) ReadBufferAttrASCII(devID, attr string) (string, error)`
- **Logic**: Uses `BUFFER_ATTR` token.
  1. Format: `READ %s BUFFER %s`
  2. Execute: [ExecASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#103-107) + [readLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#161-204).

#### `func (m *Manager) WriteBufferAttrASCII(devID, attr, value string) (int, error)`
- **Logic**: Uses `BUFFER_ATTR` token.
  1. Format: `WRITE %s BUFFER %s %d` + payload.
  2. Execute: [writeLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#95-102) + [writeAll](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#129-146) + [readInteger](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#8-63).

### C. Streaming (Buffer Operations)

#### `func (m *Manager) ReadBufferASCII(deviceID string, dst []byte) (int, error)`
**Status**: Critical Fix needed in [buffer_ascii.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go).
- **Logic**:
  1. Send: `m.writeLine(fmt.Sprintf("READBUF %s %d", deviceID, len(dst)))`
  2. Read Length: `n, err := m.readInteger()` (Actual bytes available).
  3. **CRITICAL STEP**: `m.readInteger()` (or [readLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#161-204)) to consume the **HEX MASK LINE**.
     - Only if `n > 0`.
  4. Read Payload: `m.readAll(dst[:n])`
  5. Sync: `m.conn.Read(oneByte)` to consume the trailing newline.

### D. Core I/O Optimizations

#### `func (m *Manager) readLine(maxLen int, output bool) ([]byte, error)`
- **Goal**: Optimize for speed by avoiding byte-by-byte loops.
- **Proposed Logic**:
  1. Use `bufio.Reader` (buffered I/O) wrapper around the connection if possible, OR:
  2. Read a large chunk (e.g., 4096 bytes) into a buffer using `m.conn.Read`.
  3. Use `bytes.IndexByte(buf, '\n')` to find the line terminator instantly (Assembly optimized).
  4. If found, return `buf[:index+1]` and save the remainder.
  5. The user explicitly requested: "1 check if the buffer ends with a linefeed."

## 3. Documentation Requirements
All new and refactored methods must include GoDoc comments specifying:
- **Purpose**: What the function does (e.g., "SetTimeoutASCII sets the I/O timeout").
- **Parameters**: Description of each argument.
- **Protocol**: The exact ASCII command sent (e.g., `TIMEOUT %d\n`).
- **Returns**: Meaning of return values and specific error conditions.

## 4. Detailed Task Breakdown (See task.md)
The [task.md](file:///C:/Users/Roelof%20Jan/.gemini/antigravity/brain/45256e63-e213-4691-8579-d4c150e3c2af/task.md) file will be updated to track the implementation of each method individually.

## Protocol Examples (The ASCII Handshake)

| Operation | Command Sequence | Expected Response |
| :--- | :--- | :--- |
| **Get Device Attr** | `READ iio:device0 sampling_frequency\n` | `length\n` followed by `value\n` |
| **Set Device Attr** | `WRITE iio:device0 sampling_frequency 8\n1000000\n` | `0\n` (or `-errno\n`) |
| **Get Channel Attr**| `READ iio:device0 INPUT voltage0 rf_bandwidth\n` | `length\n` followed by `value\n` |
| **Get Debug Attr** | `READ iio:device0 DEBUG direct_reg_access\n` | `length\n` followed by `value\n` |
| **Set Channel Attr**| `WRITE iio:device0 INPUT voltage0 rf_bandwidth 7\n5000000\n` | `0\n` |
| **Open Streaming** | `OPEN iio:device0 4096 0001\n` | `0\n` |
| **Read Buffer** | `READBUF iio:device0 4096\n` | `len\n` followed by `len` raw bytes + `\n` |

> [!IMPORTANT]
> **The Trailing Newline**: The IIOD server appends a `\n` to almost every response (even raw buffer reads). In Go, you MUST call [readLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#161-204) or [Read(1)](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#56-66) after consuming the expected payload to keep the socket "clean" for the next command.

## Verification Plan

### Automated Tests
- Create `ascii_params_test.go`:
  - Set various attributes (sampling frequency, gain).
  - Verify values are updated by reading them back.
- Create `ascii_streaming_test.go`:
  - Open a device.
  - Perform multiple [ReadBuf](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#39-103) calls and verify data length.
  - Close the device.

### Manual Verification
- Run the tests against the live PlutoSDR at `192.168.3.1:30431`.
- Monitor server-side output (via `iiod -v`) to confirm command strings are received correctly without syntax errors.
