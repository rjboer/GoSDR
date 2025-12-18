# Protocol Completeness, Design Scope, and Known Gaps

This document describes the current state of the IIOD client implementation, its intended scope, and the known functional gaps relative to a full libiio / pyadi-iio feature set. The goal of the current work is correct, deterministic binary RX streaming, not full protocol parity.

The implementation is deliberately minimal and protocol-faithful, prioritizing framing correctness, responder compatibility, and testability over convenience features.

## Architectural context
Modern IIOD servers (as implemented in `iiod-responder.c`) do not support ASCII-framed streaming. ASCII is used exclusively for:

- Context discovery (`PRINT`)
- Attribute reads/writes
- Control and configuration

All buffer streaming is implemented via the binary, block-based protocol, centered around:

- `CREATE_BUFFER`
- `ENABLE_BUFFER`
- `CREATE_BLOCK`
- `TRANSFER_BLOCK`

Any attempt to stream samples via ASCII commands such as `READBUF` is inherently unsafe and will desynchronize the TCP stream. This project therefore intentionally abandons ASCII streaming entirely.

## Binary protocol coverage
### Implemented (current state)
- RX streaming via binary responder
- Deterministic framing using fixed-size binary headers
- Single-block streaming using `CREATE_BUFFER → ENABLE_BUFFER → CREATE_BLOCK → TRANSFER_BLOCK (loop)`
- End-to-end validation via framing tests that tolerate arbitrary TCP segmentation

This represents the minimal viable RX pipeline required to reliably receive samples from a modern IIOD server.

### Intentionally not implemented (yet)
The following features are out of scope for the current phase and must be added before claiming full IIOD protocol coverage:

- TX (transmit) streaming
- Multiple blocks in flight
- Asynchronous / non-blocking enqueue–dequeue queues
- Buffer watermark / refill threshold control
- Trigger-based or externally synchronized starts
- Multi-client coordination and arbitration

These omissions are intentional. The current focus is correctness and protocol alignment, not throughput optimization or device policy.

### Action Plan
Ordered by recommended implementation priority, each missing capability should be delivered with clear objectives, dependency notes, success criteria, and explicit test coverage expectations:

1. **TX (transmit) streaming**
   - Objective: Add deterministic TX buffer lifecycle and streaming parity with RX.
   - Dependencies: Requires buffer lifecycle parity (allocate/enable/transfer/teardown) and TX underrun signaling surfaced to callers.
   - Success: Sustained TX at target sample rate without underruns or framing errors over long-duration runs.
   - Tests: Integration tests that stream TX tone patterns at multiple rates; unit tests for TX buffer setup/teardown edge cases.

2. **Multiple blocks in flight**
   - Objective: Allow pipelined `CREATE_BLOCK`/`TRANSFER_BLOCK` sequences to improve throughput.
   - Dependencies: Depends on TX/RX buffer correctness and block reference tracking to avoid double-free or misordered completion.
   - Success: Measured throughput improves with >1 inflight block without introducing overruns/underruns or misordered payloads.
   - Tests: Integration benchmarks with 2–4 inflight blocks; unit tests for block bookkeeping and completion handling.

3. **Asynchronous / non-blocking enqueue–dequeue queues**
   - Objective: Decouple producer/consumer timing with async queues for RX ingest and TX submission.
   - Dependencies: Builds on multi-block support and requires thread-safe queue management around buffer locks.
   - Success: RX/TX processing remains stable under scheduler jitter; no dropped samples when producers/consumers temporarily stall.
   - Tests: Concurrency-focused unit tests on queue backpressure behavior; integration tests with simulated jittered producers/consumers.

4. **Buffer watermark / refill threshold control**
   - Objective: Expose and honor buffer watermarks to control refill thresholds and reduce underrun/overrun risk.
   - Dependencies: Relies on async queue plumbing and accurate buffer occupancy reporting from the transport layer.
   - Success: Configurable watermarks demonstrably reduce underruns/overruns in stress tests compared to default settings.
   - Tests: Integration stress tests sweeping watermark settings; unit tests for configuration validation and translation to protocol commands.

5. **Trigger-based or externally synchronized starts**
   - Objective: Support start conditions tied to triggers (e.g., GPIO, PPS) for coordinated RX/TX.
   - Dependencies: Requires stable buffer lifecycle and watermark handling so trigger-armed buffers remain valid until fire.
   - Success: Triggered starts initiate reliably within expected latency bounds and align across RX/TX when requested.
   - Tests: Integration tests with simulated trigger events; unit tests for trigger configuration parsing and state transitions.

6. **Multi-client coordination and arbitration**
   - Objective: Manage multiple IIOD client IDs with clear ownership and arbitration rules.
   - Dependencies: Requires mature single-client streaming (including above items) and client ID wiring across transports.
   - Success: Concurrent clients can stream without interfering headers or buffer collisions; policy rejects incompatible mixes.
   - Tests: Integration tests with simultaneous clients performing RX/TX; unit tests for client ID routing and conflict detection.

## Transfer block response handling
### Current behavior
The current implementation assumes that each `TRANSFER_BLOCK` response contains:

- A valid binary response header
- A payload containing sample data

This is sufficient for the happy-path RX case.

### Known gap
In reality, the responder may legally return:

- Status-only responses (no payload)
- Short reads (payload smaller than requested)
- Negative return codes without payload data

These scenarios occur during:

- Buffer teardown
- Underruns
- Device-side errors
- End-of-stream conditions

### Future work
The client should be extended to:

- Always parse and surface the binary response code
- Tolerate zero-length payloads
- Distinguish between transient conditions and fatal errors
- Avoid assuming “payload always follows”

This is a protocol-level improvement, not a transport or framing issue.

## Attribute writes and negative return codes
Negative responses such as:

- `-6 → ENXIO`
- `-22 → EINVAL`
- `-95 → EOPNOTSUPP`

are expected and valid in many scenarios, including:

- Writing attributes in the wrong device or channel context
- Attempting writes after buffers are enabled
- Accessing read-only attributes
- Device-specific restrictions

These failures do not indicate a streaming or protocol error.

### Comparison with Python behavior
`pyadi-iio` mitigates these cases via:

- Strict ordering of operations
- Device-specific policy
- Automatic retries
- SSH fallbacks for legacy devices

These ergonomics are intentionally out of scope for this implementation. The Go client exposes raw protocol semantics rather than hiding them.

## Operational safeguards
### ASCII helpers after binary mode
Once binary mode is enabled:

- No ASCII helpers must be allowed to run
- Any attempt to do so should fail fast with a clear error

This prevents:

- Accidental mixed-mode usage
- Latent buffered reads
- Permanent TCP stream desynchronization

Recommended guard pattern:

```go
if m.Mode == ModeBinary {
    return errors.New("ASCII operations are forbidden after binary mode activation")
}
```

### Client ID handling
- The IIOD responder supports multiple client IDs multiplexed over a single connection.
- The current client defaults to `clientID = 0`.
- A `SetClientID()` hook exists and is correct.
- Multi-client support requires explicit wiring and coordination.

Single-client usage is safe as implemented.

## Comparison with pyadi-iio
Python’s `pyadi-iio` stack adds substantial policy and convenience layers on top of the same binary protocol primitives used here:

- Automatic retries
- SSH-based attribute fallbacks
- Device-specific heuristics
- Higher-level RX/TX abstractions

Critically:

- Python does not use a different streaming protocol.
- The core sequence is identical: `CREATE_BUFFER → ENABLE_BUFFER → CREATE_BLOCK → TRANSFER_BLOCK`.
