# mDNS / DNS-SD Discovery Test Tool

This command-line utility performs multicast DNS (mDNS) service discovery for devices that advertise the service:

```
_iio._tcp.local
```

This service type is used by **libiio / IIOD** devices such as:

- PlutoSDR  
- SDRs running the IIOD daemon  
- Any device exposing an Industrial I/O context over TCP

The program uses the shared library in:

```
internal/mdns
```

which provides a clean Go wrapper for DNS-SD discovery.

---

## Features

- Discovers all IIOD-capable devices on the local network  
- Displays:
  - Instance name (`"iiod on pluto"`, etc.)
  - Hostname
  - Port
  - IPv4 and IPv6 addresses
  - TXT metadata (if provided)
- Shows connection hints (`tcp://host:port`)  
- Cross-platform: Windows, Linux, macOS

This is a diagnostic tool intended for developers integrating GoSDR with real hardware.

---

## Usage

From the root of the repository:

```bash
go run ./cmd/mdns-test
```

Optional arguments:

```bash
-timeout <seconds>
Example:
go run ./cmd/mdns-test -timeout 5
```

---

## Example Output

```
===============================================================
 mDNS / DNS-SD Discovery Test
===============================================================
 Service : _iio._tcp.local
 Timeout : 5s
---------------------------------------------------------------
Discovered 1 service(s) in 5.029s
===============================================================
 Device #1
---------------------------------------------------------------
 Instance : iiod on pluto
 Hostname : pluto.local.
 Port     : 30431
 Addresses:
   - 192.168.2.1
 TXT Records:
   <none>
 Connection hints:
   - tcp://192.168.2.1:30431
===============================================================
```

This output confirms:

- Discovery is working  
- A PlutoSDR or other IIOD server is reachable  
- The tool can extract necessary info to build an IIOD connection URI

---

## Architecture

The tool is intentionally small.  
Discovery logic lives in:

```
internal/mdns/mdns.go
```

and exposes a wrapper function:

```go
hosts, err := mdns.DiscoverIIOD(timeoutSeconds)
```

The test program uses this wrapper and only handles formatting and printing.

---

## When to Use This Tool

Use this program when:

- Checking whether your device advertises itself via mDNS  
- Verifying network visibility of a PlutoSDR  
- Debugging IIOD discovery issues  
- Testing Windows/macOS/Linux differences in mDNS behavior  
- Ensuring that GoSDR’s internal mdns library is working correctly

## future ideas

- add support for other services
- implement mdns myself

