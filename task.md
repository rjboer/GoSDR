# Task: Implement ASCII Protocol Support in Go

## Phase 1: Core Optimizations
- [ ] **Optimize `Manager.readLine`** <!-- id: 22 -->
    - **Step**: Replace `io.ReadAll` with `bufio` or manual buffer scan.
    - **Verify**: Benchmark or check code to ensure NO per-byte loops are used.
    - **OK Condition**: Finds `\n` efficiently and returns exactly one line.
- [ ] **Add GoDoc Documentation** <!-- id: 23 -->
    - **Step**: Add comments to all exported ASCII methods.
    - **Verify**: Check for `Purpose`, `Parameters`, `Protocol`, `Returns`.

## Phase 2: Context & Attribute Management
- [ ] **Implement `SetTimeoutASCII`** <!-- id: 21 -->
    - **Step**: Send `TIMEOUT %d\n`.
    - **Verify**: Call with valid/invalid ms.
    - **OK**: Returns `0` for success, error for < 0.
- [ ] **Implement Triggers ([Get](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/binary_wrappers.go#10-32)/[Set](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#67-71))** <!-- id: 24 -->
    - **Step**: `GETTRIG` returns string; `SETTRIG` takes device + trigger.
    - **Verify**: Get returns current trigger; Set changes it.
    - **OK**: Device accepts valid trigger name.
- [ ] **Verify [ReadDeviceAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#13-42)** <!-- id: 7 -->
    - **Step**: Audit code for [ExecASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#103-107) + [readLine](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#161-204) (Length/Value).
    - **OK**: Correctly parses `samples_count` (length) and reads payload.
- [ ] **Verify [ReadChannelAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#43-73)** <!-- id: 8 -->
    - **Step**: Check standard `READ ...` sequence handling.
    - **OK**: Handles 4-part command correctly.
- [ ] **Implement `ReadDebugAttrASCII`** <!-- id: 19 -->
    - **Step**: Clone DeviceAttr logic with `DEBUG` token.
    - **OK**: Reads debug reg values.
- [ ] **Implement `ReadBufferAttrASCII`** <!-- id: 20 -->
    - **Step**: Clone DeviceAttr logic with `BUFFER` token.
    - **OK**: Reads buffer params.
- [ ] **Verify [WriteDeviceAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#144-173)** <!-- id: 17 -->
    - **Step**: Check `WRITE ... %d\n` + Payload pattern.
    - **OK**: Sends length header, then payload without extra newline.
- [ ] **Verify [WriteChannelAttrASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/attrs_ascii.go#174-206)** <!-- id: 25 -->
    - **Step**: Same as Device Write but with channel args.
    - **OK**: Correctly updates channel params.
- [ ] **Implement `WriteDebugAttrASCII`** <!-- id: 26 -->
    - **Step**: `WRITE ... DEBUG ...`.
- [ ] **Implement `WriteBufferAttrASCII`** <!-- id: 27 -->
    - **Step**: `WRITE ... BUFFER ...`.

## Phase 3: Streaming (Audit & Fix)
- [ ] **Fix [ReadBufferASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#39-103) (Mask Bug)** <!-- id: 12 -->
    - **Step**: Insert [readInteger()](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/ascii.go#8-63) call after reading length to consume Mask line.
    - **Verify**: Read 4096 bytes; ensure first bytes are NOT hex characters of a mask.
    - **OK**: Buffer contains strict binary data.
- [ ] **Verify [OpenBufferASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#8-38)** <!-- id: 10 -->
    - **Step**: Check `OPEN` command args (samples, mask, cyclic).
    - **OK**: Returns 0.
- [ ] **Verify [CloseBufferASCII](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/buffer_ascii.go#124-141)** <!-- id: 11 -->
    - **Step**: Check `CLOSE` command.
    - **OK**: Returns 0.
- [ ] **Implement `WriteBufferASCII`** <!-- id: 13 -->
    - **Step**: `WRITEBUF` header + optimized [writeAll](file:///c:/Users/Roelof%20Jan/GolandProjects/RJBOER/GoSDR/internal/connectionmgr/manager.go#129-146).
    - **OK**: Server accepts TX data.

## Phase 4: Verification
- [ ] **Create `ascii_params_test.go`** <!-- id: 14 -->
    - **Step**: Unit tests mocking the IIOD server responses.
- [ ] **Create `ascii_streaming_test.go`** <!-- id: 15 -->
    - **Step**: Mock stream with Mask line to verify fix.
