# Pluto SDR Backend Implementation Tasks

## Phase 1: Extend IIO Client with Buffer Operations

### IIO Buffer Implementation
- [x] Create [iiod/buffer.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go) with Buffer struct
- [x] Implement [CreateStreamBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#21-69) method
- [x] Implement [EnableChannel()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#70-78) for channel masking
- [x] Implement [ReadSamples()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/buffer.go#136-160) for binary data streaming
- [x] Implement [Close()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/sdr.go#25-26) for buffer cleanup
- [x] Add binary data parsing utilities (little-endian int16)

### IIO Protocol Commands
- [ ] Add [OpenBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#149-162) command to [iiod/connect.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go)
- [ ] Add [ReadBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#163-174) command for data streaming
- [ ] Add [WriteBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#175-188) command for TX data
- [ ] Add [CloseBuffer()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#189-198) command
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
- [ ] Define PlutoSDR struct in [internal/sdr/pluto.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/sdr/pluto.go)
- [ ] Add IIO client connection handling
- [ ] Add RX/TX buffer management
- [ ] Add AD9361 device handle tracking

### Init() Implementation
- [ ] Implement IIO client connection via Dial()
- [ ] Add device discovery (ListDevices)
- [ ] Configure sample rate attribute
- [ ] Configure RX LO frequency
- [ ] Configure RX gains (ch0 and ch1)
- [ ] Configure TX LO frequency and gain
- [ ] Create RX buffer for dual-channel streaming
- [ ] Create TX buffer for calibration tone

### RX() Implementation
- [ ] Read samples from RX buffer
- [ ] Parse binary IQ data (12-bit to int16)
- [ ] Deinterleave channel 0 and channel 1
- [ ] Convert int16 to complex64 format
- [ ] Return configured number of samples per channel

### TX() Implementation
- [ ] Convert complex64 samples to int16 format
- [ ] Interleave channels for AD9361
- [ ] Write to TX buffer
- [ ] Handle DDS tone generation

### Lifecycle Management
- [ ] Implement Close() method
- [ ] Add proper error handling throughout
- [ ] Implement SetPhaseDelta() as no-op
- [ ] Implement GetPhaseDelta() returning 0

---

## Phase 3: Add Missing IIO Protocol Commands

### Protocol Extensions
- [ ] Implement OPEN command in [iiod/connect.go](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go)
- [ ] Implement READBUF command with binary data handling
- [ ] Implement WRITEBUF command with binary data
- [ ] Implement CLOSE command
- [ ] Add proper status code handling for buffer ops

### Binary Protocol Support
- [ ] Add binary data reader helper
- [ ] Add binary data writer helper
- [ ] Handle length-prefixed binary payloads
- [ ] Add timeout handling for streaming operations

### Integration
- [ ] Update Client struct with buffer state tracking
- [ ] Add mutex for thread-safe buffer operations
- [ ] Implement proper cleanup on errors
- [ ] Add connection health monitoring

---

## Phase 4: IIO Interface Enhancements (Optional)

### High Priority Features

#### XML Context Discovery
- [ ] Implement `GetXMLContext()` command
- [ ] Parse XML to extract device tree
- [ ] Create [Device](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#109-120) and [Channel](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#121-136) structs
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
- [ ] Implement `ReadDebugAttr()` command
- [ ] Implement `WriteDebugAttr()` command
- [ ] Add debug attribute discovery
- [ ] Document debug attribute usage

#### Health Monitoring
- [ ] Implement `Ping()` method
- [ ] Add `ConnectionStats` struct
- [ ] Track bytes sent/received
- [ ] Monitor connection health
- [ ] Add automatic reconnection logic

#### Metadata Structures
- [ ] Define [Device](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#109-120) struct with full metadata
- [ ] Define [Channel](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#121-136) struct with attributes
- [ ] Define `Attribute` struct
- [ ] Implement type-safe attribute access
- [ ] Add validation helpers

### Low Priority Features

#### Connection Pooling
- [ ] Create `ClientPool` struct
- [ ] Implement [Get()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/iiod/connect.go#121-136) and `Put()` methods
- [ ] Add pool size configuration
- [ ] Handle pool exhaustion
- [ ] Test concurrent access

#### Batch Operations
- [ ] Implement `BatchReadAttrs()`
- [ ] Implement `BatchWriteAttrs()`
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
