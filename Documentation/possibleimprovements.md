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
