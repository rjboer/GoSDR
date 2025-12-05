# Pluto SDR Backend Implementation Tasks

## Phase 1: Extend IIO Client with Buffer Operations

### IIO Buffer Implementation
- [x] Create [iiod/buffer.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go) with Buffer struct
- [x] Implement [CreateStreamBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#21-69) method
- [x] Implement [EnableChannel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#70-78) for channel masking
- [x] Implement [ReadSamples()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#136-160) for binary data streaming
- [x] Implement [Close()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#181-191) for buffer cleanup
- [x] Add binary data parsing utilities (little-endian int16)

### IIO Protocol Commands
- [ ] Add [OpenBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#290-294) command to [iiod/connect.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go)
- [ ] Add [ReadBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#308-312) command for data streaming
- [ ] Add [WriteBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#325-329) command for TX data
- [ ] Add [CloseBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#344-348) command
- [ ] Handle binary protocol responses

### Testing
- [x] Create [iiod/buffer_test.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer_test.go)
- [x] Test buffer creation with mock server
- [x] Test channel enable/disable operations
- [x] Test binary data read/write
- [x] Test error handling and edge cases

---

## Phase 2: Implement Pluto SDR Backend

### PlutoSDR Structure
- [x] Define PlutoSDR struct in [internal/sdr/pluto.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/pluto.go)
- [x] Add IIO client connection handling
- [x] Add RX/TX buffer management
- [x] Add AD9361 device handle tracking

### Init() Implementation
- [x] Implement IIO client connection via Dial()
- [x] Add device discovery (ListDevices)
- [x] Configure sample rate attribute
- [x] Configure RX LO frequency
- [x] Configure RX gains (ch0 and ch1)
- [x] Configure TX LO frequency and gain
- [x] Create RX buffer for dual-channel streaming
- [x] Create TX buffer for calibration tone

### RX() Implementation
- [x] Read samples from RX buffer
- [x] Parse binary IQ data (12-bit to int16)
- [x] Deinterleave channel 0 and channel 1
- [x] Convert int16 to complex64 format
- [x] Return configured number of samples per channel

### TX() Implementation
- [x] Convert complex64 samples to int16 format
- [x] Interleave channels for AD9361
- [x] Write to TX buffer
- [x] Handle DDS tone generation

### Lifecycle Management
- [x] Implement Close() method
- [x] Add proper error handling throughout
- [x] Implement SetPhaseDelta() as no-op
- [x] Implement GetPhaseDelta() returning 0

---

## Phase 3: Add Missing IIO Protocol Commands

### Protocol Extensions
- [x] Implement OPEN command in [iiod/connect.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go)
- [x] Implement READBUF command with binary data handling
- [x] Implement WRITEBUF command with binary data
- [x] Implement CLOSE command
- [x] Add proper status code handling for buffer ops

### Binary Protocol Support
- [x] Add binary data reader helper
- [x] Add binary data writer helper
- [x] Handle length-prefixed binary payloads
- [x] Add timeout handling for streaming operations

### Integration
- [x] Update Client struct with buffer state tracking
- [x] Add mutex for thread-safe buffer operations
- [x] Implement proper cleanup on errors
- [x] Add connection health monitoring

---

## Phase 6: Forward-Compatible IIOD Protocol Support

### Protocol Version Detection
- [ ] Parse IIOD version from XML context (version-major, version-minor)
- [ ] Store protocol version in Client struct
- [ ] Create `ProtocolVersion` struct with comparison methods
- [ ] Implement version-based feature detection

### Legacy Protocol Support (v0.25)
- [ ] Implement attribute writes via direct register access
- [ ] Add sysfs-style attribute path mapping
- [ ] Implement `WriteAttrLegacy()` for v0.25 compatibility
- [ ] Add fallback mechanisms for unsupported commands
- [ ] Test with Pluto SDR firmware v0.38

### Modern Protocol Support (v0.26+)
- [ ] Keep existing [WriteAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#381-385) implementation for modern versions
- [ ] Support standard WRITE command format
- [ ] Implement batch operations for newer versions
- [ ] Add streaming optimizations for v0.26+

### Unified API Layer
- [ ] Create `WriteAttrCompat()` that auto-detects version
- [ ] Create `ReadAttrCompat()` with version fallbacks
- [ ] Update PlutoSDR backend to use compat methods
- [ ] Add version-specific error handling
- [ ] Document protocol differences

### Testing
- [ ] Create mock servers for v0.25 and v0.26+
- [ ] Test attribute writes on both protocol versions
- [ ] Test graceful degradation for unsupported features
- [ ] Verify XML parsing works across versions
- [ ] Test with real Pluto SDR hardware

---

## Phase 5: IIOD Performance & Reliability Enhancements

### Quick Wins (High ROI)

#### Context-Aware Operations
- [x] Add context parameter to [ReadBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#308-312)
- [x] Add context parameter to [WriteBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#325-329)
- [x] Add context parameter to [ReadAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#359-363) / [WriteAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#381-385)
- [x] Implement timeout handling with context
- [x] Test cancellation behavior

#### Metrics & Monitoring
- [x] Create [ClientMetrics](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#30-39) struct
- [x] Track bytes sent/received
- [x] Track command count and failures
- [x] Track average latency
- [x] Add [GetMetrics()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#78-82) method
- [x] Expose metrics via debug info

#### Debug Attributes
- [x] Implement [ReadDebugAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#404-408) command
- [x] Implement [WriteDebugAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#421-425) command
- [x] Add debug attribute tests
- [x] Document debug register access

### High Value Features

#### Auto-Reconnect
- [x] Create [ReconnectConfig](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#41-47) struct
- [x] Implement exponential backoff logic
- [x] Add connection health monitoring
- [x] Implement auto-reconnect on disconnect
- [x] Add `OnReconnect` callback for state restoration
- [/] Test reconnection scenarios

#### Streaming Optimization
- [ ] Create `StreamingBuffer` struct
- [ ] Implement double-buffering
- [ ] Add async read goroutine
- [ ] Implement channel-based data delivery
- [ ] Add backpressure handling
- [ ] Benchmark latency improvements

#### Batch Operations
- [x] Create [AttrOperation](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#440-447) struct
- [x] Implement [BatchReadAttrs()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#448-452)
- [x] Implement [BatchWriteAttrs()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#469-473)
- [x] Add pipelined command sending
- [/] Test atomic operations
- [ ] Benchmark initialization speedup

---

## Phase 4: IIO Interface Enhancements (Optional)

### High Priority Features

#### XML Context Discovery
- [ ] Implement [GetXMLContext()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#221-225) command
- [ ] Parse XML to extract device tree
- [ ] Create [Device](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#203-207) and [Channel](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#264-268) structs
- [ ] Implement `GetDeviceInfo()` for structured metadata
- [ ] Add tests for XML parsing

#### Trigger Support
- [ ] Implement `GetTrigger()` command
- [ ] Implement `SetTrigger()` command
- [ ] Add trigger validation
- [ ] Test synchronized multi-device sampling
- [ ] Document trigger usage patterns

#### Timeout Configuration
- [ ] Implement `SetTimeout()` command
- [ ] Add client-side timeout tracking
- [ ] Handle timeout errors gracefully
- [ ] Add timeout tests

#### Streaming API
- [ ] Create [StreamBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#21-69) method with callback
- [ ] Implement context cancellation support
- [ ] Add buffering for continuous reads
- [ ] Handle backpressure
- [ ] Test streaming performance

### Medium Priority Features

#### Debug Attributes
- [ ] Implement [ReadDebugAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#404-408) command
- [ ] Implement [WriteDebugAttr()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#421-425) command
- [ ] Add debug attribute discovery
- [ ] Document debug attribute usage

#### Health Monitoring
- [ ] Implement `Ping()` method
- [ ] Add `ConnectionStats` struct
- [ ] Track bytes sent/received
- [ ] Monitor connection health
- [ ] Add automatic reconnection logic

#### Metadata Structures
- [ ] Define [Device](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#203-207) struct with full metadata
- [ ] Define [Channel](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#264-268) struct with attributes
- [ ] Define `Attribute` struct
- [ ] Implement type-safe attribute access
- [ ] Add validation helpers

### Low Priority Features

#### Connection Pooling
- [ ] Create `ClientPool` struct
- [ ] Implement [Get()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#78-82) and `Put()` methods
- [ ] Add pool size configuration
- [ ] Handle pool exhaustion
- [ ] Test concurrent access

#### Batch Operations
- [ ] Implement [BatchReadAttrs()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#448-452)
- [ ] Implement [BatchWriteAttrs()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#469-473)
- [ ] Add transaction support
- [ ] Test atomic operations
- [ ] Benchmark performance gains

---

## Testing & Verification (Phase 4 - Optional)

### Unit Tests
- [ ] Create `internal/sdr/pluto_test.go`
- [ ] Test Init() with mock IIO client
- [ ] Test RX() data parsing and deinterleaving
- [ ] Test TX() data formatting
- [ ] Test configuration parameter mapping

### Integration Tests
- [ ] Test with mock IIOD server
- [ ] Test connection handling
- [ ] Test buffer lifecycle
- [ ] Test error recovery

### Manual Testing (Requires Hardware)
- [ ] Test connection to physical Pluto SDR
- [ ] Test IQ sample acquisition
- [ ] Test calibration tone transmission
- [ ] Test dual-channel phase coherence
- [ ] Compare with Mock backend performance
