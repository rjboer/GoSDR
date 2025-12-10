Monopulse SDR System — Architecture & Agent Specification

Version 1.0 (Professional Edition)
0. Purpose and Scope
This document defines the complete, professional system architecture specification for the Monopulse SDR stack, including:

High-level system agents
Responsibilities and interaction contracts
IIOD client architecture (binary/text protocol router)
SDR backend (PlutoSDR)
DSP subsystem
Tracking and telemetry agents
Data models
Error taxonomy
Concurrency and lifecycle states
Performance constraints
Extensibility guidelines
This document is normative and governs how all subsystems MUST interoperate.

1. System Overview

The Monopulse SDR stack consists of the following core agents:

Agent	Role
IIOD Client Agent	Unified transport (binary/text), device metadata, buffers
SDR Hardware Agent	Hardware operations (PlutoSDR)
DSP Agent	Pure signal processing (no IO)
Tracker Agent	Monopulse control loop
Telemetry Agent	Reporting, visualization, debugging
Configuration Agent	System configuration and validation

The architecture strictly separates IO, control, and DSP mathematics.

2. Agents: Roles and Responsibilities
2.1 IIOD Client Agent

Source Files:
client.go, connect.go, binarybased.go, textbased.go, xml.go, buffer.go

Mission
Provide a single authoritative API to remote IIO-based SDR devices (PlutoSDR).
All subsystems use this client; no alternative transport layers are permitted.

Responsibilities
2.1.1 Transport Routing
Detect protocol capabilities automatically:
Attempt binary VERSION
If fails → fallback to legacy text PRINT

If text works → system operates in compatibility mode

Binary/text transports MUST expose identical APIs.

2.1.2 Connection Lifecycle
TCP/Unix socket management
Heartbeats (optional)
Reconnect logic
Shutdown and resource disposal

2.1.3 XML Context Management
Download full IIOD XML context
Parse and normalize device/channels/attributes
Enrich metadata with missing fields
Maintain an internal metadata cache (lastXML)

2.1.4 Attribute Access
ReadAttrCompat, WriteAttrCompat
Prefer binary access
Automatic fallback to text mode
Normalize errors and warnings
Per-device attribute maps
High-resolution logging when debug mode is enabled

2.1.5 Streaming Buffers
Abstract RX/TX buffer creation for all transports
Enforce buffer alignment rules
Provide ReadBuffer() and WriteBuffer()
Handle overrun/underrun and reallocation

2.1.6 SSH Sysfs Fallback (Pluto only)
When IIOD lacks access to sysfs nodes
Optional security: key auth, sandbox paths

2.1.7 Debugging and Telemetry
Debug levels:
0 – Disabled  
1 – Errors only  
2 – High-level operations  
3 – Transport wireframes (hex dumps)  
4 – Full diagnostics (buffers, XML bodies)

2.2 SDR Hardware Agent
Mission
Provide hardware-level functionality behind a stable Go interface.

Responsibilities
2.2.1 Initialization
Discover physical devices
Load XML metadata from Client
Validate AD9361 presence
Apply radio configuration (LO, sample rate, gains, FIR, bandwidth)

2.2.2 Streaming
Manage RX/TX buffers
Call InterleaveIQ / DeinterleaveIQ helpers
Apply runtime configuration changes

2.2.3 Gain and LO Fallbacks
AGC fallback if manual gain unavailable
TX attenuation fallback if hardwaregain missing
LO lock verification

2.2.4 Device Safety Rules
Prevent illegal TX power
Enforce frequency bounds
Avoid rapid LO retuning sequences

2.3 DSP Agent
Pure computation. Zero side effects.

Responsibilities
Windowing, FFT, spectrum analysis
IQ channel manipulation (sum, delta)
Angle-of-arrival estimation

Monopulse error calculation

Adaptive filters (future expansion)

MUST remain stateless and deterministic.

2.4 Tracker Agent
Responsibilities
Coarse scan and initial acquisition
Tracking loop (continuous mode)

Phase and amplitude control

Integration with Telemetry Agent

Automatic re-acquisition if signal is lost

Store and expose runtime metrics

2.5 Telemetry Agent
Responsibilities
Real-time data export (WebSocket/HTTP)
Human-readable logs

Spectrum snapshots (optional)

Streaming state, DOA, SNR, gains

Debug-level reporting

Telemetry MUST NOT apply any DSP or SDR operations.

2.6 Configuration Agent
Responsibilities
Read, merge, and validate program configuration
Provide typed structs to all components
Enforce constraints (sample rate, spacing, gains)

3. Agent Interaction Contracts
3.1 Global Call Graph
Tracker
  ↓
SDR
  ↓
IIOD Client
  ↓
Transport (binary/text)
  ↓
