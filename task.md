# Pluto SDR IIOD Protocol Compatibility Tasks

## Current Status

### ✅ Completed
- Basic IIOD client implementation ([iiod/connect.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go))
- XML context parsing and caching
- Device discovery via hardcoded fallback
- Legacy protocol detection (v0.25)
- Connection establishment to Pluto SDR
- Debug logging infrastructure

### ❌ Blocking Issue
**IIOD v0.25 Protocol Incompatibility**: Pluto SDR firmware v0.38 uses IIOD v0.25, which:
- Returns XML context for every command (non-standard)
- Doesn't support `LIST_DEVICES` command
- Doesn't support `WRITE` command for attributes (returns -22 EINVAL)
- Uses different response format than modern IIOD

**Current Error**: `set sample rate: iiod error -22 (EINVAL)` when trying to write attributes

---

## Phase 1: IIOD Protocol Version Compatibility

### 1.1 Protocol Version Detection
- [ ] Extract version from cached XML context (`version-major`, `version-minor`)
- [ ] Add `ProtocolVersion` struct to Client
- [ ] Implement version comparison methods (`IsLegacy()`, `SupportsWrite()`, etc.)
- [ ] Log detected protocol version on connection

### 1.2 Research IIOD v0.25 Protocol
- [x] Document v0.25 command format differences
- [x] Identify that IIOD uses BINARY protocol, not text
- [x] Discover WRITE command requires: header (8 bytes) + length (8 bytes) + data
- [x] Find opcode values (WRITE_ATTR = 7)
- [x] Analyze libiio v0.25 source code for reference implementation
- [x] Create comprehensive protocol documentation

### 1.3 Implement Binary Protocol Support
- [ ] Create `IIODCommand` struct for binary command headers
- [ ] Implement `Marshal()` method to serialize commands to 8 bytes
- [ ] Create `WriteAttrBinary()` method with binary protocol
- [ ] Create `ReadAttrBinary()` method with binary protocol
- [ ] Parse XML context to build device/attribute index maps
- [ ] Test binary protocol with Pluto SDR hardware

### 1.4 Cleanup Obsolete Text-Based Protocol Code

**Functions to DELETE** (text-based protocol, no longer needed):
- [ ] [Send()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#48-52) - Delete (replaced by binary command sending)
- [ ] [SendWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#53-61) - Delete (replaced by binary command sending)
- [ ] [sendBinaryWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#487-637) - **KEEP but rename to `sendBinaryCommand()`** (used for binary responses)

**Functions to MODIFY** (update to use binary protocol):
- [ ] [WriteAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#381-385) - Modify to use `WriteAttrBinary()` internally
- [ ] [WriteAttrWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#386-403) - Modify to use `WriteAttrBinary()` internally
- [ ] [ReadAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#359-363) - Modify to use `ReadAttrBinary()` internally
- [ ] [ReadAttrWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#364-380) - Modify to use `ReadAttrBinary()` internally

**Functions to KEEP** (still valid):
- [ ] [Dial()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#90-94) / [DialWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#95-114) - Keep (connection establishment)
- [ ] [Close()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/pluto.go#355-390) - Keep (connection cleanup)
- [ ] [GetXMLContextWithContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#226-230) - Keep (uses PRINT opcode, works as-is)
- [ ] [ListDevicesFromXML()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#231-263) - Keep (parses XML, not protocol-dependent)

**New Functions to ADD**:
- [ ] `buildDeviceIndexMap()` - Parse XML to map device names → indices
- [ ] `buildAttributeIndexMap()` - Parse XML to map attribute names → codes
- [ ] `sendCommand()` - Send binary command header
- [ ] `readResponse()` - Read binary response (int32 status code)

### 1.5 Update PlutoSDR Backend
- [ ] Replace [WriteAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#381-385) calls with `WriteAttrCompat()`
- [ ] Add error handling for unsupported operations
- [ ] Implement graceful degradation for missing features
- [ ] Test full initialization sequence

---

## Phase 2: Alternative Configuration Methods (If IIOD v0.25 Doesn't Support Writes)

### 2.1 SSH/Sysfs Configuration
- [ ] Implement SSH client for Pluto SDR
- [ ] Create sysfs attribute write helpers
- [ ] Map IIO attributes to sysfs paths
- [ ] Test configuration via SSH
- [ ] Add SSH fallback to PlutoSDR.Init()

### 2.2 Hybrid Approach
- [ ] Use IIOD for reading and buffer operations
- [ ] Use SSH/sysfs for configuration writes
- [ ] Implement seamless switching between methods
- [ ] Document hybrid architecture

---

## Phase 3: Buffer Operations (After Configuration Works)

### 3.1 IIOD Buffer Commands
- [ ] Implement `OPEN` command for buffer creation
- [ ] Implement `READBUF` for RX data streaming
- [ ] Implement `WRITEBUF` for TX data
- [ ] Implement `CLOSE` for buffer cleanup
- [ ] Test buffer lifecycle

### 3.2 PlutoSDR RX/TX Implementation
- [ ] Implement [RX()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/sdr.go#23-24) method with buffer reads
- [ ] Implement [TX()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/sdr.go#24-25) method with buffer writes
- [ ] Add IQ data parsing (12-bit to complex64)
- [ ] Test dual-channel streaming
- [ ] Verify phase coherence

---

## Phase 4: Testing & Validation

### 4.1 Protocol Compatibility Tests
- [ ] Create mock IIOD v0.25 server
- [ ] Create mock IIOD v0.26+ server
- [ ] Test version detection
- [ ] Test fallback mechanisms
- [ ] Test error handling

### 4.2 Hardware Integration Tests
- [ ] Test full initialization with Pluto SDR
- [ ] Test RX data acquisition
- [ ] Test TX tone generation
- [ ] Test configuration changes
- [ ] Verify tracking functionality

### 4.3 Documentation
- [ ] Document IIOD protocol versions
- [ ] Document compatibility layer design
- [ ] Add troubleshooting guide
- [ ] Create migration guide for firmware updates

---

## Phase 5: Optimization (Future)

### 5.1 Performance Enhancements
- [ ] Implement connection pooling
- [ ] Add streaming buffer optimizations
- [ ] Implement batch operations (if supported)
- [ ] Add metrics and monitoring

### 5.2 Advanced Features
- [ ] Auto-reconnect with state restoration
- [ ] Debug attribute access
- [ ] Trigger support
- [ ] Multi-device synchronization

---

## Next Steps (Priority Order)

1. **Research IIOD v0.25** - Understand what write operations are supported
2. **Implement Version Detection** - Parse version from XML and store in Client
3. **Test Alternative Write Methods** - Try different command formats
4. **Implement SSH Fallback** - If IIOD writes don't work, use SSH/sysfs
5. **Complete Initialization** - Get PlutoSDR.Init() working end-to-end
6. **Implement Buffers** - Add RX/TX streaming once config works

---

## Decision Points

### Should we upgrade Pluto firmware?
- **Pros**: Get modern IIOD v0.26+ with full protocol support
- **Cons**: May not be possible/desirable for user's setup
- **Decision**: Implement compatibility layer first, upgrade is optional

### Should we use SSH for configuration?
- **Pros**: Guaranteed to work, well-documented
- **Cons**: Requires SSH credentials, more complex
- **Decision**: Use as fallback if IIOD v0.25 writes don't work

### Should we use libiio C library via CGo?
- **Pros**: Handles all protocol versions automatically
- **Cons**: User wants pure Go solution
- **Decision**: NO - stick with pure Go implementation
