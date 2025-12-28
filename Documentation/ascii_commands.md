# ASCII Control Commands

The legacy ASCII protocol exposes a handful of discovery commands that return
variable-length payloads. Each command responds with an integer length on the
first line, followed by the payload bytes and a trailing `\n` delimiter.
Commands are always issued with CRLF endings.

## HELP
- Command: `HELP\r\n`
- Response: integer byte count, then the list of available commands.
- Behavior: the payload is trimmed of the trailing line ending to keep the socket
  aligned for subsequent requests.

## VERSION
- Command: `VERSION\r\n`
- Response: integer byte count plus the server version string.
- Behavior: the newline terminator is removed from the returned string, ensuring
  later reads start at the next status line.

## PRINT
- Command: `PRINT\r\n`
- Response: integer byte count followed by the context XML and a trailing `\n`.
- Behavior: the helper strips only the protocol delimiter; the XML is returned
  unchanged otherwise. A non-positive length is treated as an error.

## ZPRINT
- Command: `ZPRINT\r\n`
- Response: integer byte count plus a zlib-compressed XML body terminated with a
  newline.
- Behavior and edge cases:
  - The trailing newline is removed before decompression to avoid corrupting the
    zlib stream.
  - Decompression errors are surfaced directly, allowing callers to detect
    truncated or non-compressed payloads.
  - Non-positive length headers are rejected before any socket reads.