PlutoSDR or other IIO device

Tracker → DSP (FFT)
Tracker → Telemetry (updates)
SDR → DSP (IQ conversions only)

3.2 Timing Contract

The SDR + DSP + Tracker pipeline MUST finish processing each RX buffer before the next arrives.

Define:

t_buffer = samples / sample_rate
t_compute < t_buffer * 0.8


Failure implies the tracker loses lock.

3.3 Failure Contracts
Failure	Agent Reaction
Binary protocol fails	Client falls back to text
Text protocol fails	Client attempts SSH sysfs fallback
Attribute not found	SDR uses fallback mode
IQ corruption	Tracker discards frame, logs event
LO not locked	SDR retries; tracker halts
RX overrun	Buffer recreated
Sample clock drift	DSP adaptive filter (future)
4. Client State Machines
4.1 IIOD Client Mode Detection FSM
START
 ↓ try VERSION (binary)
BINARY_OK? ── yes → BINARY_MODE
     │
     no
     ↓ try PRINT (text)
TEXT_OK? ────── yes → TEXT_MODE
     │
     no
     ↓
FAIL

4.2 Tracker FSM
INIT
 ↓
CALIBRATE
 ↓
SCAN
 ↓
TRACK
 ↓
ERROR ←─────────────┐
 ↑ (retry/acq)       │
 └───────────────────┘

5. Data Model Specification
5.1 IQ Data Layout
Interleaved buffer format (binary protocol)
I0 Q0 I1 Q1 I2 Q2 ...

Deinterleaved in DSP:
[]complex64 per channel

Pluto channel mapping
RX1_I → voltage0
RX1_Q → voltage1
RX2_I → voltage2
RX2_Q → voltage3

5.2 Attribute Model
GetAttr(device, channel, name) → string
SetAttr(device, channel, name, value) → error


Attributes are backed by:

Binary attribute codes (preferred)

Text-based file names (fallback)

SSH sysfs when necessary

5.3 Buffer Metadata
struct Buffer {
    Device         string
    Samples        int
    ByteStride     int
    Cyclic         bool
    TransportMode  enum { Binary, Text }
}

5.4 XML Metadata Model
Normalized and enriched:
Context:
  Name
  Version
  Devices[]:
    ID
    Name
    Channels[]
    Attributes[]
    DebugAttributes[]
    BufferAttributes[]


Missing fields MUST be auto-generated when necessary.

6. Error Taxonomy
6.1 Error Classes
IIOD_ERR_TRANSPORT
IIOD_ERR_PROTOCOL
IIOD_ERR_XML
IIOD_ERR_ATTRIBUTE
IIOD_ERR_FALLBACK
SDR_ERR_GAIN
SDR_ERR_LO_UNLOCKED
SDR_ERR_BUFFER
SDR_ERR_OVERRUN
DSP_ERR_IQ_INVALID
TRACKER_ERR_NO_SIGNAL
TRACKER_ERR_TIMEOUT
CONFIG_ERR_INVALID

7. Concurrency Model
7.1 General Principles
DSP operations are stateless, safe to run concurrently
RX/TX buffer operations MUST be serialized per IIOD connection
Tracker loop runs in dedicated goroutine
Telemetry runs in background event loop
Locks:
transportLock protects all socket operations
xmlLock protects metadata updates
bufferLock protects buffer allocation table

7.2 Thread Safety Contracts
The SDR Agent MUST NOT directly modify the Client without going through public methods.

8. Performance Requirements
8.1 Timing
End-to-end processing must remain within 80% of buffer duration
Buffer overrun must trigger controlled reinitialization

8.2 Memory
Buffer reuse: avoid per-frame allocations
FFT workspace must be pooled

8.3 Throughput
PlutoSDR must sustain:
≥ 2 MSPS RX without drops
≥ 2 MSPS TX for tone generation
Stable LO lock over long periods

9. Security & Operational Safety
SSH access must be limited to validated sysfs paths
TX gain/attenuation must respect board limits
Prevent inadvertent high-power transmissions
Protect against malformed XML attacks

10. Extensibility Guidelines
10.1 Adding a New SDR Backend
To support LimeSDR, USRP, etc.:
Implement SDR interface
Provide device discovery function
Build attribute and buffer mapping logic
Register new backend in factory

10.2 Adding a New IIOD Transport
Add a transport module
Implement VERSION/PRINT detection
Register with dispatcher in connect.go
Ensure compat methods are wired

10.3 Adding New DSP Algorithms
MUST be deterministic
MUST be thread-safe
MUST accept and return pure data structures

11. Future Work
Automatic calibration agent
Predictive tracking using adaptive filters
GPU/SIMD FFT acceleration
Virtual IIOD server for offline testing
Machine-learning–based DOA estimation