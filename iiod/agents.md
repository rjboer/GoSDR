1\. IIOD Client Agent

Source Files
client.go (primary state owner)
connect.go, textbased.go, binarybased.go, xml.go, buffer.go

Mission
Provide a single authoritative representation of:

Transport (binary protocol, text protocol, legacy mode)
Connection lifecycle (TCP, Unix socket, fallback)
All XML context, device attributes, channel maps
Buffer creation, destruction, read/write, debug hooks
Logging, metrics, and diagnostics
Error normalization across transports

Responsibilities
Central State Owner
No other package may create independent IIOD-like state.
Client.go exposes all IO operations via methods:
    Attribute Get/Set
    Buffer Open/Close/Read/Write
Device and Channel enumeration
XML context retrieval
Protocol Routing
Automatic detection of binary protocol:
    Attempt VERSION (binary)
    If fail → attempt PRINT (text)
    If text mode works → mark server as legacy
Runtime routing ensures that callers do not know or care which mode is active.
XML Metadata Cache
Parse, normalize, cross-link XML attributes
Fill missing devices/channels if the remote server is incomplete
Keep lastXML for introspection
Attribute Access Layer
    ReadAttrCompat() → automatically selects binary first; fallback to text
    WriteAttrCompat() → same fallback mechanism
Logs explicit fallback events
Unified Buffer API
Exposes:
    OpenBuffer(device, samples, cyclic)
    ReadBuffer()
    WriteBuffer()
    CloseBuffer()
All implemented in terms of the active transport (binary or text).
Debug + Telemetry Integration
Debug levels: OFF / BASIC / VERBOSE / WIREFRAME
Hexdump of TX/RX frames when requested
Tracks protocol errors and auto-downgrades transport if corruption detected
PlutoSDR SSH Fallback (optional)
When sysfs access is required (e.g., RF board control outside IIOD),
the client may engage an SSH subsystem to read/write sysfs nodes.
This remains a secondary path, triggered only when IIOD lacks attribute coverage.

